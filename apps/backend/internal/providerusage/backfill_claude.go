package providerusage

import (
	"encoding/json"
	"io"
	"path/filepath"
	"time"
)

// claudeScanner parses Claude Code transcripts:
// ~/.claude/projects/*/*.jsonl. Assistant lines with message.usage become
// hourly weighted-token buckets; transcripts expose token activity but no
// quota percentage.
type claudeScanner struct {
	home     string
	accounts Accounts
}

// NewClaudeScanner returns the Claude transcript scanner for home.
func NewClaudeScanner(home string) Scanner {
	return &claudeScanner{home: home, accounts: HostAccounts(home)}
}

func (s *claudeScanner) Source() string { return SourceClaudeLocal }

func (s *claudeScanner) ListFiles() ([]string, error) {
	return listFilesByExtension(filepath.Join(s.home, ".claude", "projects"), ".jsonl")
}

type claudeLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	// RequestID repeats across the per-content-block lines of one assistant
	// message; message.id is the fallback dedupe identity.
	RequestID string `json:"requestId"`
	Message   *struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// Parse dedupes by assistant-message identity: Claude Code writes one line per
// content block and repeats the same message.usage on each, so summing every
// line would multiply the tokens by the block count.
// ponytail: lastKey is per-Parse, so a resume whose offset splits a repeated
// message across the boundary can double-count that one message; bounded to one
// message per resume and rare.
func (s *claudeScanner) Parse(r io.Reader, _ string) ([]Observation, int64, error) {
	var observations []Observation
	var lastKey string
	consumed, err := readCompleteLines(r, func(line []byte) {
		observation, key, ok := s.parseLine(line)
		if !ok {
			return
		}
		if key != "" && key == lastKey {
			return
		}
		lastKey = key
		observations = append(observations, observation)
	})
	return observations, consumed, err
}

func (s *claudeScanner) parseLine(line []byte) (Observation, string, bool) {
	var raw claudeLine
	if err := json.Unmarshal(line, &raw); err != nil {
		return Observation{}, "", false
	}
	if raw.Message == nil || raw.Message.Usage == nil {
		return Observation{}, "", false
	}
	observedAt, err := time.Parse(time.RFC3339, raw.Timestamp)
	if err != nil {
		return Observation{}, "", false
	}
	usage := raw.Message.Usage
	weighted := claudeWeightedTokens(usage.InputTokens, usage.OutputTokens,
		usage.CacheCreationInputTokens, usage.CacheReadInputTokens)
	bucket := observedAt.UTC().Truncate(time.Hour)
	key := raw.RequestID
	if key == "" {
		key = raw.Message.ID
	}
	return Observation{
		AccountKey:     s.accounts.Anthropic,
		Provider:       "anthropic",
		WindowLabel:    "tokens",
		Kind:           KindTokens,
		WeightedTokens: &weighted,
		Model:          raw.Message.Model,
		ObservedAt:     bucket,
		Source:         SourceClaudeLocal,
	}, key, true
}

func claudeWeightedTokens(input, output, cacheCreation, cacheRead int64) int64 {
	return input + output + cacheCreation + int64(cacheReadWeight*float64(cacheRead))
}
