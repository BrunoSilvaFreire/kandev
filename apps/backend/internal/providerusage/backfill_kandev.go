package providerusage

import (
	"context"
	"time"
)

// LedgerEvent is one row of the task_usage_events ledger, narrowed to the
// fields backfill needs.
type LedgerEvent struct {
	ID                int64
	AgentProfileID    string
	AgentType         string
	Model             string
	Provider          string
	TokensIn          int64
	TokensOut         int64
	TokensCachedRead  int64
	TokensCachedWrite int64
	OccurredAt        time.Time
}

// LedgerReader pages the task_usage_events ledger.
type LedgerReader interface {
	ListUsageEventsAfter(ctx context.Context, afterID int64, limit int) ([]LedgerEvent, error)
}

// AccountResolver maps a profile to its live account key, registering on
// demand. It is satisfied by the usage adapter.
type AccountResolver interface {
	EnsureCacheKey(ctx context.Context, profileID string) (string, bool)
}

// ListUsageEventsAfter implements LedgerReader over task_usage_events.
func (r *Repository) ListUsageEventsAfter(ctx context.Context, afterID int64, limit int) ([]LedgerEvent, error) {
	query := r.reader.Rebind(`SELECT id, agent_profile_id, agent_type, model, provider,
		tokens_in, COALESCE(tokens_out, 0), COALESCE(tokens_cached_read, 0),
		COALESCE(tokens_cached_write, 0), occurred_at
		FROM task_usage_events WHERE id > ? ORDER BY id LIMIT ?`)
	rows, err := r.reader.QueryContext(ctx, query, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var events []LedgerEvent
	for rows.Next() {
		var event LedgerEvent
		if err := rows.Scan(&event.ID, &event.AgentProfileID, &event.AgentType, &event.Model,
			&event.Provider, &event.TokensIn, &event.TokensOut, &event.TokensCachedRead,
			&event.TokensCachedWrite, &event.OccurredAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// attributor maps a ledger row to an account and provider. Profile identity
// wins; otherwise the agent type decides. Resolved keys are memoized for one
// run so a page of up to 5000 rows from one profile is not 5000 DB reads.
type attributor struct {
	accounts Accounts
	resolver AccountResolver
	cache    map[string]string
	miss     map[string]bool
}

func (a *attributor) resetRunCache() {
	a.cache = make(map[string]string)
	a.miss = make(map[string]bool)
}

func (a *attributor) resolvedAccountKey(ctx context.Context, profileID string) (string, bool) {
	if profileID == "" || a.resolver == nil {
		return "", false
	}
	if a.miss == nil {
		a.resetRunCache()
	}
	if key, ok := a.cache[profileID]; ok {
		return key, true
	}
	if a.miss[profileID] {
		return "", false
	}
	if key, ok := a.resolver.EnsureCacheKey(ctx, profileID); ok {
		a.cache[profileID] = key
		return key, true
	}
	a.miss[profileID] = true
	return "", false
}

func (a *attributor) accountKey(ctx context.Context, event LedgerEvent) string {
	if key, ok := a.resolvedAccountKey(ctx, event.AgentProfileID); ok {
		return key
	}
	switch event.AgentType {
	case AgentTypeClaude:
		return a.accounts.Anthropic
	case AgentTypeCodex:
		return a.accounts.OpenAI
	case AgentTypeAgy, AgentTypeAntigravity:
		return a.accounts.Antigravity
	default:
		return ProviderUnknownPrefix + event.AgentType
	}
}

func ledgerProvider(agentType, provider string) string {
	switch agentType {
	case AgentTypeClaude:
		return ProviderAnthropic
	case AgentTypeCodex:
		return ProviderOpenAI
	case AgentTypeAgy, AgentTypeAntigravity:
		return ProviderAntigravity
	}
	if provider != "" {
		return provider
	}
	return agentType
}

// kandevObservations converts a batch of ledger rows to hourly token buckets.
func (a *attributor) kandevObservations(ctx context.Context, events []LedgerEvent) []Observation {
	observations := make([]Observation, 0, len(events))
	for i := range events {
		event := &events[i]
		weighted := event.TokensIn + event.TokensOut + event.TokensCachedWrite +
			int64(cacheReadWeight*float64(event.TokensCachedRead))
		observations = append(observations, Observation{
			AccountKey:     a.accountKey(ctx, *event),
			Provider:       ledgerProvider(event.AgentType, event.Provider),
			WindowLabel:    "tokens",
			Kind:           KindTokens,
			WeightedTokens: &weighted,
			Model:          event.Model,
			ObservedAt:     event.OccurredAt.UTC().Truncate(time.Hour),
			Source:         SourceKandev,
		})
	}
	return observations
}
