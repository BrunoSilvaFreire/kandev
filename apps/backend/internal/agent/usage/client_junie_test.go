package usage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func junieJWT(t *testing.T, exp time.Time) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"ES256","typ":"at+jwt"}`))
	claims, err := json.Marshal(map[string]any{"iat": exp.Add(-time.Hour).Unix(), "exp": exp.Unix()})
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(claims)
	return header + "." + payload + ".sig"
}

func writeJunieCreds(t *testing.T, token string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secure_credentials.json")
	inner, err := json.Marshal(map[string]any{"jbAccount": map[string]any{"access_token": token, "refresh_token": "refresh-should-not-be-used"}})
	if err != nil {
		t.Fatalf("marshal inner: %v", err)
	}
	outer, err := json.Marshal(map[string]any{"secrets": []map[string]string{{"key": "jb-account-stored", "secret": string(inner)}}})
	if err != nil {
		t.Fatalf("marshal outer: %v", err)
	}
	if err := os.WriteFile(path, outer, 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	return path
}

type junieRequest struct {
	auth    string
	accept  string
	eap     string
	release string
}

type junieFake struct {
	mu       sync.Mutex
	requests []junieRequest
	handler  func(req junieRequest) (int, string)
}

func (f *junieFake) serveHTTP(w http.ResponseWriter, r *http.Request) {
	req := junieRequest{
		auth:    r.Header.Get("Authorization"),
		accept:  r.Header.Get("Accept"),
		eap:     r.Header.Get("X-Accept-EAP-License"),
		release: r.Header.Get("X-Accept-Release-License"),
	}
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.mu.Unlock()
	status, body := f.handler(req)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func (f *junieFake) recorded() []junieRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]junieRequest(nil), f.requests...)
}

func TestJunieUsageAPIKeyWinsOverFile(t *testing.T) {
	fake := &junieFake{handler: func(req junieRequest) (int, string) {
		return http.StatusOK, `{"balanceLeft":5,"balanceUnit":"USD","licenseType":"JUNP","active":true}`
	}}
	server := httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	defer server.Close()

	path := writeJunieCreds(t, junieJWT(t, time.Now().Add(time.Hour)))
	client := NewJunieUsageClient(func(context.Context) (string, error) { return "api-key-123", nil }, path)
	client.baseURL = server.URL
	if _, err := client.FetchUsage(context.Background()); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	for _, req := range fake.recorded() {
		if req.auth != "Bearer api-key-123" {
			t.Fatalf("expected API key auth, got %q", req.auth)
		}
	}
}

func TestJunieUsageUnexpiredFileTokenUsed(t *testing.T) {
	fake := &junieFake{handler: func(req junieRequest) (int, string) {
		return http.StatusOK, `{"balanceLeft":5,"balanceUnit":"USD","licenseType":"JUNP","active":true}`
	}}
	server := httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	defer server.Close()

	token := junieJWT(t, time.Now().Add(time.Hour))
	path := writeJunieCreds(t, token)
	client := NewJunieUsageClient(nil, path)
	client.baseURL = server.URL
	if _, err := client.FetchUsage(context.Background()); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	for _, req := range fake.recorded() {
		if req.auth != "Bearer "+token {
			t.Fatalf("expected file token auth")
		}
	}
}

func TestJunieUsageExpiredFileTokenMakesNoRequest(t *testing.T) {
	fake := &junieFake{handler: func(req junieRequest) (int, string) {
		t.Fatalf("no request expected for an expired token")
		return http.StatusOK, ""
	}}
	server := httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	defer server.Close()

	path := writeJunieCreds(t, junieJWT(t, time.Now().Add(-time.Minute)))
	client := NewJunieUsageClient(nil, path)
	client.baseURL = server.URL
	_, err := client.FetchUsage(context.Background())
	if !errors.Is(err, ErrCredentialsExpired) {
		t.Fatalf("expected ErrCredentialsExpired, got %v", err)
	}
	if len(fake.recorded()) != 0 {
		t.Fatalf("expected zero requests, got %d", len(fake.recorded()))
	}
}

func TestJunieUsageMissingCredentials(t *testing.T) {
	client := NewJunieUsageClient(nil, filepath.Join(t.TempDir(), "absent.json"))
	_, err := client.FetchUsage(context.Background())
	if !errors.Is(err, ErrCredentialsMissing) {
		t.Fatalf("expected ErrCredentialsMissing, got %v", err)
	}
}

func TestJunieUsageBothLicenses(t *testing.T) {
	fake := &junieFake{handler: func(req junieRequest) (int, string) {
		if req.release == "true" {
			return http.StatusOK, `{"balanceLeft":993478.3365,"balanceUnit":"CREDITS","licenseType":"AIP","active":true}`
		}
		return http.StatusOK, `{"balanceLeft":300.0,"balanceUnit":"USD","licenseType":"JUNP","active":true}`
	}}
	server := httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	defer server.Close()

	client := NewJunieUsageClient(func(context.Context) (string, error) { return "k", nil }, filepath.Join(t.TempDir(), "absent.json"))
	client.baseURL = server.URL
	usage, err := client.FetchUsage(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if usage.Provider != "junie" {
		t.Fatalf("provider = %q", usage.Provider)
	}
	if len(usage.Balances) != 2 {
		t.Fatalf("balances = %+v", usage.Balances)
	}
	aip := usage.Balances[0]
	if aip.Label != "aip" || aip.Unit != "CREDITS" || aip.Amount != 993478.3365 {
		t.Fatalf("aip balance = %+v", aip)
	}
	if usage.Subscription == nil || usage.Subscription.Status != "active" || usage.Subscription.Plan != "AIP" {
		t.Fatalf("subscription = %+v", usage.Subscription)
	}
	if usage.Plan != "AIP" {
		t.Fatalf("plan = %q", usage.Plan)
	}
	requests := fake.recorded()
	if len(requests) != 2 {
		t.Fatalf("requests = %d", len(requests))
	}
	for _, req := range requests {
		if req.accept != "application/json" {
			t.Fatalf("accept = %q", req.accept)
		}
		if req.release == "true" && (req.eap != "false") {
			t.Fatalf("aip headers = %+v", req)
		}
		if req.release == "" && req.eap == "" {
			continue
		}
	}
}

func TestJunieUsageUnauthorizedIsExpired(t *testing.T) {
	fake := &junieFake{handler: func(req junieRequest) (int, string) {
		return http.StatusUnauthorized, `{"error":"nope"}`
	}}
	server := httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	defer server.Close()

	client := NewJunieUsageClient(func(context.Context) (string, error) { return "sekret-token", nil }, filepath.Join(t.TempDir(), "absent.json"))
	client.baseURL = server.URL
	_, err := client.FetchUsage(context.Background())
	if !errors.Is(err, ErrCredentialsExpired) {
		t.Fatalf("expected ErrCredentialsExpired, got %v", err)
	}
	if strings.Contains(err.Error(), "sekret-token") {
		t.Fatalf("token leaked into error: %v", err)
	}
}

func TestJunieUsagePartialFailureKeepsOther(t *testing.T) {
	fake := &junieFake{handler: func(req junieRequest) (int, string) {
		if req.release == "true" {
			return http.StatusInternalServerError, `boom`
		}
		return http.StatusOK, `{"balanceLeft":42.5,"balanceUnit":"USD","licenseType":"JUNP","active":true}`
	}}
	server := httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	defer server.Close()

	client := NewJunieUsageClient(func(context.Context) (string, error) { return "k", nil }, filepath.Join(t.TempDir(), "absent.json"))
	client.baseURL = server.URL
	usage, err := client.FetchUsage(context.Background())
	if err != nil {
		t.Fatalf("expected partial result, got %v", err)
	}
	if len(usage.Balances) != 1 || usage.Balances[0].Label != "junp" {
		t.Fatalf("balances = %+v", usage.Balances)
	}
}

func TestJunieUsageZeroBalanceBecomesWindow(t *testing.T) {
	fake := &junieFake{handler: func(req junieRequest) (int, string) {
		return http.StatusOK, `{"balanceLeft":0,"balanceUnit":"CREDITS","licenseType":"AIP","active":true}`
	}}
	server := httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	defer server.Close()

	client := NewJunieUsageClient(func(context.Context) (string, error) { return "k", nil }, filepath.Join(t.TempDir(), "absent.json"))
	client.baseURL = server.URL
	usage, err := client.FetchUsage(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(usage.Windows) != 1 || usage.Windows[0].Label != "balance" || usage.Windows[0].UtilizationPct != 100 {
		t.Fatalf("windows = %+v", usage.Windows)
	}
}

func TestJunieUsageNoTokenInIncompleteError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()
	token := "super-secret-jwt"
	client := NewJunieUsageClient(func(context.Context) (string, error) { return token, nil }, filepath.Join(t.TempDir(), "absent.json"))
	client.baseURL = server.URL
	_, err := client.FetchUsage(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
	if strings.Contains(fmt.Sprint(err), token) {
		t.Fatalf("token leaked into error: %v", err)
	}
}
