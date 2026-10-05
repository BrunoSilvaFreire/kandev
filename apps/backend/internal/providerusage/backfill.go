package providerusage

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

// cacheReadWeight discounts cached-read tokens in the weighed token formula.
const cacheReadWeight = 0.1

// Accounts holds the live account keys backfilled history is attributed to.
// Transcripts carry no account identity, so backfill uses the current host
// credential's key (a recorded limitation).
type Accounts struct {
	Anthropic   string
	OpenAI      string
	Antigravity string
}

// HostAccounts derives the standard host account keys from home.
func HostAccounts(home string) Accounts {
	return Accounts{
		Anthropic:   agentusage.CacheKey("anthropic", filepath.Join(home, ".claude", ".credentials.json")),
		OpenAI:      agentusage.CacheKey("openai", filepath.Join(home, ".codex", "auth.json")),
		Antigravity: agentusage.AntigravityCacheKey(),
	}
}

// Scanner parses one local history source. ListFiles is only called when the
// source is enabled, so a disabled source never lists or reads its tree.
type Scanner interface {
	Source() string
	ListFiles() ([]string, error)
	Parse(r io.Reader, path string) ([]Observation, int64, error)
}

// pathHash hashes a file path into the cursor table's fixed-width key.
func pathHash(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])
}

// readCompleteLines calls fn for each newline-terminated line and returns the
// number of bytes consumed. A trailing partial line is left unconsumed so a
// growing file resumes at a line boundary on the next run.
func readCompleteLines(r io.Reader, fn func(line []byte)) (int64, error) {
	reader := bufio.NewReader(r)
	var consumed int64
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return consumed, nil
			}
			return consumed, err
		}
		fn(line)
		consumed += int64(len(line))
	}
}

// listFilesByExtension walks root and returns matching files, or nil when the
// directory does not exist.
func listFilesByExtension(root, ext string) ([]string, error) {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ext) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
