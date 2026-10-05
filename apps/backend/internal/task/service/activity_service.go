package service

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// ErrActivityUnsupported is returned when the configured repository has no
// task-activity projection capability.
var ErrActivityUnsupported = errors.New("task activity is not supported")

// taskActivityReader is the optional repository capability backing the merged
// task-activity projection. Declared locally so repository fakes that model
// only the core surface stay unchanged.
type taskActivityReader interface {
	ListTaskActivityTransitions(ctx context.Context, taskID string, filter models.TaskActivityFilter) ([]*models.TaskStepTransition, error)
	CountCommittedTransitionPairs(ctx context.Context, taskID string) ([]models.StepTransitionCount, error)
	ListTaskActivityDocumentRevisions(ctx context.Context, taskID string, before *time.Time, limit int) ([]*models.TaskDocumentRevision, error)
	CountStepEntriesByTask(ctx context.Context, taskID string) ([]models.StepVisitSummary, error)
	ListTaskSessionRoutes(ctx context.Context, taskID string) ([]*models.TaskSessionRoute, error)
	ListTaskActivityReviewRuns(ctx context.Context, taskID, stepID string, before *time.Time, limit int) ([]*models.TaskReviewRun, error)
}

func (s *Service) activityReader() (taskActivityReader, bool) {
	reader, ok := s.tasks.(taskActivityReader)
	return reader, ok
}

// ListTaskActivity returns one page of the merged, chronological task activity
// stream, oldest cursor semantics: newest first by (occurred_at, kind, id).
// `has_more` is true when older events remain; the caller derives the opaque
// cursor from the last event's OccurredAt.
func (s *Service) ListTaskActivity(
	ctx context.Context,
	taskID string,
	filter models.TaskActivityFilter,
) ([]*models.TaskActivityEvent, bool, error) {
	if err := s.AuthorizeTaskAccess(ctx, taskID); err != nil {
		return nil, false, err
	}
	reader, ok := s.activityReader()
	if !ok {
		return nil, false, ErrActivityUnsupported
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	events, err := s.collectActivityCandidates(ctx, reader, taskID, filter, limit)
	if err != nil {
		return nil, false, err
	}
	return paginateActivity(events, limit)
}

// collectActivityCandidates gathers bounded rows from every authoritative
// source, filtered (where possible in SQL) to strictly older than the cursor.
func (s *Service) collectActivityCandidates(
	ctx context.Context,
	reader taskActivityReader,
	taskID string,
	filter models.TaskActivityFilter,
	limit int,
) ([]*models.TaskActivityEvent, error) {
	before := filter.Before
	events := make([]*models.TaskActivityEvent, 0, limit+1)

	transitions, err := reader.ListTaskActivityTransitions(ctx, taskID, models.TaskActivityFilter{
		StepID: filter.StepID, Before: before, Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	routes, routeErr := reader.ListTaskSessionRoutes(ctx, taskID)
	if routeErr != nil {
		return nil, routeErr
	}
	if filter.StepID == "" {
		for _, route := range routes {
			events = append(events, activityRouteEvent(route))
		}
		revisions, revErr := reader.ListTaskActivityDocumentRevisions(ctx, taskID, before, limit)
		if revErr != nil {
			return nil, revErr
		}
		for _, rev := range revisions {
			events = append(events, &models.TaskActivityEvent{
				Kind: models.ActivityKindDocumentRevision, ID: rev.ID, OccurredAt: rev.CreatedAt,
				DocumentRevision: rev,
			})
		}
		for _, transition := range transitions {
			events = append(events, &models.TaskActivityEvent{
				Kind: models.ActivityKindTransition, ID: activityTransitionID(transition),
				OccurredAt: transition.OccurredAt, Transition: transition,
			})
		}
	} else {
		// Step scope is a visit timeline: attach each visit's routing decision
		// (matched through workflow_step_transition_id) so the row can show the
		// routed profile/outcome/reason and Open the persisted destination.
		events = append(events, activityStepVisitEvents(transitions, routes, filter.StepID)...)
	}
	runs, runErr := reader.ListTaskActivityReviewRuns(ctx, taskID, filter.StepID, before, limit)
	if runErr != nil {
		return nil, runErr
	}
	for _, run := range runs {
		events = append(events, &models.TaskActivityEvent{
			Kind: models.ActivityKindReviewRun, ID: run.ID, OccurredAt: run.CreatedAt, ReviewRun: run,
		})
	}
	if filter.StepID == "" {
		sessionEvents, sessErr := s.sessionActivityEvents(ctx, taskID)
		if sessErr != nil {
			return nil, sessErr
		}
		events = append(events, sessionEvents...)
	}
	if before != nil {
		kept := events[:0]
		for _, event := range events {
			if event.OccurredAt.Before(*before) {
				kept = append(kept, event)
			}
		}
		events = kept
	}
	return events, nil
}

// sessionActivityEvents emits one creation event per session and, when the
// session has final completion, one completion event. Current state rides on
// the session payload; this never claims a full historical state machine.
func (s *Service) sessionActivityEvents(ctx context.Context, taskID string) ([]*models.TaskActivityEvent, error) {
	sessionsByTask, err := s.sessions.BatchGetSessionsByTaskIDs(ctx, []string{taskID})
	if err != nil {
		return nil, err
	}
	var events []*models.TaskActivityEvent
	for _, session := range sessionsByTask[taskID] {
		events = append(events, &models.TaskActivityEvent{
			Kind: models.ActivityKindSessionCreated, ID: "created:" + session.ID,
			OccurredAt: session.StartedAt, Session: session,
		})
		if session.CompletedAt != nil {
			events = append(events, &models.TaskActivityEvent{
				Kind: models.ActivityKindSessionCompleted, ID: "completed:" + session.ID,
				OccurredAt: *session.CompletedAt, Session: session,
			})
		}
	}
	return events, nil
}

// activityRouteEvent wraps one durable routing decision as a stream row.
func activityRouteEvent(route *models.TaskSessionRoute) *models.TaskActivityEvent {
	return &models.TaskActivityEvent{
		Kind: models.ActivityKindRoute, ID: route.ID, OccurredAt: route.CreatedAt, Route: route,
	}
}

// activityStepVisitEvents builds the step-scoped visit stream: every committed
// transition into the step, with its routing decision attached when the route
// ledger links through workflow_step_transition_id. Step routes that carry no
// transition link still surface as standalone rows so a visit without a
// recorded transition is never hidden.
func activityStepVisitEvents(
	transitions []*models.TaskStepTransition,
	routes []*models.TaskSessionRoute,
	stepID string,
) []*models.TaskActivityEvent {
	stepRoutes := make([]*models.TaskSessionRoute, 0, len(routes))
	byTransition := make(map[int64]*models.TaskSessionRoute)
	for _, route := range routes {
		if route.DestinationWorkflowStepID != stepID {
			continue
		}
		stepRoutes = append(stepRoutes, route)
		if route.WorkflowStepTransitionID != nil {
			byTransition[*route.WorkflowStepTransitionID] = route
		}
	}
	events := make([]*models.TaskActivityEvent, 0, len(transitions)+len(stepRoutes))
	for _, transition := range transitions {
		event := &models.TaskActivityEvent{
			Kind: models.ActivityKindTransition, ID: activityTransitionID(transition),
			OccurredAt: transition.OccurredAt, Transition: transition,
		}
		if route, ok := byTransition[transition.ID]; ok {
			event.Route = route
		}
		events = append(events, event)
	}
	for _, route := range stepRoutes {
		if route.WorkflowStepTransitionID == nil {
			events = append(events, activityRouteEvent(route))
		}
	}
	return events
}

func activityTransitionID(transition *models.TaskStepTransition) string {
	return "transition:" + strconv.FormatInt(transition.ID, 10)
}

// paginateActivity sorts newest-first with a deterministic tie-break and
// returns a page that never splits a same-timestamp group, so the next page
// (strictly older than the returned cursor) can neither skip nor duplicate.
func paginateActivity(
	events []*models.TaskActivityEvent,
	limit int,
) ([]*models.TaskActivityEvent, bool, error) {
	sort.SliceStable(events, func(i, j int) bool {
		left, right := events[i], events[j]
		if !left.OccurredAt.Equal(right.OccurredAt) {
			return left.OccurredAt.After(right.OccurredAt)
		}
		leftKey := string(left.Kind) + ":" + left.ID
		rightKey := string(right.Kind) + ":" + right.ID
		return leftKey > rightKey
	})
	page := events
	if len(page) > limit {
		page = page[:limit]
		boundary := page[len(page)-1].OccurredAt
		for len(events) > len(page) && events[len(page)].OccurredAt.Equal(boundary) {
			page = events[:len(page)+1]
		}
	}
	hasMore := len(events) > len(page)
	return page, hasMore, nil
}

// TaskTransitionSummary returns the bounded header payload: committed directed
// pair counts, plus per-step visit counts with the persisted destination
// session of the latest route into each step (never the initiating session).
func (s *Service) TaskTransitionSummary(ctx context.Context, taskID string) (*models.TaskTransitionSummary, error) {
	if err := s.AuthorizeTaskAccess(ctx, taskID); err != nil {
		return nil, err
	}
	reader, ok := s.activityReader()
	if !ok {
		return nil, ErrActivityUnsupported
	}
	counts, err := reader.CountCommittedTransitionPairs(ctx, taskID)
	if err != nil {
		return nil, err
	}
	visits, err := reader.CountStepEntriesByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	routes, err := reader.ListTaskSessionRoutes(ctx, taskID)
	if err != nil {
		return nil, err
	}
	sessionsByStep := stepVisitSessions(routes)
	for i := range visits {
		sessions := sessionsByStep[visits[i].StepID]
		if sessions == nil {
			sessions = []models.StepVisitSession{}
		}
		visits[i].Sessions = sessions
	}
	if counts == nil {
		counts = []models.StepTransitionCount{}
	}
	if visits == nil {
		visits = []models.StepVisitSummary{}
	}
	return &models.TaskTransitionSummary{Counts: counts, Visits: visits}, nil
}

// stepVisitSessions groups the route ledger's destination sessions by
// destination step, deduplicated by session and ordered oldest to newest. A
// session that re-entered a step contributes one entry at its first arrival.
func stepVisitSessions(routes []*models.TaskSessionRoute) map[string][]models.StepVisitSession {
	byStep := make(map[string][]models.StepVisitSession)
	seen := make(map[string]map[string]bool)
	for _, route := range routes {
		if route.DestinationSessionID == nil || route.DestinationWorkflowStepID == "" {
			continue
		}
		step := route.DestinationWorkflowStepID
		session := *route.DestinationSessionID
		if seen[step] == nil {
			seen[step] = make(map[string]bool)
		}
		if seen[step][session] {
			continue
		}
		seen[step][session] = true
		byStep[step] = append(byStep[step], models.StepVisitSession{
			SessionID:      session,
			AgentProfileID: route.AgentProfileID,
			TransitionID:   route.WorkflowStepTransitionID,
			OccurredAt:     route.CreatedAt,
		})
	}
	for step := range byStep {
		sort.SliceStable(byStep[step], func(i, j int) bool {
			return byStep[step][i].OccurredAt.Before(byStep[step][j].OccurredAt)
		})
	}
	return byStep
}
