package providerusage

import (
	"strings"
	"testing"
)

func TestCodexScannerParse(t *testing.T) {
	scanner := NewCodexScanner(t.TempDir())
	content := `{"timestamp":"2026-09-20T10:00:00Z","payload":{"rate_limits":{"plan_type":"plus","primary":{"used_percent":42,"window_minutes":300,"resets_at":1790000000},"secondary":{"used_percent":10,"window_minutes":10080,"resets_at":1790500000}}}}
not json
`
	observations, consumed, err := scanner.Parse(strings.NewReader(content), "rollout.jsonl")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(observations) != 2 {
		t.Fatalf("expected 2 observations, got %d: %+v", len(observations), observations)
	}
	labels := map[string]float64{}
	for _, o := range observations {
		if o.Provider != "openai" || o.Kind != KindMeasured || o.Source != SourceCodexLocal {
			t.Fatalf("unexpected observation: %+v", o)
		}
		labels[o.WindowLabel] = *o.UtilizationPct
	}
	if labels["5-hour"] != 42 || labels["7-day"] != 10 {
		t.Fatalf("unexpected labels: %+v", labels)
	}
	if consumed != int64(len(content)) {
		t.Fatalf("consumed = %d, want %d", consumed, len(content))
	}
}

func TestCompleteLinesLeavePartial(t *testing.T) {
	content := `{"a":1}
{"b":2}
{"partial"`
	var lines int
	consumed, err := readCompleteLines(strings.NewReader(content), func([]byte) { lines++ })
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if lines != 2 {
		t.Fatalf("expected 2 complete lines, got %d", lines)
	}
	if consumed != int64(len(content)-len(`{"partial"`)) {
		t.Fatalf("consumed = %d, want %d", consumed, len(content)-len(`{"partial"`))
	}
}

func TestClaudeScannerParseWeights(t *testing.T) {
	scanner := NewClaudeScanner(t.TempDir())
	line := `{"type":"assistant","timestamp":"2026-09-20T10:34:12Z","message":{"model":"claude-x","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":20,"cache_read_input_tokens":300}}}`
	observations, _, err := scanner.Parse(strings.NewReader(line+"\n"), "session.jsonl")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(observations))
	}
	obs := observations[0]
	// 100 + 50 + 20 + 0.1*300 = 200
	if obs.Kind != KindTokens || *obs.WeightedTokens != 200 {
		t.Fatalf("unexpected weighted tokens: %+v", obs)
	}
	if obs.Model != "claude-x" || obs.ObservedAt.Minute() != 0 {
		t.Fatalf("expected hourly bucket and model, got %+v", obs)
	}
}

func TestClaudeScannerDedupesRepeatedMessage(t *testing.T) {
	scanner := NewClaudeScanner(t.TempDir())
	first := `{"type":"assistant","timestamp":"2026-09-20T10:00:00Z","requestId":"req-1","message":{"id":"msg-1","model":"claude-x","usage":{"input_tokens":100,"output_tokens":50}}}`
	second := `{"type":"assistant","timestamp":"2026-09-20T10:00:05Z","requestId":"req-2","message":{"id":"msg-2","model":"claude-x","usage":{"input_tokens":10,"output_tokens":5}}}`
	content := first + "\n" + first + "\n" + first + "\n" + second + "\n"

	observations, _, err := scanner.Parse(strings.NewReader(content), "session.jsonl")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(observations) != 2 {
		t.Fatalf("expected 2 observations after dedupe, got %d: %+v", len(observations), observations)
	}
	if *observations[0].WeightedTokens != 150 || *observations[1].WeightedTokens != 15 {
		t.Fatalf("unexpected weighted tokens: %+v", observations)
	}
}

func TestClaudeScannerDedupesByMessageIDFallback(t *testing.T) {
	scanner := NewClaudeScanner(t.TempDir())
	line := `{"type":"assistant","timestamp":"2026-09-20T10:00:00Z","message":{"id":"msg-9","model":"claude-x","usage":{"input_tokens":7,"output_tokens":3}}}`
	observations, _, err := scanner.Parse(strings.NewReader(line+"\n"+line+"\n"), "session.jsonl")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(observations) != 1 {
		t.Fatalf("expected 1 observation when requestId is absent, got %d", len(observations))
	}
}

func TestClaudeScannerSkipsNonUsage(t *testing.T) {
	scanner := NewClaudeScanner(t.TempDir())
	lines := `{"type":"user","timestamp":"2026-09-20T10:00:00Z","message":{"model":"claude-x"}}
{"type":"assistant","timestamp":"2026-09-20T10:00:00Z","message":{"model":"claude-x"}}
`
	observations, _, err := scanner.Parse(strings.NewReader(lines), "session.jsonl")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(observations) != 0 {
		t.Fatalf("expected no observations, got %+v", observations)
	}
}

func TestAntigravityScannerParse(t *testing.T) {
	scanner := NewAntigravityScanner(t.TempDir())
	line := `I0924 20:10:24.123456 1234 handler.go:99] request failed: RESOURCE_EXHAUSTED quota exceeded. Resets in 29m14s` + "\n"
	observations, _, err := scanner.Parse(strings.NewReader(line), "/logs/cli-20260924_201000.log")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(observations))
	}
	obs := observations[0]
	if obs.Kind != KindLimitHit || obs.Provider != "antigravity" || obs.ResetAt.IsZero() {
		t.Fatalf("unexpected observation: %+v", obs)
	}
	if obs.ObservedAt.Year() != 2026 || int(obs.ResetAt.Sub(obs.ObservedAt).Minutes()) != 29 {
		t.Fatalf("unexpected times: observed=%v reset=%v", obs.ObservedAt, obs.ResetAt)
	}
}

func TestAntigravityScannerIgnoresOtherLines(t *testing.T) {
	scanner := NewAntigravityScanner(t.TempDir())
	observations, _, err := scanner.Parse(strings.NewReader("I0924 20:10:24.123 all good\n"), "/logs/cli-20260924_201000.log")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(observations) != 0 {
		t.Fatalf("expected no observations, got %+v", observations)
	}
}
