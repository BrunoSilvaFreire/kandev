package providerusage

import (
	"context"
	"strings"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// SourceLive marks measured observations recorded from a live provider fetch.
const SourceLive = "live"

// recordTimeout bounds a recorder or limit-hit insert so it never delays the
// caller. Inserts are detached and best-effort.
const recordTimeout = 2 * time.Second

// LiveProvider is the live fetch and account-key surface the overview needs.
type LiveProvider interface {
	GetUsage(ctx context.Context, profileID string) (*agentusage.ProviderUsage, error)
	CacheKeyFor(profileID string) (string, bool)
	EnsureCacheKey(ctx context.Context, profileID string) (string, bool)
	// QuotaUnavailableReason reports the closed-set reason a profile has no
	// live quota source, or "" when one exists.
	QuotaUnavailableReason(ctx context.Context, profileID string) string
	// QuotaCredential names the global secret that enables this profile's live
	// quota, or nil when the provider needs no credential.
	QuotaCredential(ctx context.Context, profileID string) *CredentialHint
	// CredentialHints lists every credential Kandev knows how to store.
	CredentialHints(ctx context.Context) []CredentialHint
	// InvalidateQuotaCache drops cached live fetches so a newly saved
	// credential is reflected on the next read.
	InvalidateQuotaCache()
}

// ProfileSource lists the configured agents and their profiles.
type ProfileSource interface {
	ListAgents(ctx context.Context) ([]*settingsmodels.Agent, error)
	ListAgentProfiles(ctx context.Context, agentID string) ([]*settingsmodels.AgentProfile, error)
}

// Service owns the provider-usage history: live recording, limit hits, the
// index job, and the overview projection.
type Service struct {
	repo     *Repository
	live     LiveProvider
	profiles ProfileSource
	accounts Accounts
	indexer  *Indexer
	log      *logger.Logger
}

// NewService builds the service over the repository, live provider, profile
// source, and indexer.
func NewService(repo *Repository, live LiveProvider, profiles ProfileSource, accounts Accounts, indexer *Indexer, log *logger.Logger) *Service {
	return &Service{repo: repo, live: live, profiles: profiles, accounts: accounts, indexer: indexer, log: log}
}

// Repository exposes the read-only limit-hit query used by the live adapter.
func (s *Service) Repository() *Repository { return s.repo }

// RecordLive is the usage cache's fetch recorder. It stores one measured
// observation per window and never affects the caller.
func (s *Service) RecordLive(key string, usage *agentusage.ProviderUsage) {
	if key == "" || usage == nil || len(usage.Windows) == 0 {
		return
	}
	at := usage.FetchedAt
	if at.IsZero() {
		at = time.Now().UTC()
	}
	observations := make([]Observation, 0, len(usage.Windows))
	for _, window := range usage.Windows {
		pct := window.UtilizationPct
		observations = append(observations, Observation{
			AccountKey:     key,
			Provider:       usage.Provider,
			WindowLabel:    window.Label,
			Kind:           KindMeasured,
			UtilizationPct: &pct,
			ResetAt:        window.ResetAt,
			ObservedAt:     at.UTC(),
			Source:         SourceLive,
		})
	}
	// Detached so a slow DB write never adds latency to the live fetch caller.
	go s.insertDetached(observations, "record live usage failed")
}

func (s *Service) insertDetached(observations []Observation, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), recordTimeout)
	defer cancel()
	if err := s.repo.InsertObservations(ctx, observations); err != nil {
		s.log.Debug(message, zap.Error(err))
	}
}

// RecordLimitHit stores a classified quota or rate-limit failure for the
// owning profile's account.
func (s *Service) RecordLimitHit(profileID, agentID, code string, resetHint time.Time) {
	// Detached: account resolution is a DB read and the insert a write, and
	// neither may add latency to the agent failure boundary.
	go s.recordLimitHit(context.Background(), profileID, agentID, code, resetHint)
}

func (s *Service) recordLimitHit(parent context.Context, profileID, agentID, code string, resetHint time.Time) {
	ctx, cancel := context.WithTimeout(parent, recordTimeout)
	defer cancel()
	accountKey := ""
	if profileID != "" && s.live != nil {
		if key, ok := s.live.EnsureCacheKey(ctx, profileID); ok {
			accountKey = key
		}
	}
	if accountKey == "" {
		accountKey = ProviderUnknownPrefix + agentID
	}
	provider := providerForAgent(agentID)
	if accountKey == agentusage.OpenCodeGoCacheKey() {
		provider = ProviderOpenCodeGo
	}
	observation := Observation{
		AccountKey:  accountKey,
		Provider:    provider,
		WindowLabel: "limit",
		Kind:        KindLimitHit,
		ResetAt:     resetHint,
		ObservedAt:  time.Now().UTC(),
		Source:      SourceRoutingError,
	}
	if err := s.repo.InsertObservations(ctx, []Observation{observation}); err != nil {
		s.log.Debug("record limit hit failed", zap.Error(err), zap.String("code", code))
	}
}

// StartIndex starts the single-flight history index job.
func (s *Service) StartIndex() IndexStatus {
	return s.indexer.Start()
}

// IndexStatus returns the current index progress.
func (s *Service) IndexStatus() IndexStatus {
	return s.indexer.Status()
}

// SetSource changes one local source's consent state. Enabling requests an
// incremental run (a rerun if one is already in flight); disabling stops the
// running job before deleting that source's observations and cursors, so the
// delete cannot be undone by a still-running insert.
func (s *Service) SetSource(ctx context.Context, source string, enabled bool) error {
	if err := s.repo.SetSource(ctx, source, enabled); err != nil {
		return err
	}
	if enabled {
		s.indexer.Start()
		return nil
	}
	// Disabling must not strand the other sources: stop the in-flight pass so
	// the delete cannot race an insert, then restart an incremental pass that
	// skips the now-disabled source.
	restart := s.indexer.StopAndWait(ctx)
	err := s.repo.DeleteSource(ctx, source)
	if restart {
		s.indexer.Start()
	}
	return err
}

// CredentialHints lists the credentials Kandev can store, with Configured
// resolved by the live provider. The secret values are never included.
func (s *Service) CredentialHints(ctx context.Context) []CredentialHint {
	if s.live == nil {
		return nil
	}
	return s.live.CredentialHints(ctx)
}

// InvalidateQuotaCache drops cached live fetches so a newly saved credential
// is reflected on the next overview read.
func (s *Service) InvalidateQuotaCache() {
	if s.live != nil {
		s.live.InvalidateQuotaCache()
	}
}

// Sources returns every source's consent state, including the always-on
// Kandev source.
func (s *Service) Sources(ctx context.Context) (map[string]Source, error) {
	return s.repo.ListSources(ctx)
}

func providerForAgent(agentID string) string {
	switch agentID {
	case AgentTypeClaude:
		return ProviderAnthropic
	case AgentTypeCodex:
		return ProviderOpenAI
	case AgentTypeAgy, AgentTypeAntigravity:
		return ProviderAntigravity
	case AgentTypeGemini:
		return ProviderGoogle
	case AgentTypeJunie:
		return ProviderJunie
	default:
		return agentID
	}
}

func providerForProfile(agentType, model string) string {
	if agentType == "opencode-acp" && strings.HasPrefix(model, "opencode-go/") {
		return ProviderOpenCodeGo
	}
	return providerForAgent(agentType)
}
