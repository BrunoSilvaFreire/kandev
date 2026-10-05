package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	opencodeGoBaseURL = "https://opencode.ai"
	jsonNullBody      = "null"
)

// OpenCodeGoCookieSecretName is the Kandev global secret holding the OpenCode
// console session cookie. Browser session cookies are never read from disk.
const OpenCodeGoCookieSecretName = "opencode-console-cookie"

// ErrCredentialsMissing means a local, read-only credential source was absent.
var ErrCredentialsMissing = errors.New("provider credentials are missing")

// openCodeGoConsoleSource is the synthetic credential path for the OpenCode Go
// console meters, which are account-wide and have no on-disk credential file.
const openCodeGoConsoleSource = "console"

// OpenCodeGoCacheKey returns the single live cache key shared by every
// opencode-go/* profile. The Go console meters are account-wide, so all Go
// profiles resolve to one account.
func OpenCodeGoCacheKey() string {
	return CacheKey("opencode-go", openCodeGoConsoleSource)
}

// OpenCodeGoUsageClient reads the OpenCode Go console meters. The cookie
// callback owns secret retrieval so this package never knows Kandev secrets.
type OpenCodeGoUsageClient struct {
	cookie     func(context.Context) (string, error)
	baseURL    string
	httpClient *http.Client
	mu         sync.Mutex
	orgID      string
}

func NewOpenCodeGoUsageClient(cookie func(context.Context) (string, error)) *OpenCodeGoUsageClient {
	return &OpenCodeGoUsageClient{cookie: cookie, baseURL: opencodeGoBaseURL, httpClient: &http.Client{Timeout: 10 * time.Second}}
}

func (c *OpenCodeGoUsageClient) FetchUsage(ctx context.Context) (*ProviderUsage, error) {
	if c.cookie == nil {
		return nil, ErrCredentialsMissing
	}
	cookie, err := c.cookie(ctx)
	if err != nil || strings.TrimSpace(cookie) == "" {
		return nil, ErrCredentialsMissing
	}
	org, err := c.resolveOrg(ctx, cookie)
	if err != nil {
		return nil, err
	}
	// The meters/subscription and the prepaid balance are independent calls:
	// either may fail without discarding the other's data.
	raw, goErr := c.fetchGoStatus(ctx, cookie, org)
	balance, billingErr := c.fetchPrepaidBalance(ctx, cookie, org)
	if goErr != nil && billingErr != nil {
		if errors.Is(goErr, ErrCredentialsExpired) || errors.Is(billingErr, ErrCredentialsExpired) {
			return nil, ErrCredentialsExpired
		}
		return nil, goErr
	}
	usage := &ProviderUsage{Provider: "opencode-go", FetchedAt: time.Now().UTC()}
	if goErr == nil {
		usage.Plan = raw.Product
		usage.Windows = raw.windows()
		usage.Subscription = raw.subscription(time.Now())
	}
	if billingErr == nil && balance != nil {
		usage.Balances = append(usage.Balances, *balance)
	}
	return usage, nil
}

// fetchGoStatus reads the Go meters and subscription state. A 404 or a `null`
// body means the account has no Go subscription and is not an error.
func (c *OpenCodeGoUsageClient) fetchGoStatus(ctx context.Context, cookie, org string) (*opencodeGoStatus, error) {
	body, status, err := c.get(ctx, "/console/api/go/status", cookie, org)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil, ErrCredentialsExpired
	}
	if status == http.StatusNotFound {
		return &opencodeGoStatus{}, nil
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("opencode go usage: status %d", status)
	}
	if trimmed := strings.TrimSpace(string(body)); trimmed == "" || trimmed == jsonNullBody {
		return &opencodeGoStatus{}, nil
	}
	var raw opencodeGoStatus
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("opencode go usage: decode response: %w", err)
	}
	return &raw, nil
}

// fetchPrepaidBalance reads the account's available prepaid balance in USD.
// availableMicroCents is a decimal string; 1 USD = 1e8 micro-cents.
func (c *OpenCodeGoUsageClient) fetchPrepaidBalance(ctx context.Context, cookie, org string) (*Balance, error) {
	body, status, err := c.get(ctx, "/console/api/billing/status", cookie, org)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil, ErrCredentialsExpired
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("opencode go billing: status %d", status)
	}
	if trimmed := strings.TrimSpace(string(body)); trimmed == "" || trimmed == jsonNullBody {
		return nil, nil
	}
	var raw opencodeGoBilling
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("opencode go billing: decode response: %w", err)
	}
	micro, err := strconv.ParseFloat(strings.TrimSpace(raw.AvailableMicroCents), 64)
	if err != nil {
		return nil, nil
	}
	return &Balance{Label: "prepaid", Amount: micro / 1e8, Unit: "USD"}, nil
}

func (c *OpenCodeGoUsageClient) resolveOrg(ctx context.Context, cookie string) (string, error) {
	if override := strings.TrimSpace(os.Getenv("OPENCODE_ORG_ID")); override != "" {
		return override, nil
	}
	c.mu.Lock()
	if c.orgID != "" {
		org := c.orgID
		c.mu.Unlock()
		return org, nil
	}
	c.mu.Unlock()
	body, status, err := c.get(ctx, "/console/api/orgs", cookie, "")
	if err != nil {
		return "", err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return "", ErrCredentialsExpired
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("opencode go usage: org status %d", status)
	}
	org := opencodeOrgID(body)
	if org == "" {
		return "", fmt.Errorf("opencode go usage: no organization")
	}
	c.mu.Lock()
	c.orgID = org
	c.mu.Unlock()
	return org, nil
}

func (c *OpenCodeGoUsageClient) get(ctx context.Context, path, cookie, org string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("opencode go usage: build request: %w", err)
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Accept", "application/json")
	if org != "" {
		req.Header.Set("x-org-id", org)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("opencode go usage: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, 0, fmt.Errorf("opencode go usage: read response: %w", err)
	}
	return body, resp.StatusCode, nil
}

func opencodeOrgID(body []byte) string {
	var direct []struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(body, &direct) == nil && len(direct) == 1 {
		return direct[0].ID
	}
	var wrapped struct {
		Organizations []struct {
			ID string `json:"id"`
		} `json:"organizations"`
		Orgs []struct {
			ID string `json:"id"`
		} `json:"orgs"`
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &wrapped) != nil {
		return ""
	}
	for _, orgs := range [][]struct {
		ID string `json:"id"`
	}{wrapped.Organizations, wrapped.Orgs, wrapped.Data} {
		if len(orgs) == 1 && orgs[0].ID != "" {
			return orgs[0].ID
		}
	}
	return ""
}

type opencodeGoMeter struct {
	ResetsAt        string `json:"resetsAt"`
	LimitMicroCents string `json:"limitMicroCents"`
	UsedMicroCents  string `json:"usedMicroCents"`
}

type opencodeGoMeters struct {
	FiveHour opencodeGoMeter `json:"fiveHour"`
	Week     opencodeGoMeter `json:"week"`
	Month    opencodeGoMeter `json:"month"`
}

type opencodeGoAccess struct {
	StartsAt string           `json:"startsAt"`
	EndsAt   string           `json:"endsAt"`
	Meters   opencodeGoMeters `json:"meters"`
}

type opencodeGoStatus struct {
	Product           string            `json:"product"`
	RenewalProduct    string            `json:"renewalProduct"`
	UseBalance        bool              `json:"useBalance"`
	CancelAtPeriodEnd bool              `json:"cancelAtPeriodEnd"`
	RenewalPending    bool              `json:"renewalPending"`
	Access            *opencodeGoAccess `json:"access"`
}

type opencodeGoBilling struct {
	AvailableMicroCents string `json:"availableMicroCents"`
}

// subscription reports the account's plan state. A null body, an absent access
// block, or a past endsAt reads as inactive.
func (s opencodeGoStatus) subscription(now time.Time) *Subscription {
	sub := &Subscription{Status: "inactive"}
	if s.Access == nil {
		return sub
	}
	endsAt, _ := time.Parse(time.RFC3339, s.Access.EndsAt)
	sub.Plan = s.Product
	sub.PeriodEndsAt = endsAt
	sub.UsesBalance = s.UseBalance
	if endsAt.IsZero() || !endsAt.After(now) {
		return sub
	}
	switch {
	case s.RenewalPending:
		sub.Status = "renewal_pending"
	case s.CancelAtPeriodEnd:
		sub.Status = "canceling"
	default:
		sub.Status = "active"
	}
	return sub
}

func (s opencodeGoStatus) windows() []UtilizationWindow {
	if s.Access == nil {
		return nil
	}
	meters := []struct {
		label string
		meter opencodeGoMeter
		reset string
	}{
		{"5-hour", s.Access.Meters.FiveHour, s.Access.Meters.FiveHour.ResetsAt},
		{"7-day", s.Access.Meters.Week, s.Access.Meters.Week.ResetsAt},
		{"30-day", s.Access.Meters.Month, s.Access.EndsAt},
	}
	windows := make([]UtilizationWindow, 0, len(meters))
	for _, item := range meters {
		limit, limitErr := strconv.ParseFloat(item.meter.LimitMicroCents, 64)
		used, usedErr := strconv.ParseFloat(item.meter.UsedMicroCents, 64)
		if limitErr != nil || usedErr != nil || limit <= 0 {
			continue
		}
		pct := used / limit * 100
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		reset, _ := time.Parse(time.RFC3339, item.reset)
		windows = append(windows, UtilizationWindow{Label: item.label, UtilizationPct: pct, ResetAt: reset})
	}
	return windows
}
