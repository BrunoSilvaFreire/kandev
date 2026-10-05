package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
)

// ErrTaskSearchUnsupported is returned when the configured message repository
// has no task-wide search capability. It is not a silent empty result.
var ErrTaskSearchUnsupported = errors.New("task message search is not supported")

// taskMessageSearcher is the optional repository capability backing task-wide
// message search. Keeping it optional lets repository fakes that model only the
// session-scoped surface stay unchanged.
type taskMessageSearcher interface {
	SearchTaskMessages(
		ctx context.Context, taskID string, opts models.SearchTaskMessagesOptions,
	) ([]*models.TaskMessageSearchHit, bool, error)
}

// SearchTaskMessages returns one keyset page of task messages matching the
// query, in the global active-session-first order. `cursor` is nil for the
// first page; callers decode/validate the opaque cursor before calling.
func (s *Service) SearchTaskMessages(
	ctx context.Context,
	taskID, activeSessionID, query string,
	limit int,
	cursor *models.SearchTaskMessagesCursor,
) ([]*models.TaskMessageSearchHit, bool, error) {
	if err := s.AuthorizeTaskAccess(ctx, taskID); err != nil {
		return nil, false, err
	}
	searcher, ok := s.messages.(taskMessageSearcher)
	if !ok {
		return nil, false, ErrTaskSearchUnsupported
	}
	hits, hasMore, err := searcher.SearchTaskMessages(ctx, taskID, models.SearchTaskMessagesOptions{
		Query:           query,
		ActiveSessionID: activeSessionID,
		Limit:           limit,
		Cursor:          cursor,
	})
	if err != nil || len(hits) == 0 {
		return hits, hasMore, err
	}
	s.enrichTaskSearchSessions(ctx, taskID, hits)
	return hits, hasMore, nil
}

// enrichTaskSearchSessions attaches each hit's owning session for display.
// Enrichment is best-effort: a lookup failure leaves Session nil rather than
// failing the search, because the hit's own fields are still authoritative.
func (s *Service) enrichTaskSearchSessions(
	ctx context.Context,
	taskID string,
	hits []*models.TaskMessageSearchHit,
) {
	sessionsByTask, err := s.sessions.BatchGetSessionsByTaskIDs(ctx, []string{taskID})
	if err != nil {
		return
	}
	byID := make(map[string]*models.TaskSession)
	for _, session := range sessionsByTask[taskID] {
		byID[session.ID] = session
	}
	for _, hit := range hits {
		if hit.Message == nil {
			continue
		}
		if session, ok := byID[hit.Message.TaskSessionID]; ok {
			hit.Session = session
		}
	}
}
