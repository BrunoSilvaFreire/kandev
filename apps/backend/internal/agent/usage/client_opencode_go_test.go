package usage

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenCodeGoUsageParsesMetersAndMemoizesOrg(t *testing.T) {
	orgCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/console/api/orgs":
			orgCalls++
			_, _ = w.Write([]byte(`{"orgs":[{"id":"org-1"}]}`))
		case "/console/api/go/status":
			if r.Header.Get("x-org-id") != "org-1" {
				t.Fatalf("org = %q", r.Header.Get("x-org-id"))
			}
			_, _ = w.Write([]byte(`{"product":"go","access":{"endsAt":"2026-10-01T00:00:00Z","meters":{"fiveHour":{"resetsAt":"2026-09-27T00:00:00Z","limitMicroCents":"100","usedMicroCents":"25"},"week":{"limitMicroCents":"100","usedMicroCents":"50"},"month":{"limitMicroCents":"100","usedMicroCents":"200"}}}}`))
		}
	}))
	defer server.Close()
	client := NewOpenCodeGoUsageClient(func(context.Context) (string, error) { return "session=secret", nil })
	client.baseURL = server.URL
	for i := 0; i < 2; i++ {
		if _, err := client.FetchUsage(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	usage, _ := client.FetchUsage(context.Background())
	if orgCalls != 1 || len(usage.Windows) != 3 || usage.Windows[2].UtilizationPct != 100 || usage.Windows[2].ResetAt.IsZero() {
		t.Fatalf("calls/windows = %d/%#v", orgCalls, usage.Windows)
	}
}

func TestOpenCodeGoUsageMissingOrExpiredCookieDoesNotCallOrLeak(t *testing.T) {
	client := NewOpenCodeGoUsageClient(func(context.Context) (string, error) { return "", nil })
	if _, err := client.FetchUsage(context.Background()); !errors.Is(err, ErrCredentialsMissing) {
		t.Fatalf("error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("session=secret"))
	}))
	defer server.Close()
	client = NewOpenCodeGoUsageClient(func(context.Context) (string, error) { return "session=secret", nil })
	client.baseURL = server.URL
	if _, err := client.FetchUsage(context.Background()); !errors.Is(err, ErrCredentialsExpired) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error = %v", err)
	}
}

func openCodeGoFake(t *testing.T, goBody string, goStatus int, billingBody string, billingStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/console/api/orgs":
			_, _ = w.Write([]byte(`{"orgs":[{"id":"org-1"}]}`))
		case "/console/api/go/status":
			w.WriteHeader(goStatus)
			_, _ = w.Write([]byte(goBody))
		case "/console/api/billing/status":
			w.WriteHeader(billingStatus)
			_, _ = w.Write([]byte(billingBody))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
}

func TestOpenCodeGoSubscriptionStates(t *testing.T) {
	past := "2020-01-01T00:00:00Z"
	future := "2999-01-01T00:00:00Z"
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"active", `{"product":"go","useBalance":true,"access":{"endsAt":"` + future + `"}}`, "active"},
		{"canceling", `{"product":"go","cancelAtPeriodEnd":true,"access":{"endsAt":"` + future + `"}}`, "canceling"},
		{"renewal_pending", `{"product":"go","renewalPending":true,"cancelAtPeriodEnd":true,"access":{"endsAt":"` + future + `"}}`, "renewal_pending"},
		{"past", `{"product":"go","access":{"endsAt":"` + past + `"}}`, "inactive"},
		{"null", `null`, "inactive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw opencodeGoStatus
			if err := json.Unmarshal([]byte(tc.raw), &raw); err != nil && tc.raw != "null" {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := raw.subscription(time.Now()); got.Status != tc.want {
				t.Fatalf("status = %q, want %q", got.Status, tc.want)
			}
		})
	}
}

func TestOpenCodeGoFetchFullSuccess(t *testing.T) {
	server := openCodeGoFake(t, `{"product":"go","renewalProduct":"go","useBalance":true,"cancelAtPeriodEnd":false,"renewalPending":false,"access":{"endsAt":"2999-01-01T00:00:00Z","meters":{"fiveHour":{"limitMicroCents":"100","usedMicroCents":"25"},"week":{"limitMicroCents":"100","usedMicroCents":"50"},"month":{"limitMicroCents":"100","usedMicroCents":"100"}}}}`, http.StatusOK,
		`{"billingMode":"prepaid","availableMicroCents":"610558101"}`, http.StatusOK)
	defer server.Close()
	client := NewOpenCodeGoUsageClient(func(context.Context) (string, error) { return "session=secret", nil })
	client.baseURL = server.URL
	usage, err := client.FetchUsage(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(usage.Windows) != 3 {
		t.Fatalf("windows = %d", len(usage.Windows))
	}
	if usage.Subscription == nil || usage.Subscription.Status != "active" || usage.Subscription.Plan != "go" || !usage.Subscription.UsesBalance {
		t.Fatalf("subscription = %+v", usage.Subscription)
	}
	if len(usage.Balances) != 1 || usage.Balances[0].Label != "prepaid" || usage.Balances[0].Unit != "USD" {
		t.Fatalf("balances = %+v", usage.Balances)
	}
	if diff := usage.Balances[0].Amount - 6.10558101; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("balance = %v", usage.Balances[0].Amount)
	}
}

func TestOpenCodeGoNullStatusStillReturnsBalance(t *testing.T) {
	server := openCodeGoFake(t, `null`, http.StatusOK, `{"availableMicroCents":"610558101"}`, http.StatusOK)
	defer server.Close()
	client := NewOpenCodeGoUsageClient(func(context.Context) (string, error) { return "session=secret", nil })
	client.baseURL = server.URL
	usage, err := client.FetchUsage(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if usage.Subscription == nil || usage.Subscription.Status != "inactive" || len(usage.Windows) != 0 {
		t.Fatalf("usage = %+v", usage)
	}
	if len(usage.Balances) != 1 {
		t.Fatalf("balances = %+v", usage.Balances)
	}
}

func TestOpenCodeGoBillingFailureKeepsMeters(t *testing.T) {
	server := openCodeGoFake(t, `{"product":"go","access":{"endsAt":"2999-01-01T00:00:00Z","meters":{"fiveHour":{"limitMicroCents":"100","usedMicroCents":"25"},"week":{"limitMicroCents":"100","usedMicroCents":"50"},"month":{"limitMicroCents":"100","usedMicroCents":"100"}}}}`, http.StatusOK, `boom`, http.StatusInternalServerError)
	defer server.Close()
	client := NewOpenCodeGoUsageClient(func(context.Context) (string, error) { return "session=secret", nil })
	client.baseURL = server.URL
	usage, err := client.FetchUsage(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(usage.Windows) != 3 || usage.Subscription == nil || len(usage.Balances) != 0 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestOpenCodeGoBothFailExpired(t *testing.T) {
	server := openCodeGoFake(t, `session=secret`, http.StatusUnauthorized, `session=secret`, http.StatusUnauthorized)
	defer server.Close()
	client := NewOpenCodeGoUsageClient(func(context.Context) (string, error) { return "session=secret", nil })
	client.baseURL = server.URL
	_, err := client.FetchUsage(context.Background())
	if !errors.Is(err, ErrCredentialsExpired) {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("cookie leaked into error: %v", err)
	}
}
