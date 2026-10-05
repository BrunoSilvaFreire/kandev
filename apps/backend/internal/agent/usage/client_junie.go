package usage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// JunieAPIKeySecretName is the Kandev global secret holding a Junie CLI API
	// key (generated at https://junie.jetbrains.com/cli). It is the durable
	// source for the account balance; the local login token is a best-effort
	// fallback that lapses after about an hour.
	JunieAPIKeySecretName = "junie-api-key"

	junieBaseURL      = "https://ingrazzio-cloud-prod.labs.jb.gg"
	junieAuthPath     = "/auth/test"
	junieProviderName = "junie"
)

// JunieUsageClient reads the Junie account balance and license state from the
// Ingrazzio backend the Junie CLI itself calls. Credential retrieval is owned
// by the caller so this package never knows Kandev secrets. It is strictly
// read-only and never refreshes anything.
type JunieUsageClient struct {
	apiKey     func(context.Context) (string, error)
	credPath   string
	baseURL    string
	httpClient *http.Client
}

// NewJunieUsageClient creates a client. apiKey may be nil, in which case only
// the local credential file is consulted.
func NewJunieUsageClient(apiKey func(context.Context) (string, error), credPath string) *JunieUsageClient {
	return &JunieUsageClient{
		apiKey:     apiKey,
		credPath:   credPath,
		baseURL:    junieBaseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// JunieCacheKey returns the single live cache key shared by every junie-acp
// profile: one account per install, whatever the token source.
func JunieCacheKey() string {
	return CacheKey(junieProviderName, "account")
}

type junieSecureCredentials struct {
	Secrets []struct {
		Key    string `json:"key"`
		Secret string `json:"secret"`
	} `json:"secrets"`
}

type junieAuthInfo struct {
	LicenseType string   `json:"licenseType"`
	BalanceLeft *float64 `json:"balanceLeft"`
	BalanceUnit string   `json:"balanceUnit"`
	Active      bool     `json:"active"`
}

// FetchUsage implements ProviderUsageClient. It queries /auth/test twice: once
// asking for the release (AIP) license and once with no license headers
// (JUNP). Either call may succeed on its own.
func (c *JunieUsageClient) FetchUsage(ctx context.Context) (*ProviderUsage, error) {
	token, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	aip, aipErr := c.fetchAuth(ctx, token, true)
	junp, junpErr := c.fetchAuth(ctx, token, false)
	if aipErr != nil && junpErr != nil {
		return nil, preferredJunieError(aipErr, junpErr)
	}
	return junieUsageFrom(aip, junp), nil
}

// token resolves the bearer token: the API-key secret wins, otherwise the
// local login token while it is still valid.
func (c *JunieUsageClient) token(ctx context.Context) (string, error) {
	if c.apiKey != nil {
		if key, err := c.apiKey(ctx); err == nil && strings.TrimSpace(key) != "" {
			return strings.TrimSpace(key), nil
		}
	}
	return c.fileToken()
}

func (c *JunieUsageClient) fileToken() (string, error) {
	token, err := readJunieFileToken(c.credPath)
	if err != nil {
		return "", err
	}
	exp, ok := junieTokenExpiry(token)
	if !ok {
		return "", ErrCredentialsMissing
	}
	if !exp.After(time.Now()) {
		return "", ErrCredentialsExpired
	}
	return token, nil
}

// readJunieFileToken reads the Junie CLI login token from the local credential
// file, without any expiry judgement. It makes no HTTP call and never reads the
// refresh token.
func readJunieFileToken(credPath string) (string, error) {
	data, err := os.ReadFile(credPath)
	if err != nil {
		return "", ErrCredentialsMissing
	}
	var creds junieSecureCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", ErrCredentialsMissing
	}
	for _, entry := range creds.Secrets {
		if entry.Key != "jb-account-stored" {
			continue
		}
		var inner struct {
			JbAccount struct {
				AccessToken string `json:"access_token"`
			} `json:"jbAccount"`
		}
		if err := json.Unmarshal([]byte(entry.Secret), &inner); err != nil {
			return "", ErrCredentialsMissing
		}
		if token := strings.TrimSpace(inner.JbAccount.AccessToken); token != "" {
			return token, nil
		}
	}
	return "", ErrCredentialsMissing
}

// JunieLocalCredentialState reports whether the local credential file carries
// a login token and whether that token is expired, with no HTTP call. A token
// without a decodable exp is treated as present and not expired.
func JunieLocalCredentialState(credPath string) (present, expired bool) {
	token, err := readJunieFileToken(credPath)
	if err != nil {
		return false, false
	}
	exp, ok := junieTokenExpiry(token)
	if !ok {
		return true, false
	}
	return true, !exp.After(time.Now())
}

// junieTokenExpiry decodes the JWT exp claim without verifying the signature.
func junieTokenExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp *float64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == nil {
		return time.Time{}, false
	}
	return time.Unix(int64(*claims.Exp), 0), true
}

func (c *JunieUsageClient) fetchAuth(ctx context.Context, token string, aip bool) (*junieAuthInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+junieAuthPath, nil)
	if err != nil {
		return nil, fmt.Errorf("junie usage: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if aip {
		req.Header.Set("X-Accept-EAP-License", "false")
		req.Header.Set("X-Accept-Release-License", "true")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("junie usage: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("junie usage: read response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrCredentialsExpired
	}
	if resp.StatusCode != http.StatusOK {
		// The status is safe to surface; the token never enters the message.
		return nil, fmt.Errorf("junie usage: status %d", resp.StatusCode)
	}
	var info junieAuthInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("junie usage: decode response: %w", err)
	}
	return &info, nil
}

func preferredJunieError(aipErr, junpErr error) error {
	if errors.Is(aipErr, ErrCredentialsExpired) || errors.Is(junpErr, ErrCredentialsExpired) {
		return ErrCredentialsExpired
	}
	return aipErr
}

// junieUsageFrom folds the two license responses into one account view.
func junieUsageFrom(responses ...*junieAuthInfo) *ProviderUsage {
	usage := &ProviderUsage{Provider: junieProviderName, FetchedAt: time.Now().UTC()}
	seen := make(map[string]bool)
	status := "inactive"
	plan := ""
	var total float64
	for _, info := range responses {
		if info == nil {
			continue
		}
		if info.Active {
			if status != "active" {
				status = "active"
			}
			if plan == "" {
				plan = info.LicenseType
			}
		}
		if info.BalanceLeft == nil {
			continue
		}
		label := strings.ToLower(strings.TrimSpace(info.LicenseType))
		if label == "" {
			label = "balance"
		}
		if seen[label] {
			continue
		}
		seen[label] = true
		total += *info.BalanceLeft
		usage.Balances = append(usage.Balances, Balance{Label: label, Amount: *info.BalanceLeft, Unit: info.BalanceUnit})
	}
	usage.Plan = plan
	usage.Subscription = &Subscription{Status: status, Plan: plan}
	if len(usage.Balances) > 0 && total <= 0 {
		usage.Windows = append(usage.Windows, UtilizationWindow{Label: "balance", UtilizationPct: 100})
	}
	return usage
}
