package usage

import (
	"context"
	"errors"
	"testing"
)

func agyFixture() string {
	return `{
  "userStatus": {
    "planStatus": {"planInfo": {"teamsTier": "pro"}},
    "cascadeModelConfigData": {
      "clientModelConfigs": [
        {"label": "Claude Sonnet 4", "quotaInfo": {"remainingFraction": 0.8, "resetTime": "2026-09-24T20:00:00Z"}},
        {"label": "Claude Opus 4", "quotaInfo": {"remainingFraction": 0.6, "resetTime": "2026-09-24T19:00:00Z"}},
        {"label": "Gemini 2.5 Pro", "quotaInfo": {"remainingFraction": 0.25, "resetTime": "2026-09-24T21:00:00Z"}},
        {"label": "Gemini 2.5 Flash", "quotaInfo": {"remainingFraction": 0.9, "resetTime": "2026-09-24T22:00:00Z"}},
        {"label": "GPT-OSS 120B", "isExhausted": true},
        {"label": "Experimental X", "quotaInfo": {"remainingFraction": 0.5, "resetTime": "2026-09-24T23:00:00Z"}}
      ]
    }
  }
}`
}

func TestParseAntigravityStatusGroups(t *testing.T) {
	got, err := parseAntigravityStatus([]byte(agyFixture()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Provider != "antigravity" || got.Plan != "pro" {
		t.Fatalf("unexpected provider/plan: %+v", got)
	}
	byLabel := map[string]UtilizationWindow{}
	for _, w := range got.Windows {
		byLabel[w.Label] = w
	}
	if len(got.Windows) != 5 {
		t.Fatalf("expected 5 grouped windows, got %d: %+v", len(got.Windows), got.Windows)
	}
	// Claude group: min fraction 0.6 → 40%, earliest reset 19:00.
	if w := byLabel["Claude models"]; w.UtilizationPct < 39.9 || w.UtilizationPct > 40.1 || w.ResetAt.Hour() != 19 {
		t.Fatalf("unexpected Claude group: %+v", w)
	}
	if w := byLabel["Gemini Pro"]; w.UtilizationPct < 74.9 || w.UtilizationPct > 75.1 {
		t.Fatalf("unexpected Gemini Pro group: %+v", w)
	}
	if w := byLabel["Gemini Flash"]; w.UtilizationPct < 9.9 || w.UtilizationPct > 10.1 {
		t.Fatalf("unexpected Gemini Flash group: %+v", w)
	}
	if w := byLabel["GPT-OSS"]; w.UtilizationPct != 100 {
		t.Fatalf("expected exhausted GPT-OSS at 100%%, got %+v", w)
	}
	if _, ok := byLabel["Experimental X"]; !ok {
		t.Fatalf("expected an unrecognized label to keep its own group: %+v", got.Windows)
	}
}

func TestAntigravityWindowsSkipsGroupsWithoutFraction(t *testing.T) {
	if windows := antigravityWindows([]agyModelConfig{{Label: "Mystery model"}}); len(windows) != 0 {
		t.Fatalf("a group with no fraction data must be skipped, got %+v", windows)
	}
	windows := antigravityWindows([]agyModelConfig{{Label: "Claude Sonnet", IsExhausted: true}})
	if len(windows) != 1 || windows[0].UtilizationPct != 100 {
		t.Fatalf("an exhausted group with no fraction must report 100%%, got %+v", windows)
	}
}

func TestAntigravityFetchNoProcess(t *testing.T) {
	client := &AntigravityUsageClient{
		listProcesses: func() ([]agyProcess, error) { return nil, nil },
		listenPorts:   func(int) ([]int, error) { return nil, nil },
		httpDo: func(context.Context, string, string, map[string]string, []byte, bool) (int, []byte, error) {
			return 0, nil, errors.New("must not be called")
		},
	}
	if _, err := client.FetchUsage(context.Background()); !errors.Is(err, ErrSourceNotRunning) {
		t.Fatalf("expected ErrSourceNotRunning, got %v", err)
	}
}

func TestAntigravityFetchHTTPThenHTTPS(t *testing.T) {
	var schemes []string
	insecure := map[string]bool{}
	client := &AntigravityUsageClient{
		listProcesses: func() ([]agyProcess, error) { return []agyProcess{{pid: 1, csrf: "tok"}}, nil },
		listenPorts:   func(int) ([]int, error) { return []int{5555}, nil },
		httpDo: func(_ context.Context, _ string, url string, headers map[string]string, _ []byte, isInsecure bool) (int, []byte, error) {
			scheme := "http"
			if headers["X-Codeium-Csrf-Token"] != "tok" {
				t.Fatal("expected CSRF header to be forwarded")
			}
			if isInsecure {
				scheme = "https"
			}
			schemes = append(schemes, scheme)
			insecure[scheme] = isInsecure
			if scheme == "http" {
				return 500, nil, nil
			}
			return 200, []byte(agyFixture()), nil
		},
	}
	got, err := client.FetchUsage(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Windows) == 0 {
		t.Fatal("expected windows from the https fallback")
	}
	if len(schemes) != 2 || schemes[0] != "http" || schemes[1] != "https" {
		t.Fatalf("expected http then https, got %v", schemes)
	}
	if insecure["http"] {
		t.Fatal("http must not use insecure TLS")
	}
	if !insecure["https"] {
		t.Fatal("https fallback must skip TLS verification for loopback")
	}
}
