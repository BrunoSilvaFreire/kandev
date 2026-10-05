package providerusage

import (
	"bytes"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

var (
	agyLogNameRe = regexp.MustCompile(`cli-(\d{8})_(\d{6})\.log$`)
	agyGlobRe    = regexp.MustCompile(`^[IWEF](\d{4}) (\d{2}:\d{2}:\d{2})`)
	agyResetRe   = regexp.MustCompile(`Resets in ([0-9]+[hms][0-9hms]*)`)
)

// antigravityScanner parses Antigravity CLI logs:
// ~/.gemini/antigravity-cli/log/cli-YYYYMMDD_HHMMSS.log. Lines reporting
// RESOURCE_EXHAUSTED become limit-hit points with a reset hint.
type antigravityScanner struct {
	home     string
	accounts Accounts
}

// NewAntigravityScanner returns the Antigravity CLI log scanner for home.
func NewAntigravityScanner(home string) Scanner {
	return &antigravityScanner{home: home, accounts: HostAccounts(home)}
}

func (s *antigravityScanner) Source() string { return SourceAgyLocal }

func (s *antigravityScanner) ListFiles() ([]string, error) {
	return listFilesByExtension(filepath.Join(s.home, ".gemini", "antigravity-cli", "log"), ".log")
}

func (s *antigravityScanner) Parse(r io.Reader, path string) ([]Observation, int64, error) {
	year := agyYear(path)
	var observations []Observation
	consumed, err := readCompleteLines(r, func(line []byte) {
		if observation, ok := parseAntigravityLogLine(line, year, s.accounts.Antigravity); ok {
			observations = append(observations, observation)
		}
	})
	return observations, consumed, err
}

func parseAntigravityLogLine(line []byte, year int, accountKey string) (Observation, bool) {
	if !bytes.Contains(line, []byte("RESOURCE_EXHAUSTED")) {
		return Observation{}, false
	}
	match := agyGlobRe.FindSubmatch(line)
	if match == nil {
		return Observation{}, false
	}
	observedAt, err := time.ParseInLocation("2006 0102 15:04:05", strconv.Itoa(year)+" "+string(match[1])+" "+string(match[2]), time.Local)
	if err != nil {
		return Observation{}, false
	}
	observedAt = observedAt.UTC()
	resetAt := time.Time{}
	if reset := agyResetRe.FindSubmatch(line); reset != nil {
		if duration, err := time.ParseDuration(string(reset[1])); err == nil {
			resetAt = observedAt.Add(duration)
		}
	}
	return Observation{
		AccountKey:  accountKey,
		Provider:    "antigravity",
		WindowLabel: "limit",
		Kind:        KindLimitHit,
		ResetAt:     resetAt,
		ObservedAt:  observedAt,
		Source:      SourceAgyLocal,
	}, true
}

func agyYear(path string) int {
	match := agyLogNameRe.FindStringSubmatch(filepath.Base(path))
	if match == nil || len(match[1]) < 4 {
		return time.Now().Year()
	}
	year, err := strconv.Atoi(match[1][:4])
	if err != nil {
		return time.Now().Year()
	}
	return year
}
