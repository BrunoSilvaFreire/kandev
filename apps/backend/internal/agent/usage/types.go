// Package usage provides subscription utilization tracking for agent providers.
// It fetches utilization data from provider APIs (Anthropic, OpenAI) for agents
// authenticated via OAuth/subscription credentials rather than API keys.
package usage

import (
	"context"
	"time"
)

// BillingType identifies how an agent is billed.
type BillingType string

const (
	// BillingTypeAPIKey means the agent uses an API key with per-token billing.
	BillingTypeAPIKey BillingType = "api_key"
	// BillingTypeSubscription means the agent uses OAuth/subscription credentials.
	BillingTypeSubscription BillingType = "subscription"
)

// UtilizationWindow represents one rate-limit window's utilization.
type UtilizationWindow struct {
	Label          string    `json:"label"`           // e.g. "5-hour", "7-day"
	UtilizationPct float64   `json:"utilization_pct"` // 0–100
	ResetAt        time.Time `json:"reset_at"`
	// Model scopes the window to one model (for example a Gemini Code Assist
	// bucket). Empty means the window applies to every model. See RemainingPct.
	Model string `json:"model,omitempty"`
}

// ProviderUsage is the full utilization response for one provider credential.
type ProviderUsage struct {
	Provider     string              `json:"provider"`       // "anthropic", "openai"
	Plan         string              `json:"plan,omitempty"` // e.g. "max", "pro", "plus", "free"
	Windows      []UtilizationWindow `json:"windows"`
	Balances     []Balance           `json:"balances,omitempty"`
	Subscription *Subscription       `json:"subscription,omitempty"`
	FetchedAt    time.Time           `json:"fetched_at"`
}

// Balance is a prepaid amount left on the account. It is informational and
// never feeds RemainingPct.
type Balance struct {
	Label  string  `json:"label"` // e.g. "aip", "junp", "prepaid"
	Amount float64 `json:"amount"`
	Unit   string  `json:"unit"` // "CREDITS", "USD"
}

// Subscription is the provider-reported plan state. Informational only.
type Subscription struct {
	Status       string    `json:"status"` // closed set: active | canceling | renewal_pending | inactive
	Plan         string    `json:"plan,omitempty"`
	PeriodEndsAt time.Time `json:"period_ends_at,omitempty"`
	// UsesBalance reports that the provider keeps serving from prepaid
	// balance after the plan meters are exhausted.
	UsesBalance bool `json:"uses_balance"`
}

// ProviderUsageClient fetches live utilization from a provider API.
type ProviderUsageClient interface {
	FetchUsage(ctx context.Context) (*ProviderUsage, error)
}
