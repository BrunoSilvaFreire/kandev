package backendapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	agentregistry "github.com/kandev/kandev/internal/agent/registry"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/providerusage"
)

// buildProviderUsageService wires the durable provider-usage history store,
// the history index job, and the recording service over the shared DB pool.
func buildProviderUsageService(dbPool *db.Pool, adapter *usageProviderAdapter, profiles providerusage.ProfileSource, home string, log *logger.Logger) (*providerusage.Service, error) {
	repo, err := providerusage.New(dbPool.Writer(), dbPool.Reader())
	if err != nil {
		return nil, err
	}
	accounts := providerusage.HostAccounts(home)
	indexer := providerusage.NewDefaultIndexer(repo, repo, adapter, home)
	return providerusage.NewService(repo, adapter, profiles, accounts, indexer, log), nil
}

// buildProviderUsageServiceForServices builds the provider-usage service from
// route wiring when the shared usage adapter is present. It returns (nil, nil)
// when no adapter is wired, so the caller can leave the Usage routes
// unregistered instead of failing startup.
func buildProviderUsageServiceForServices(
	services *Services,
	dbPool *db.Pool,
	repos *Repositories,
	home string,
	log *logger.Logger,
) (*providerusage.Service, error) {
	if services == nil || services.UsageAdapter == nil {
		return nil, nil
	}
	return buildProviderUsageService(dbPool, services.UsageAdapter, repos.AgentSettings, home, log)
}

// mockUsageTagPrefix marks an E2E-only profile tag carrying a deterministic
// provider utilization percentage (for example "mock-quota-10" means 10%
// utilization, 90% remaining). It is read only under the e2e mock profile so
// production telemetry can never be faked.
const mockUsageTagPrefix = "mock-quota-"

// openCodeACPAgentType is the agent registry type name for the OpenCode ACP
// agent. Its opencode-go/* models expose account-wide Go meters.
const openCodeACPAgentType = "opencode-acp"

func e2eMockUsageEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("KANDEV_E2E_MOCK"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func mockUsageFromTags(profile *settingsmodels.AgentProfile) (*agentusage.ProviderUsage, bool) {
	if profile == nil {
		return nil, false
	}
	for _, tag := range profile.Tags {
		if !strings.HasPrefix(tag, mockUsageTagPrefix) {
			continue
		}
		pct, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(tag, mockUsageTagPrefix)), 64)
		if err != nil {
			continue
		}
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		return &agentusage.ProviderUsage{
			Provider: "mock",
			Windows: []agentusage.UtilizationWindow{{
				Label:          "5-hour",
				UtilizationPct: pct,
				ResetAt:        time.Now().UTC().Add(time.Hour),
			}},
		}, true
	}
	return nil, false
}

// usageProviderAdapter implements officeagents.UsageProvider by:
//  1. Looking up the agent profile by ID from the settings store.
//  2. Looking up the agent type from the registry to get its billing type.
//  3. Delegating to the UsageService with the appropriate client registered.
type usageProviderAdapter struct {
	svc           *agentusage.UsageService
	settingsStore settingsstore.Repository
	agentRegistry *agentregistry.Registry
	proxyResolver usageProxyResolver
	limitHits     interface {
		LatestLimitHit(context.Context, string) (time.Time, time.Time, bool, error)
	}
	opencodeCookie func(context.Context) (string, error)
	junieAPIKey    func(context.Context) (string, error)
}

// GetUsage implements officeagents.UsageProvider.
func (a *usageProviderAdapter) GetUsage(ctx context.Context, profileID string) (*agentusage.ProviderUsage, error) {
	profile, err := a.settingsStore.GetAgentProfile(ctx, profileID)
	if err != nil {
		// A profile lookup failure is unavailable, not unknown: the consumer
		// can distinguish "no telemetry" from "could not resolve the profile"
		// and keeps the three-state contract.
		return nil, fmt.Errorf("load agent profile %s for usage: %w", profileID, err)
	}
	if e2eMockUsageEnabled() {
		if usage, ok := mockUsageFromTags(profile); ok {
			return usage, nil
		}
		return nil, nil
	}
	agentType, err := a.agentType(ctx, profile)
	if err != nil {
		return nil, err
	}
	ag, ok := a.agentRegistry.Get(agentType)
	if !ok {
		return nil, nil
	}
	if a.proxyResolver != nil {
		if client, cacheKey, ok := a.proxyResolver.Resolve(profile, agentType); ok {
			a.svc.Register(profileID, client, cacheKey)
			return a.getUsageWithLimitHit(ctx, profileID)
		}
	}
	if client, key, ok := a.specialClient(profile, agentType); ok {
		a.svc.Register(profileID, client, key)
		return a.getUsageWithLimitHit(ctx, profileID)
	}
	if ag.BillingType() != agentusage.BillingTypeSubscription {
		return nil, nil
	}
	// Ensure client is registered for this profile.
	a.ensureRegistered(profileID, agentType)
	return a.getUsageWithLimitHit(ctx, profileID)
}

func (a *usageProviderAdapter) getUsageWithLimitHit(ctx context.Context, profileID string) (*agentusage.ProviderUsage, error) {
	usage, err := a.svc.GetUsage(ctx, profileID)
	return a.withLimitHit(ctx, profileID, usage, err)
}

// ProfileModel lets settings utilization apply model-scoped provider windows.
func (a *usageProviderAdapter) ProfileModel(ctx context.Context, profileID string) (string, error) {
	profile, err := a.settingsStore.GetAgentProfile(ctx, profileID)
	if err != nil || profile == nil {
		return "", err
	}
	return profile.Model, nil
}

func (a *usageProviderAdapter) withLimitHit(ctx context.Context, profileID string, usage *agentusage.ProviderUsage, fetchErr error) (*agentusage.ProviderUsage, error) {
	if a.limitHits == nil {
		return usage, fetchErr
	}
	key, ok := a.svc.CacheKeyFor(profileID)
	if !ok {
		return usage, fetchErr
	}
	hitAt, resetAt, found, err := a.limitHits.LatestLimitHit(ctx, key)
	if err != nil || !found || resetAt.IsZero() || !resetAt.After(time.Now()) {
		return usage, fetchErr
	}
	if usage != nil && usage.FetchedAt.After(hitAt) {
		return usage, fetchErr
	}
	// Copy before appending: usage may be the pointer stored in the shared
	// UsageCache, and every caller within the TTL would otherwise grow that one
	// slice (piling up synthetic windows and racing concurrent readers).
	merged := agentusage.ProviderUsage{}
	if usage != nil {
		merged = *usage
	}
	merged.Windows = append(append([]agentusage.UtilizationWindow(nil), merged.Windows...),
		agentusage.UtilizationWindow{Label: "limit", UtilizationPct: 100, ResetAt: resetAt})
	return &merged, nil
}

// agentType resolves a profile's agent *type* name (for example "codex-acp").
// profile.AgentID is the `agents.id` UUID that the settings store and the
// runtime profile resolver use, while the agent registry, the billing type,
// and the usage-client switch are all keyed by the type name. Resolving through
// the settings store is the only correct mapping between the two.
func (a *usageProviderAdapter) agentType(ctx context.Context, profile *settingsmodels.AgentProfile) (string, error) {
	if profile == nil {
		return "", fmt.Errorf("resolve agent type: profile is nil")
	}
	if a.settingsStore == nil {
		return "", fmt.Errorf("resolve agent type for profile %s: settings store unavailable", profile.ID)
	}
	agent, err := a.settingsStore.GetAgent(ctx, profile.AgentID)
	if err != nil {
		return "", fmt.Errorf("resolve agent %s for profile %s: %w", profile.AgentID, profile.ID, err)
	}
	if agent == nil {
		return "", fmt.Errorf("resolve agent %s for profile %s: not found", profile.AgentID, profile.ID)
	}
	return agent.Name, nil
}

// ensureRegistered creates and registers a usage client for the profile if not already registered.
func (a *usageProviderAdapter) ensureRegistered(profileID, agentName string) {
	// We rely on the fact that GetUsage returns nil,nil for unregistered profiles,
	// so we can call Register without causing duplicate cache entries — the cache key
	// is credential-path-based, not profileID-based, so two profiles with the same
	// credentials share one cache entry.
	//
	// `home` is required for both branches — bail rather than registering a
	// client pointed at a relative ".claude/.credentials.json" / ".codex/auth.json"
	// that silently misses the real file (common in containers or when HOME is
	// unset). Without this guard the consumer sees BillingTypeAPIKey forever.
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	switch agentName {
	case claudeACPAgentID:
		credPath := filepath.Join(home, ".claude", ".credentials.json")
		client := agentusage.NewClaudeUsageClientWithPath(credPath)
		key := agentusage.CacheKey("anthropic", credPath)
		a.svc.Register(profileID, client, key)
	case "codex-acp":
		// Path must match codex_acp.go's SourceFiles / Runtime mounts —
		// the real Codex CLI persists OAuth tokens at ~/.codex/auth.json,
		// not the earlier XDG-style ~/.config/codex/ guess.
		authPath := filepath.Join(home, ".codex", "auth.json")
		client := agentusage.NewCodexUsageClientWithPath(authPath)
		key := agentusage.CacheKey("openai", authPath)
		a.svc.Register(profileID, client, key)
	case "agy-acp":
		// Antigravity exposes its quota through the running local language
		// server over loopback; there is no credential file. The account key
		// matches the one the history estimator uses for local Antigravity.
		client := agentusage.NewAntigravityUsageClient()
		a.svc.Register(profileID, client, agentusage.AntigravityCacheKey())
	case "gemini":
		// Gemini Code Assist quota reads the token the Gemini CLI stored. The
		// client is read-only and shares one cache entry per credential path.
		credPath := filepath.Join(home, ".gemini", "oauth_creds.json")
		client := agentusage.NewGeminiUsageClientWithPath(credPath)
		key := agentusage.CacheKey("google", credPath)
		a.svc.Register(profileID, client, key)
	}
}

// specialClient returns the account-wide live client for a profile whose
// provider needs credentials outside the normal subscription gate (OpenCode Go
// meters, Junie balances). The client is created even when no credential is
// set: it returns ErrCredentialsMissing without an HTTP call, which is exactly
// the case QuotaUnavailableReason reports.
func (a *usageProviderAdapter) specialClient(profile *settingsmodels.AgentProfile, agentType string) (agentusage.ProviderUsageClient, string, bool) {
	if agentType == openCodeACPAgentType && strings.HasPrefix(profile.Model, "opencode-go/") {
		return agentusage.NewOpenCodeGoUsageClient(a.opencodeCookie), agentusage.OpenCodeGoCacheKey(), true
	}
	if agentType == providerusage.AgentTypeJunie {
		credPath := junieCredentialPath()
		if credPath == "" {
			return nil, "", false
		}
		return agentusage.NewJunieUsageClient(a.junieAPIKey, credPath), agentusage.JunieCacheKey(), true
	}
	return nil, "", false
}

// junieCredentialPath is the Junie CLI login file used as the fallback token
// source, or "" when the home directory cannot be resolved.
func junieCredentialPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".junie", "secure_credentials.json")
}

// CacheKeyFor returns the live cache key registered for a profile.
func (a *usageProviderAdapter) CacheKeyFor(profileID string) (string, bool) {
	return a.svc.CacheKeyFor(profileID)
}

// QuotaCredential returns the credential hint for a profile whose provider
// needs an externally supplied secret, or nil when none is needed.
func (a *usageProviderAdapter) QuotaCredential(ctx context.Context, profileID string) *providerusage.CredentialHint {
	profile, err := a.settingsStore.GetAgentProfile(ctx, profileID)
	if err != nil || profile == nil {
		return nil
	}
	agentType, err := a.agentType(ctx, profile)
	if err != nil {
		return nil
	}
	if agentType == openCodeACPAgentType && strings.HasPrefix(profile.Model, "opencode-go/") {
		return a.openCodeGoHint(ctx)
	}
	if agentType == providerusage.AgentTypeJunie {
		return a.junieHint(ctx)
	}
	return nil
}

// CredentialHints lists every credential Kandev can store. The adapter owns the
// descriptors so the settings page does not need the full overview.
func (a *usageProviderAdapter) CredentialHints(ctx context.Context) []providerusage.CredentialHint {
	return []providerusage.CredentialHint{*a.openCodeGoHint(ctx), *a.junieHint(ctx)}
}

func (a *usageProviderAdapter) openCodeGoHint(ctx context.Context) *providerusage.CredentialHint {
	return &providerusage.CredentialHint{
		Kind:       providerusage.CredentialKindOpenCodeConsoleCookie,
		AgentType:  openCodeACPAgentType,
		SecretName: agentusage.OpenCodeGoCookieSecretName,
		Configured: secretConfigured(ctx, a.opencodeCookie),
	}
}

func (a *usageProviderAdapter) junieHint(ctx context.Context) *providerusage.CredentialHint {
	return &providerusage.CredentialHint{
		Kind:       providerusage.CredentialKindJunieAPIKey,
		AgentType:  providerusage.AgentTypeJunie,
		SecretName: agentusage.JunieAPIKeySecretName,
		Configured: secretConfigured(ctx, a.junieAPIKey),
	}
}

// InvalidateQuotaCache drops cached live fetches after a credential change.
func (a *usageProviderAdapter) InvalidateQuotaCache() {
	a.svc.InvalidateAll()
}

func secretConfigured(ctx context.Context, get func(context.Context) (string, error)) bool {
	if get == nil {
		return false
	}
	value, err := get(ctx)
	return err == nil && strings.TrimSpace(value) != ""
}

// QuotaUnavailableReason mirrors the skip branches of GetUsage so the overview
// can explain an account with no measured telemetry. It returns "" when a live
// quota source exists.
func (a *usageProviderAdapter) QuotaUnavailableReason(ctx context.Context, profileID string) string {
	profile, err := a.settingsStore.GetAgentProfile(ctx, profileID)
	if err != nil || profile == nil {
		return providerusage.QuotaUnavailableProfileMissing
	}
	agentType, err := a.agentType(ctx, profile)
	if err != nil {
		return providerusage.QuotaUnavailableNoProviderEndpoint
	}
	ag, ok := a.agentRegistry.Get(agentType)
	if !ok {
		return providerusage.QuotaUnavailableNoProviderEndpoint
	}
	proxyResolved := false
	if a.proxyResolver != nil {
		_, _, proxyResolved = a.proxyResolver.Resolve(profile, agentType)
	}
	if agentType == openCodeACPAgentType && strings.HasPrefix(profile.Model, "opencode-go/") {
		return a.openCodeGoReason(ctx)
	}
	if agentType == providerusage.AgentTypeJunie {
		return a.junieReason(ctx)
	}
	return quotaUnavailableReasonFor(true, true, ag.BillingType() == agentusage.BillingTypeSubscription, proxyResolved)
}

// openCodeGoReason explains an OpenCode Go account with no measured telemetry.
// A missing cookie and an expired session both surface as actionable reasons.
func (a *usageProviderAdapter) openCodeGoReason(ctx context.Context) string {
	if a.opencodeCookie == nil {
		return providerusage.QuotaUnavailableCredentialsMissing
	}
	cookie, cookieErr := a.opencodeCookie(ctx)
	if cookieErr != nil || strings.TrimSpace(cookie) == "" {
		return providerusage.QuotaUnavailableCredentialsMissing
	}
	if errors.Is(a.svc.LastError(agentusage.OpenCodeGoCacheKey()), agentusage.ErrCredentialsExpired) {
		return providerusage.QuotaUnavailableCredentialsExpired
	}
	return ""
}

// junieReason explains a Junie account with no measured telemetry. The durable
// API-key secret wins; otherwise the best-effort local login token must still
// be unexpired.
func (a *usageProviderAdapter) junieReason(ctx context.Context) string {
	if a.junieAPIKey != nil {
		if key, err := a.junieAPIKey(ctx); err == nil && strings.TrimSpace(key) != "" {
			return ""
		}
	}
	credPath := junieCredentialPath()
	if credPath == "" {
		return providerusage.QuotaUnavailableCredentialsMissing
	}
	present, expired := agentusage.JunieLocalCredentialState(credPath)
	if !present {
		return providerusage.QuotaUnavailableCredentialsMissing
	}
	if expired {
		return providerusage.QuotaUnavailableCredentialsExpired
	}
	return ""
}

// quotaUnavailableReasonFor is the pure classification behind
// QuotaUnavailableReason, kept separate so every skip branch is testable
// without a real registry or resolver.
func quotaUnavailableReasonFor(profileFound, agentFound, subscription, proxyResolved bool) string {
	switch {
	case !profileFound:
		return providerusage.QuotaUnavailableProfileMissing
	case !agentFound:
		return providerusage.QuotaUnavailableNoProviderEndpoint
	case proxyResolved:
		return ""
	case !subscription:
		return providerusage.QuotaUnavailableAPIKeyBilling
	default:
		return ""
	}
}

// EnsureCacheKey resolves the live cache key for a profile, registering a
// client on demand the same way GetUsage does. It returns false when the
// profile has no live quota source (unknown agent or API-key billing).
func (a *usageProviderAdapter) EnsureCacheKey(ctx context.Context, profileID string) (string, bool) {
	if key, ok := a.svc.CacheKeyFor(profileID); ok {
		return key, true
	}
	profile, err := a.settingsStore.GetAgentProfile(ctx, profileID)
	if err != nil || profile == nil {
		return "", false
	}
	agentType, err := a.agentType(ctx, profile)
	if err != nil {
		return "", false
	}
	if a.proxyResolver != nil {
		if client, cacheKey, ok := a.proxyResolver.Resolve(profile, agentType); ok {
			a.svc.Register(profileID, client, cacheKey)
			return cacheKey, true
		}
	}
	if client, key, ok := a.specialClient(profile, agentType); ok {
		a.svc.Register(profileID, client, key)
		return key, true
	}
	ag, ok := a.agentRegistry.Get(agentType)
	if !ok || ag.BillingType() != agentusage.BillingTypeSubscription {
		return "", false
	}
	a.ensureRegistered(profileID, agentType)
	return a.svc.CacheKeyFor(profileID)
}

// newUsageProviderAdapter creates an adapter and returns it.
func newUsageProviderAdapter(
	settingsStore settingsstore.Repository,
	agentRegistry *agentregistry.Registry,
) *usageProviderAdapter {
	return &usageProviderAdapter{
		svc:           agentusage.NewUsageService(),
		settingsStore: settingsStore,
		agentRegistry: agentRegistry,
		proxyResolver: defaultUsageProxyResolver(),
	}
}
