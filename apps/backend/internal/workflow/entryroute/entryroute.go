// Package entryroute carries a resolved workflow-entry agent-profile choice
// from the caller that performs an actual step entry down to the task
// repository transaction that writes the step-transition ledger row. It has no
// dependency on the task repository or the orchestrator; the repository reads
// only the already-resolved PendingRoute carried on context, mirroring
// internal/workflow/stepentry and internal/steptelemetry.
package entryroute

import (
	"context"
	"errors"
	"fmt"
)

// ErrNoFrozenProfile is returned when a tag-configured entry has no persisted,
// entry-identity-matching route. A launch must not guess or re-score.
var ErrNoFrozenProfile = errors.New("no frozen agent profile for workflow entry")

// ErrSelectorUnavailable is returned when a tag-configured entry is written
// while no profile selector is wired. Failing closed keeps an unwired
// composition from committing a tagged entry without its frozen route.
var ErrSelectorUnavailable = errors.New("workflow entry profile selector is unavailable")

// TargetKindProfile is the route target kind for a normal tag-selected entry.
const TargetKindProfile = "profile"

type pendingKey struct{}

// PendingRoute is the resolved, not-yet-persisted choice for one workflow
// entry. The transition transaction fills the entry identity and the
// deterministic operation ID from the committed ledger row.
type PendingRoute struct {
	// DestinationStepID must match the transition's destination step; a
	// mismatch (a diverted or raced move) discards the pending route.
	DestinationStepID string
	AgentProfileID    string
	// StartPolicy is the normalized profile-session start policy string.
	StartPolicy string
	// SourceSessionID is the session the entry moved from, when one existed.
	SourceSessionID string
	// Reason is a short, non-sensitive label for observability.
	Reason string
}

// WithPendingRoute attaches a resolved route choice to ctx.
func WithPendingRoute(ctx context.Context, route PendingRoute) context.Context {
	if route.AgentProfileID == "" || route.DestinationStepID == "" {
		return ctx
	}
	return context.WithValue(ctx, pendingKey{}, route)
}

// FromContext reads back a PendingRoute previously attached with
// WithPendingRoute. ok is false when none was attached.
func FromContext(ctx context.Context) (PendingRoute, bool) {
	if ctx == nil {
		return PendingRoute{}, false
	}
	route, ok := ctx.Value(pendingKey{}).(PendingRoute)
	if !ok || route.AgentProfileID == "" || route.DestinationStepID == "" {
		return PendingRoute{}, false
	}
	return route, true
}

// EntryIdentity formats a committed task_step_transitions.id as the durable
// workflow-entry identity. It is the single source of the "entry:%020d" shape.
func EntryIdentity(ledgerID int64) string {
	return fmt.Sprintf("entry:%020d", ledgerID)
}

// OperationID is the deterministic operation identifier for a workflow-session
// route. It stays byte-compatible with the explicit session-target routes.
func OperationID(taskID, stepID, entryIdentity, targetKind, targetID, startPolicy string) string {
	return fmt.Sprintf("workflow-session:%s:%s:%s:%s:%s:%s", taskID, stepID, entryIdentity, targetKind, targetID, startPolicy)
}
