package reviewepisode

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	// The window runs to the statement's WHERE; a statement without one ends
	// at the bound instead.
	taskUpdate = regexp.MustCompile(`(?s)UPDATE tasks\b(.{0,1000}?)(WHERE|\z)`)
	stateWrite = regexp.MustCompile(`(^|[^_a-z])state\s*=`)
)

// exemptWriters build a statement whose caller applies the episode itself.
var exemptWriters = map[string]string{
	"buildTaskUpdateQuery": "updateTaskTx runs applyTaskReviewEpisodeTx around this statement",
}

// Every statement that writes tasks.state must keep the Review episode in
// step, either through the task repository helper or the shared SET clause.
func TestEveryTaskStateWriterMaintainsTheReviewEpisode(t *testing.T) {
	root := filepath.Join("..", "..")
	found := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		source := string(raw)
		for _, match := range taskUpdate.FindAllStringSubmatchIndex(source, -1) {
			if !stateWrite.MatchString(source[match[2]:match[3]]) {
				continue
			}
			found++
			start := strings.LastIndex(source[:match[0]], "\nfunc ")
			end := strings.Index(source[match[0]:], "\nfunc ")
			if end < 0 {
				end = len(source) - match[0]
			}
			body := source[start+1 : match[0]+end]
			name := strings.TrimSpace(strings.SplitN(body, "\n", 2)[0])
			if strings.Contains(body, "applyTaskReviewEpisodeTx") ||
				strings.Contains(body, "syncTaskReviewEpisodeTx") || strings.Contains(body, "reviewepisode.SetClause") {
				continue
			}
			exempt := false
			for writer := range exemptWriters {
				exempt = exempt || strings.Contains(name, writer+"(")
			}
			if !exempt {
				t.Errorf("%s: %s writes tasks.state without maintaining the Review episode", path, name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if found < 8 {
		t.Fatalf("found only %d task-state writers; the scan pattern no longer matches the code", found)
	}
}
