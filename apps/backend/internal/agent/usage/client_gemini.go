package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	geminiUsageBaseURL      = "https://cloudcode-pa.googleapis.com"
	geminiLoadCodeAssistRPC = "/v1internal:loadCodeAssist"
	geminiRetrieveQuotaRPC  = "/v1internal:retrieveUserQuota"

	geminiProvider = "google"

	// geminiExpirySkew treats a token within this window of its expiry as
	// expired, so a fetch never starts with a token that is about to lapse.
	geminiExpirySkew = 60 * time.Second
)

// ErrCredentialsExpired reports that a stored provider credential is expired.
// Kandev never refreshes it (that would require borrowing the provider's OAuth
// client credentials), so the account is reported unknown or unavailable
// rather than 0%.
var ErrCredentialsExpired = errors.New("provider credentials are expired")

// GeminiUsageClient fetches Gemini Code Assist quota using the OAuth token the
// Gemini CLI already stored at ~/.gemini/oauth_creds.json. It is strictly
// read-only: it never refreshes the token and never writes the file.
type GeminiUsageClient struct {
	credentialsPath string
	baseURL         string
	httpClient      *http.Client

	mu              sync.Mutex
	project         string
	tier            string
	projectResolved bool
}

// NewGeminiUsageClientWithPath creates a client with an explicit credentials
// path (for tests).
func NewGeminiUsageClientWithPath(credentialsPath string) *GeminiUsageClient {
	return &GeminiUsageClient{
		credentialsPath: credentialsPath,
		baseURL:         geminiUsageBaseURL,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
	}
}

// CredentialsPath returns the path this client reads the token from.
func (c *GeminiUsageClient) CredentialsPath() string {
	return c.credentialsPath
}

// HasSubscriptionCredentials reports whether the credentials file exists and
// carries a non-empty access or refresh token.
func (c *GeminiUsageClient) HasSubscriptionCredentials() bool {
	creds, err := c.readCredentials()
	if err != nil {
		return false
	}
	return creds.AccessToken != "" || creds.RefreshToken != ""
}

type geminiCredentials struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiryDate   int64  `json:"expiry_date"` // Unix milliseconds
}

type geminiLoadCodeAssistResponse struct {
	CloudAiCompanionProject json.RawMessage `json:"cloudaicompanionProject"`
	CurrentTier             *struct {
		ID string `json:"id"`
	} `json:"currentTier"`
}

type geminiQuotaBucket struct {
	ModelID           string   `json:"modelId"`
	TokenType         string   `json:"tokenType"`
	RemainingFraction *float64 `json:"remainingFraction"`
	ResetTime         string   `json:"resetTime"`
}

type geminiQuotaResponse struct {
	Buckets []geminiQuotaBucket `json:"buckets"`
}

// FetchUsage implements ProviderUsageClient.
func (c *GeminiUsageClient) FetchUsage(ctx context.Context) (*ProviderUsage, error) {
	creds, err := c.readCredentials()
	if err != nil {
		return nil, fmt.Errorf("gemini usage: credentials unavailable: %w", err)
	}
	if creds.AccessToken == "" {
		return nil, ErrCredentialsMissing
	}
	if geminiTokenExpired(creds.ExpiryDate, time.Now()) {
		return nil, ErrCredentialsExpired
	}

	project, tier, err := c.resolveProject(ctx, creds.AccessToken)
	if err != nil {
		return nil, err
	}
	if project == "" {
		return nil, fmt.Errorf("gemini usage: no cloud project resolved")
	}

	body, err := c.retrieveUserQuota(ctx, creds.AccessToken, project)
	if err != nil {
		return nil, err
	}
	var raw geminiQuotaResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("gemini usage: decode quota: %w", err)
	}
	now := time.Now()
	// Non-nil so the API serializes `windows` as an array even when empty.
	return &ProviderUsage{
		Provider:  geminiProvider,
		Plan:      tier,
		Windows:   geminiWindows(raw.Buckets),
		FetchedAt: now,
	}, nil
}

// geminiTokenExpired reports whether expiry_date (Unix milliseconds) is within
// the skew window of now. A zero expiry is treated as unknown, not expired.
func geminiTokenExpired(expiryMillis int64, now time.Time) bool {
	if expiryMillis == 0 {
		return false
	}
	return !time.UnixMilli(expiryMillis).After(now.Add(geminiExpirySkew))
}

// resolveProject returns the Cloud project and plan tier. GOOGLE_CLOUD_PROJECT
// overrides discovery; otherwise loadCodeAssist is called once and its result
// (project and tier) is memoized in memory only.
func (c *GeminiUsageClient) resolveProject(ctx context.Context, token string) (string, string, error) {
	envProject := strings.TrimSpace(os.Getenv("GOOGLE_CLOUD_PROJECT"))
	if project, tier, ok := c.cachedProject(); ok {
		if envProject != "" {
			return envProject, tier, nil
		}
		return project, tier, nil
	}
	if envProject != "" {
		// The environment already names the project, so no discovery request
		// is needed; the tier is left empty.
		c.storeProject(envProject, "")
		return envProject, "", nil
	}

	body, err := c.loadCodeAssist(ctx, token)
	if err != nil {
		return "", "", err
	}
	var raw geminiLoadCodeAssistResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", "", fmt.Errorf("gemini usage: decode loadCodeAssist: %w", err)
	}
	project := geminiProjectID(raw.CloudAiCompanionProject)
	tier := ""
	if raw.CurrentTier != nil {
		tier = raw.CurrentTier.ID
	}
	c.storeProject(project, tier)
	return project, tier, nil
}

func (c *GeminiUsageClient) cachedProject() (string, string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.project, c.tier, c.projectResolved
}

func (c *GeminiUsageClient) storeProject(project, tier string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.project = project
	c.tier = tier
	c.projectResolved = true
}

// geminiProjectID parses cloudaicompanionProject, which the API returns either
// as a bare string or as an object with an id.
func geminiProjectID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}
	var asObject struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &asObject); err == nil {
		return strings.TrimSpace(asObject.ID)
	}
	return ""
}

func (c *GeminiUsageClient) loadCodeAssist(ctx context.Context, token string) ([]byte, error) {
	payload := map[string]any{
		"metadata": map[string]string{
			"ideType":    "IDE_UNSPECIFIED",
			"platform":   "PLATFORM_UNSPECIFIED",
			"pluginType": "GEMINI",
		},
	}
	return c.postJSON(ctx, geminiLoadCodeAssistRPC, token, payload)
}

func (c *GeminiUsageClient) retrieveUserQuota(ctx context.Context, token, project string) ([]byte, error) {
	return c.postJSON(ctx, geminiRetrieveQuotaRPC, token, map[string]string{"project": project})
}

func (c *GeminiUsageClient) postJSON(ctx context.Context, path, token string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("gemini usage: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini usage: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini usage: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("gemini usage: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// Upstream bodies can contain a project ID or account email. The status
		// is sufficient for diagnostics and is safe to surface.
		return nil, fmt.Errorf("gemini usage: status %d", resp.StatusCode)
	}
	return respBody, nil
}

func (c *GeminiUsageClient) readCredentials() (*geminiCredentials, error) {
	data, err := os.ReadFile(c.credentialsPath)
	if err != nil {
		return nil, errors.New("credentials file unavailable")
	}
	var creds geminiCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, errors.New("credentials file is invalid")
	}
	return &creds, nil
}

// geminiWindows maps quota buckets to windows. A bucket without a
// remainingFraction has nothing authoritative to report and is skipped, as is
// a bucket with no model id; an empty result reads as unknown.
func geminiWindows(buckets []geminiQuotaBucket) []UtilizationWindow {
	windows := make([]UtilizationWindow, 0, len(buckets))
	for _, bucket := range buckets {
		if bucket.RemainingFraction == nil || strings.TrimSpace(bucket.ModelID) == "" {
			continue
		}
		windows = append(windows, UtilizationWindow{
			Label:          geminiBucketLabel(bucket),
			UtilizationPct: geminiUtilization(*bucket.RemainingFraction),
			ResetAt:        geminiResetAt(bucket.ResetTime),
			Model:          bucket.ModelID,
		})
	}
	return windows
}

func geminiBucketLabel(bucket geminiQuotaBucket) string {
	if bucket.TokenType == "" {
		return bucket.ModelID
	}
	return bucket.ModelID + " " + bucket.TokenType
}

func geminiUtilization(remainingFraction float64) float64 {
	pct := (1 - remainingFraction) * 100
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

func geminiResetAt(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t
	}
	return time.Time{}
}
