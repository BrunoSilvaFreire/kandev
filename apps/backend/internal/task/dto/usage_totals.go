package dto

import (
	"strconv"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TaskUsageTotalsScope identifies whether a TaskUsageTotalsDTO aggregates an
// entire task or a single session within it (docs/specs/task-cost-ledger/
// spec.md API surface).
type TaskUsageTotalsScope string

const (
	TaskUsageTotalsScopeTask    TaskUsageTotalsScope = "task"
	TaskUsageTotalsScopeSession TaskUsageTotalsScope = "session"
	// TaskUsageTotalsScopeGroup is the scope of one finest-grain breakdown
	// group (session, agent profile, agent type, model, provider).
	TaskUsageTotalsScopeGroup TaskUsageTotalsScope = "group"
)

// TaskUsageTotalsDTO is the exact JSON shape both usage-totals HTTP routes
// return (AC-18, AC-19, AC-20). FirstEventAt/LastEventAt serialize to JSON
// null, never omitted, when the scope has no contributing rows.
type TaskUsageTotalsDTO struct {
	Scope                TaskUsageTotalsScope `json:"scope"`
	ScopeID              string               `json:"scope_id"`
	TokensIn             int64                `json:"tokens_in"`
	TokensCachedRead     int64                `json:"tokens_cached_read"`
	TokensCachedWrite    int64                `json:"tokens_cached_write"`
	TokensOut            int64                `json:"tokens_out"`
	TokensThought        int64                `json:"tokens_thought"`
	TokensTotal          int64                `json:"tokens_total"`
	CostSubcents         int64                `json:"cost_subcents"`
	CostSubcentsDecimal  string               `json:"cost_subcents_decimal"`
	EventCount           int64                `json:"event_count"`
	EstimatedEventCount  int64                `json:"estimated_event_count"`
	UnpricedEventCount   int64                `json:"unpriced_event_count"`
	OutputTokensComplete bool                 `json:"output_tokens_complete"`
	FirstEventAt         *time.Time           `json:"first_event_at"`
	LastEventAt          *time.Time           `json:"last_event_at"`
}

// TaskUsageBreakdownDTO is the JSON shape of GET /tasks/:id/usage/breakdown:
// the task total plus the finest-grain groups the client rolls up into the
// per-agent, per-model and per-session views. Groups is always a JSON array
// (never null) so an empty ledger serializes as `[]`.
type TaskUsageBreakdownDTO struct {
	TaskID string              `json:"task_id"`
	Task   TaskUsageTotalsDTO  `json:"task"`
	Groups []TaskUsageGroupDTO `json:"groups"`
}

// TaskUsageGroupDTO is one finest-grain group. SessionID serializes to JSON
// null (never omitted) for rows whose session was deleted.
type TaskUsageGroupDTO struct {
	SessionID      *string            `json:"session_id"`
	AgentProfileID string             `json:"agent_profile_id"`
	AgentType      string             `json:"agent_type"`
	Model          string             `json:"model"`
	Provider       string             `json:"provider"`
	Totals         TaskUsageTotalsDTO `json:"totals"`
}

// ToTaskUsageBreakdownDTO combines the task total and the grouped rows the
// repository returned into the breakdown response, reusing
// ToTaskUsageTotalsDTO for both scopes.
func ToTaskUsageBreakdownDTO(
	taskID string,
	taskTotals *models.TaskUsageTotals,
	groups []*models.TaskUsageTotalsGroup,
) TaskUsageBreakdownDTO {
	out := TaskUsageBreakdownDTO{
		TaskID: taskID,
		Task:   ToTaskUsageTotalsDTO(TaskUsageTotalsScopeTask, taskID, taskTotals),
		Groups: make([]TaskUsageGroupDTO, 0, len(groups)),
	}
	for _, group := range groups {
		out.Groups = append(out.Groups, TaskUsageGroupDTO{
			SessionID:      group.SessionID,
			AgentProfileID: group.AgentProfileID,
			AgentType:      group.AgentType,
			Model:          group.Model,
			Provider:       group.Provider,
			Totals:         ToTaskUsageTotalsDTO(TaskUsageTotalsScopeGroup, "", &group.Totals),
		})
	}
	return out
}

// ToTaskUsageTotalsDTO combines a repository-layer aggregate with the scope
// and scope ID the HTTP route resolved it for.
func ToTaskUsageTotalsDTO(scope TaskUsageTotalsScope, scopeID string, totals *models.TaskUsageTotals) TaskUsageTotalsDTO {
	return TaskUsageTotalsDTO{
		Scope:                scope,
		ScopeID:              scopeID,
		TokensIn:             totals.TokensIn,
		TokensCachedRead:     totals.TokensCachedRead,
		TokensCachedWrite:    totals.TokensCachedWrite,
		TokensOut:            totals.TokensOut,
		TokensThought:        totals.TokensThought,
		TokensTotal:          totals.TokensTotal,
		CostSubcents:         totals.CostSubcents,
		CostSubcentsDecimal:  strconv.FormatInt(totals.CostSubcents, 10),
		EventCount:           totals.EventCount,
		EstimatedEventCount:  totals.EstimatedEventCount,
		UnpricedEventCount:   totals.UnpricedEventCount,
		OutputTokensComplete: totals.OutputTokensComplete,
		FirstEventAt:         totals.FirstEventAt,
		LastEventAt:          totals.LastEventAt,
	}
}
