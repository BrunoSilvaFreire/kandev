package usage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func writeGeminiCreds(t *testing.T, access, refresh string, expiryMillis int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "oauth_creds.json")
	writeGeminiCredsAt(t, path, access, refresh, expiryMillis)
	return path
}

func writeGeminiCredsAt(t *testing.T, path, access, refresh string, expiryMillis int64) {
	t.Helper()
	payload := fmt.Sprintf(`{"access_token":%q,"refresh_token":%q,"expiry_date":%d}`, access, refresh, expiryMillis)
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
}

type geminiFake struct {
	mu             sync.Mutex
	loadAssistHits int
	quotaHits      int
	quotaBody      string
	status         int
}

func (f *geminiFake) server(loadAssistJSON, quotaJSON string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case geminiLoadCodeAssistRPC:
			f.loadAssistHits++
			_, _ = w.Write([]byte(loadAssistJSON))
		case geminiRetrieveQuotaRPC:
			f.quotaHits++
			body, _ := io.ReadAll(r.Body)
			f.quotaBody = string(body)
			if f.status != 0 && f.status != http.StatusOK {
				w.WriteHeader(f.status)
				_, _ = w.Write([]byte("denied"))
				return
			}
			_, _ = w.Write([]byte(quotaJSON))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestGeminiFetchUsageParsesBuckets(t *testing.T) {
	creds := writeGeminiCreds(t, "tok", "ref", time.Now().Add(time.Hour).UnixMilli())
	fake := &geminiFake{}
	srv := fake.server(
		`{"cloudaicompanionProject":"proj-1","currentTier":{"id":"free-tier"}}`,
		`{"buckets":[
			{"modelId":"gemini-2.5-pro","tokenType":"input","remainingFraction":0.25,"resetTime":"2026-10-01T00:00:00Z","unknownField":"ignored"},
			{"modelId":"gemini-2.5-flash","remainingFraction":2.0},
			{"modelId":"gemini-2.5-lite"},
			{"remainingFraction":0.5}
		]}`,
	)
	defer srv.Close()

	client := NewGeminiUsageClientWithPath(creds)
	client.baseURL = srv.URL
	usage, err := client.FetchUsage(context.Background())
	if err != nil {
		t.Fatalf("FetchUsage: %v", err)
	}
	if usage.Provider != "google" || usage.Plan != "free-tier" {
		t.Fatalf("provider/plan = %q/%q, want google/free-tier", usage.Provider, usage.Plan)
	}
	if len(usage.Windows) != 2 {
		t.Fatalf("windows = %#v, want 2", usage.Windows)
	}
	pro := usage.Windows[0]
	if pro.Label != "gemini-2.5-pro input" || pro.Model != "gemini-2.5-pro" || pro.UtilizationPct != 75 {
		t.Fatalf("pro window = %#v, want label/model/75", pro)
	}
	if pro.ResetAt.IsZero() {
		t.Fatal("pro reset time was not parsed")
	}
	if got := usage.Windows[1].UtilizationPct; got != 0 {
		t.Fatalf("flash utilization = %v, want clamped 0", got)
	}
}

func TestGeminiProjectObjectForm(t *testing.T) {
	creds := writeGeminiCreds(t, "tok", "", time.Now().Add(time.Hour).UnixMilli())
	fake := &geminiFake{}
	srv := fake.server(
		`{"cloudaicompanionProject":{"id":"proj-obj"}}`,
		`{"buckets":[{"modelId":"gemini-2.5-pro","remainingFraction":0.5}]}`,
	)
	defer srv.Close()

	client := NewGeminiUsageClientWithPath(creds)
	client.baseURL = srv.URL
	if _, err := client.FetchUsage(context.Background()); err != nil {
		t.Fatalf("FetchUsage: %v", err)
	}
	if !strings.Contains(fake.quotaBody, `"project":"proj-obj"`) {
		t.Fatalf("quota body = %s, want the object-form id", fake.quotaBody)
	}
}

func TestGeminiProjectEnvOverride(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "env-proj")
	creds := writeGeminiCreds(t, "tok", "", time.Now().Add(time.Hour).UnixMilli())
	fake := &geminiFake{}
	srv := fake.server(
		`{"cloudaicompanionProject":"discovered"}`,
		`{"buckets":[{"modelId":"gemini-2.5-pro","remainingFraction":0.5}]}`,
	)
	defer srv.Close()

	client := NewGeminiUsageClientWithPath(creds)
	client.baseURL = srv.URL
	if _, err := client.FetchUsage(context.Background()); err != nil {
		t.Fatalf("FetchUsage: %v", err)
	}
	if !strings.Contains(fake.quotaBody, `"project":"env-proj"`) {
		t.Fatalf("quota body = %s, want the env project", fake.quotaBody)
	}
	if fake.loadAssistHits != 0 {
		t.Fatalf("loadCodeAssist called %d times with an env project, want 0", fake.loadAssistHits)
	}
}

func TestGeminiProjectMemoized(t *testing.T) {
	creds := writeGeminiCreds(t, "tok", "", time.Now().Add(time.Hour).UnixMilli())
	fake := &geminiFake{}
	srv := fake.server(
		`{"cloudaicompanionProject":"proj-1"}`,
		`{"buckets":[]}`,
	)
	defer srv.Close()

	client := NewGeminiUsageClientWithPath(creds)
	client.baseURL = srv.URL
	for i := 0; i < 2; i++ {
		if _, err := client.FetchUsage(context.Background()); err != nil {
			t.Fatalf("FetchUsage %d: %v", i, err)
		}
	}
	if fake.loadAssistHits != 1 {
		t.Fatalf("loadCodeAssist called %d times, want 1", fake.loadAssistHits)
	}
}

func TestGeminiExpiredTokenIsTypedAndSideEffectFree(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oauth_creds.json")
	writeGeminiCredsAt(t, path, "tok", "ref", time.Now().Add(-time.Hour).UnixMilli())
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}

	fake := &geminiFake{}
	srv := fake.server(`{}`, `{}`)
	defer srv.Close()
	client := NewGeminiUsageClientWithPath(path)
	client.baseURL = srv.URL

	_, fetchErr := client.FetchUsage(context.Background())
	if !errors.Is(fetchErr, ErrCredentialsExpired) {
		t.Fatalf("error = %v, want ErrCredentialsExpired", fetchErr)
	}
	if fake.loadAssistHits != 0 || fake.quotaHits != 0 {
		t.Fatalf("expired token made %d/%d HTTP calls, want 0/0", fake.loadAssistHits, fake.quotaHits)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("the credentials file was modified")
	}
}

func TestGeminiMissingCredentialsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private-email@example.test", "missing.json")
	client := NewGeminiUsageClientWithPath(path)
	if _, err := client.FetchUsage(context.Background()); err == nil {
		t.Fatal("a missing credentials file must be an error")
	} else if strings.Contains(err.Error(), path) || strings.Contains(err.Error(), "private-email@example.test") {
		t.Fatalf("error leaked credential path: %v", err)
	}
}

func TestGeminiNon200OmitsToken(t *testing.T) {
	creds := writeGeminiCreds(t, "super-secret-token", "", time.Now().Add(time.Hour).UnixMilli())
	fake := &geminiFake{status: http.StatusUnauthorized}
	srv := fake.server(`{"cloudaicompanionProject":"proj-1"}`, `{}`)
	defer srv.Close()

	client := NewGeminiUsageClientWithPath(creds)
	client.baseURL = srv.URL
	_, err := client.FetchUsage(context.Background())
	if err == nil {
		t.Fatal("a non-200 response must be an error")
	}
	for _, forbidden := range []string{"super-secret-token", "proj-1", "denied"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("error leaked %q: %v", forbidden, err)
		}
	}
}

func TestGeminiHasSubscriptionCredentials(t *testing.T) {
	access := writeGeminiCreds(t, "tok", "", time.Now().Add(time.Hour).UnixMilli())
	if !NewGeminiUsageClientWithPath(access).HasSubscriptionCredentials() {
		t.Fatal("an access token must count as subscription credentials")
	}
	refreshOnly := writeGeminiCreds(t, "", "ref", 0)
	if !NewGeminiUsageClientWithPath(refreshOnly).HasSubscriptionCredentials() {
		t.Fatal("a refresh token must count as subscription credentials")
	}
	empty := writeGeminiCreds(t, "", "", 0)
	if NewGeminiUsageClientWithPath(empty).HasSubscriptionCredentials() {
		t.Fatal("an empty credential file is not a subscription")
	}
	if NewGeminiUsageClientWithPath(filepath.Join(t.TempDir(), "missing.json")).HasSubscriptionCredentials() {
		t.Fatal("a missing file is not a subscription")
	}
}

func TestGeminiNoBucketsIsUnknown(t *testing.T) {
	creds := writeGeminiCreds(t, "tok", "", time.Now().Add(time.Hour).UnixMilli())
	fake := &geminiFake{}
	srv := fake.server(`{"cloudaicompanionProject":"proj-1"}`, `{"buckets":[]}`)
	defer srv.Close()

	client := NewGeminiUsageClientWithPath(creds)
	client.baseURL = srv.URL
	usage, err := client.FetchUsage(context.Background())
	if err != nil {
		t.Fatalf("FetchUsage: %v", err)
	}
	if usage == nil || len(usage.Windows) != 0 {
		t.Fatalf("windows = %#v, want none", usage)
	}
	if _, known := RemainingPct(usage, ""); known {
		t.Fatal("no buckets must read as unknown")
	}
}
