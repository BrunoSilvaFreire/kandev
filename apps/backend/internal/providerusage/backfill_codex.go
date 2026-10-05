package providerusage

import (
	"encoding/json"
	"io"
	"path/filepath"
	"time"

	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

// codexScanner parses Codex rollout transcripts:
// ~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl. Lines carrying
// payload.rate_limits are measured utilization points.
type codexScanner struct {
	home     string
	accounts Accounts
}

// NewCodexScanner returns the Codex transcript scanner for home.
func NewCodexScanner(home string) Scanner {
	return &codexScanner{home: home, accounts: HostAccounts(home)}
}

func (s *codexScanner) Source() string { return SourceCodexLocal }

func (s *codexScanner) ListFiles() ([]string, error) {
	return listFilesByExtension(filepath.Join(s.home, ".codex", "sessions"), ".jsonl")
}

type codexLine struct {
	Timestamp string `json:"timestamp"`
	Payload   struct {
		RateLimits *codexRateLimits `json:"rate_limits"`
	} `json:"payload"`
}

type codexRateLimits struct {
	PlanType  string       `json:"plan_type"`
	Primary   *codexWindow `json:"primary"`
	Secondary *codexWindow `json:"secondary"`
}

type codexWindow struct {
	UsedPercent     float64 `json:"used_percent"`
	WindowMinutes   int64   `json:"window_minutes"`
	ResetsAt        int64   `json:"resets_at"`
	ResetsInSeconds int64   `json:"resets_in_seconds"`
}

func (s *codexScanner) Parse(r io.Reader, _ string) ([]Observation, int64, error) {
	var observations []Observation
	consumed, err := readCompleteLines(r, func(line []byte) {
		observations = append(observations, s.parseLine(line)...)
	})
	return observations, consumed, err
}

func (s *codexScanner) parseLine(line []byte) []Observation {
	var raw codexLine
	if err := json.Unmarshal(line, &raw); err != nil || raw.Payload.RateLimits == nil {
		return nil
	}
	observedAt, err := time.Parse(time.RFC3339, raw.Timestamp)
	if err != nil {
		return nil
	}
	var observations []Observation
	for _, window := range []*codexWindow{raw.Payload.RateLimits.Primary, raw.Payload.RateLimits.Secondary} {
		if window == nil {
			continue
		}
		pct := window.UsedPercent
		observations = append(observations, Observation{
			AccountKey:     s.accounts.OpenAI,
			Provider:       "openai",
			WindowLabel:    agentusage.CodexWindowLabel(window.WindowMinutes * 60),
			Kind:           KindMeasured,
			UtilizationPct: &pct,
			ResetAt:        codexResetAt(window, observedAt),
			ObservedAt:     observedAt,
			Source:         SourceCodexLocal,
		})
	}
	return observations
}

func codexResetAt(window *codexWindow, observedAt time.Time) time.Time {
	if window.ResetsAt > 0 {
		return time.Unix(window.ResetsAt, 0).UTC()
	}
	if window.ResetsInSeconds > 0 {
		return observedAt.Add(time.Duration(window.ResetsInSeconds) * time.Second)
	}
	return time.Time{}
}
