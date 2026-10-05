// Package providerusage owns durable provider-usage history: measured quota
// observations, token activity, limit hits, the resumable history index, and
// the estimates derived from them. Live provider fetches stay in
// internal/agent/usage; this package only records and serves their results.
package providerusage

import "time"

// Observation kinds.
const (
	KindMeasured = "measured"
	KindTokens   = "tokens"
	KindLimitHit = "limit_hit"
)

// Observation sources.
const (
	SourceKandev       = "kandev"
	SourceClaudeLocal  = "claude_local"
	SourceCodexLocal   = "codex_local"
	SourceAgyLocal     = "antigravity_local"
	SourceRoutingError = "routing_error"
)

// KandevLedgerCursorKey is the fixed cursor key for the task_usage_events
// ledger, which is not a file and so has no path hash.
const KandevLedgerCursorKey = "kandev:task_usage_events"

// Agent type and provider identifiers shared by attribution and the overview.
const (
	AgentTypeClaude       = "claude-acp"
	AgentTypeCodex        = "codex-acp"
	AgentTypeAgy          = "agy-acp"
	AgentTypeAntigravity  = "antigravity-acp"
	AgentTypeGemini       = "gemini"
	AgentTypeJunie        = "junie-acp"
	ProviderAnthropic     = "anthropic"
	ProviderOpenAI        = "openai"
	ProviderAntigravity   = "antigravity"
	ProviderGoogle        = "google"
	ProviderOpenCodeGo    = "opencode-go"
	ProviderJunie         = "junie"
	ProviderUnknownPrefix = "agent:"
	AccountProfilePrefix  = "profile:"
)

// QuotaUnavailableReason is a closed set explaining why an account has no live
// quota source. The frontend maps each value to translated copy.
const (
	QuotaUnavailableAPIKeyBilling      = "api_key_billing"
	QuotaUnavailableNoProviderEndpoint = "no_provider_endpoint"
	QuotaUnavailableProfileMissing     = "profile_missing"
	QuotaUnavailableCredentialsMissing = "credentials_missing"
	QuotaUnavailableCredentialsExpired = "credentials_expired"
)

// Observation is one recorded data point. Pointer fields are nil when the
// kind does not carry that measurement.
type Observation struct {
	AccountKey     string
	Provider       string
	WindowLabel    string
	Kind           string
	UtilizationPct *float64
	WeightedTokens *int64
	Model          string
	ResetAt        time.Time
	ObservedAt     time.Time
	Source         string
}

// FileCursor tracks resumable index progress for one file (or the fixed
// Kandev ledger key).
type FileCursor struct {
	PathHash   string
	Source     string
	Size       int64
	Mtime      time.Time
	ByteOffset int64
	UpdatedAt  time.Time
}

// Source describes one history source's consent state.
type Source struct {
	Source    string
	Enabled   bool
	ChangedAt time.Time
}

// ToggleableSources lists the local sources that a PUT may change. The
// always-on Kandev source is deliberately absent.
var ToggleableSources = []string{SourceClaudeLocal, SourceCodexLocal, SourceAgyLocal}

// SourceEnabled reports whether a source may be read. Kandev is always on.
func SourceEnabled(sources map[string]Source, source string) bool {
	if source == SourceKandev {
		return true
	}
	return sources[source].Enabled
}
