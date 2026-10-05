package providerusage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeScanner struct {
	source  string
	files   []string
	listed  *bool
	parseFn func(data []byte) []Observation
}

func (f *fakeScanner) Source() string { return f.source }

func (f *fakeScanner) ListFiles() ([]string, error) {
	if f.listed != nil {
		*f.listed = true
	}
	return f.files, nil
}

func (f *fakeScanner) Parse(r io.Reader, _ string) ([]Observation, int64, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, 0, err
	}
	var complete []byte
	if idx := lastNewline(data); idx >= 0 {
		complete = data[:idx+1]
	}
	if f.parseFn == nil {
		return nil, int64(len(complete)), nil
	}
	return f.parseFn(complete), int64(len(complete)), nil
}

func lastNewline(data []byte) int {
	for i := len(data) - 1; i >= 0; i-- {
		if data[i] == '\n' {
			return i
		}
	}
	return -1
}

// blockingScanner signals entry into Parse and waits for release, so a test
// can disable its source while a file is being processed.
type blockingScanner struct {
	source  string
	account string
	files   []string
	entered chan struct{}
	release chan struct{}
}

func (b *blockingScanner) Source() string { return b.source }

func (b *blockingScanner) ListFiles() ([]string, error) { return b.files, nil }

func (b *blockingScanner) Parse(io.Reader, string) ([]Observation, int64, error) {
	select {
	case b.entered <- struct{}{}:
	default:
	}
	<-b.release
	pct := 1.0
	account := b.account
	if account == "" {
		account = "k"
	}
	return []Observation{{
		AccountKey: account, Provider: "openai", WindowLabel: "5-hour", Kind: KindMeasured,
		UtilizationPct: &pct, ObservedAt: time.Now().UTC(), Source: b.source,
	}}, 5, nil
}

type fakeLedger struct {
	events []LedgerEvent
	block  chan struct{}
}

func (f *fakeLedger) ListUsageEventsAfter(ctx context.Context, afterID int64, limit int) ([]LedgerEvent, error) {
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	var out []LedgerEvent
	for _, event := range f.events {
		if event.ID > afterID {
			out = append(out, event)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type staticResolver struct {
	key string
	ok  bool
}

func (s staticResolver) EnsureCacheKey(context.Context, string) (string, bool) {
	return s.key, s.ok
}

func TestIndexerKandevOnlyAndRerunNoOp(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	now := time.Now().UTC()
	ledger := &fakeLedger{events: []LedgerEvent{
		{ID: 1, AgentProfileID: "p1", AgentType: "claude-acp", Model: "claude", Provider: "anthropic",
			TokensIn: 100, TokensOut: 50, OccurredAt: now},
		{ID: 2, AgentProfileID: "p2", AgentType: "codex-acp", Model: "gpt", Provider: "openai",
			TokensIn: 10, OccurredAt: now},
	}}
	idx := NewIndexer(repo, ledger, staticResolver{key: "acct", ok: true}, Accounts{Anthropic: "a", OpenAI: "o"}, nil)

	idx.run(ctx)
	if idx.Status().State != IndexDone {
		t.Fatalf("expected done, got %+v", idx.Status())
	}
	first, _ := repo.ListObservations(ctx, "acct", time.Time{})
	if len(first) != 2 {
		t.Fatalf("expected 2 observations, got %d", len(first))
	}
	idx.run(ctx)
	second, _ := repo.ListObservations(ctx, "acct", time.Time{})
	if len(second) != 2 {
		t.Fatalf("rerun must be a no-op, got %d observations", len(second))
	}
}

func TestIndexerLocalListerGatedByConsent(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	listed := false
	scanner := &fakeScanner{source: SourceClaudeLocal, listed: &listed}
	idx := NewIndexer(repo, &fakeLedger{}, staticResolver{}, Accounts{}, []Scanner{scanner})

	idx.run(ctx)
	if listed {
		t.Fatal("a disabled local source must never be listed")
	}

	if err := repo.SetSource(ctx, SourceClaudeLocal, true); err != nil {
		t.Fatalf("set source: %v", err)
	}
	idx.run(ctx)
	if !listed {
		t.Fatal("an enabled local source must be listed")
	}
}

func TestIndexerConcurrentStartReturnsRunning(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	block := make(chan struct{})
	idx := NewIndexer(repo, &fakeLedger{block: block}, staticResolver{}, Accounts{}, nil)

	first := idx.Start()
	if first.State != IndexRunning {
		t.Fatalf("expected running, got %+v", first)
	}
	second := idx.Start()
	if second.State != IndexRunning {
		t.Fatalf("second start must report running, got %+v", second)
	}
	close(block)
	waitForDone(t, idx, ctx)
}

func TestIndexFileResumesAtOffset(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	path := filepath.Join(t.TempDir(), "history.jsonl")
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	scanner := &fakeScanner{source: SourceCodexLocal, files: []string{path}, parseFn: func(data []byte) []Observation {
		var out []Observation
		for range strings.Split(strings.TrimSpace(string(data)), "\n") {
			pct := 1.0
			out = append(out, Observation{
				AccountKey: "k", Provider: "openai", WindowLabel: "5-hour", Kind: KindMeasured,
				UtilizationPct: &pct, ObservedAt: time.Now().UTC(), Source: SourceCodexLocal,
			})
		}
		return out
	}}
	idx := NewIndexer(repo, &fakeLedger{}, staticResolver{}, Accounts{}, []Scanner{scanner})
	if err := repo.SetSource(ctx, SourceCodexLocal, true); err != nil {
		t.Fatalf("enable source: %v", err)
	}

	idx.run(ctx)
	cursor, ok, _ := repo.GetCursor(ctx, pathHash(path))
	if !ok || cursor.ByteOffset != int64(len("one\ntwo\n")) {
		t.Fatalf("unexpected cursor: %+v ok=%v", cursor, ok)
	}
	rows, _ := repo.ListObservations(ctx, "k", time.Time{})
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows after first run, got %d", len(rows))
	}

	appendFile(t, path, "three\n")
	idx.run(ctx)
	cursor, _, _ = repo.GetCursor(ctx, pathHash(path))
	if cursor.ByteOffset != int64(len("one\ntwo\nthree\n")) {
		t.Fatalf("cursor did not advance: %+v", cursor)
	}
	rows, _ = repo.ListObservations(ctx, "k", time.Time{})
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows after resume, got %d", len(rows))
	}
}

func TestIndexerDisableDuringRunForgetsSource(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	path := filepath.Join(t.TempDir(), "history.jsonl")
	if err := os.WriteFile(path, []byte("one\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := repo.SetSource(ctx, SourceCodexLocal, true); err != nil {
		t.Fatalf("enable source: %v", err)
	}
	scanner := &blockingScanner{
		source:  SourceCodexLocal,
		files:   []string{path},
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	idx := NewIndexer(repo, &fakeLedger{}, staticResolver{}, Accounts{}, []Scanner{scanner})
	svc := NewService(repo, nil, nil, Accounts{}, idx, testLogger(t))

	idx.Start()
	<-scanner.entered

	done := make(chan error, 1)
	go func() { done <- svc.SetSource(ctx, SourceCodexLocal, false) }()
	close(scanner.release)
	if err := <-done; err != nil {
		t.Fatalf("disable source: %v", err)
	}
	waitForDone(t, idx, ctx)

	rows, _ := repo.ListObservations(ctx, "k", time.Time{})
	if len(rows) != 0 {
		t.Fatalf("disabled source must leave no observations, got %d", len(rows))
	}
	if _, ok, _ := repo.GetCursor(ctx, pathHash(path)); ok {
		t.Fatal("disabled source must leave no cursor")
	}
}

func TestIndexerDisableKeepsOtherSourcesIndexed(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	aPath := filepath.Join(t.TempDir(), "claude.jsonl")
	bPath := filepath.Join(t.TempDir(), "codex.jsonl")
	if err := os.WriteFile(aPath, []byte("a\n"), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(bPath, []byte("b\n"), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}
	if err := repo.SetSource(ctx, SourceClaudeLocal, true); err != nil {
		t.Fatalf("enable claude: %v", err)
	}
	if err := repo.SetSource(ctx, SourceCodexLocal, true); err != nil {
		t.Fatalf("enable codex: %v", err)
	}

	blocked := &blockingScanner{
		source:  SourceClaudeLocal,
		account: "a-account",
		files:   []string{aPath},
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	other := &fakeScanner{source: SourceCodexLocal, files: []string{bPath}, parseFn: func([]byte) []Observation {
		pct := 1.0
		return []Observation{{
			AccountKey: "b-account", Provider: "openai", WindowLabel: "5-hour", Kind: KindMeasured,
			UtilizationPct: &pct, ObservedAt: time.Now().UTC(), Source: SourceCodexLocal,
		}}
	}}
	idx := NewIndexer(repo, &fakeLedger{}, staticResolver{}, Accounts{}, []Scanner{blocked, other})
	svc := NewService(repo, nil, nil, Accounts{}, idx, testLogger(t))

	idx.Start()
	<-blocked.entered

	done := make(chan error, 1)
	go func() { done <- svc.SetSource(ctx, SourceCodexLocal, false) }()
	close(blocked.release)
	if err := <-done; err != nil {
		t.Fatalf("disable source: %v", err)
	}

	// The disabled source must not have partial history, and the still-enabled
	// source must be indexed to completion by the restarted pass.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rows, _ := repo.ListObservations(ctx, "a-account", time.Time{})
		if len(rows) > 0 && idx.Status().State == IndexDone {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if rows, _ := repo.ListObservations(ctx, "a-account", time.Time{}); len(rows) == 0 {
		t.Fatal("the still-enabled source was not indexed after the restart")
	}
	if _, ok, _ := repo.GetCursor(ctx, pathHash(aPath)); !ok {
		t.Fatal("the still-enabled source has no cursor")
	}
	if rows, _ := repo.ListObservations(ctx, "b-account", time.Time{}); len(rows) != 0 {
		t.Fatalf("the disabled source must not be indexed, got %d rows", len(rows))
	}
	if _, ok, _ := repo.GetCursor(ctx, pathHash(bPath)); ok {
		t.Fatal("the disabled source must have no cursor")
	}
	if state := idx.Status().State; state != IndexDone {
		t.Fatalf("final state = %q, want done", state)
	}
}

func TestIndexerRerunIndexesNewlyEnabledSource(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	path := filepath.Join(t.TempDir(), "history.jsonl")
	if err := os.WriteFile(path, []byte("one\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	block := make(chan struct{})
	scanner := &fakeScanner{source: SourceCodexLocal, files: []string{path}, parseFn: func([]byte) []Observation {
		pct := 1.0
		return []Observation{{
			AccountKey: "k", Provider: "openai", WindowLabel: "5-hour", Kind: KindMeasured,
			UtilizationPct: &pct, ObservedAt: time.Now().UTC(), Source: SourceCodexLocal,
		}}
	}}
	idx := NewIndexer(repo, &fakeLedger{block: block}, staticResolver{}, Accounts{}, []Scanner{scanner})

	idx.Start()
	if err := repo.SetSource(ctx, SourceCodexLocal, true); err != nil {
		t.Fatalf("enable source: %v", err)
	}
	idx.Start() // requests a rerun while the first pass is blocked
	close(block)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if rows, _ := repo.ListObservations(ctx, "k", time.Time{}); len(rows) > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("a source enabled during a running job was not indexed on the rerun")
}

func TestIndexerAttributionFallback(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	accounts := Accounts{Anthropic: "anthropic-key", OpenAI: "openai-key", Antigravity: "agy-key"}
	ledger := &fakeLedger{events: []LedgerEvent{
		{ID: 1, AgentProfileID: "missing", AgentType: "claude-acp", TokensIn: 1, OccurredAt: time.Now().UTC()},
		{ID: 2, AgentType: "codex-acp", TokensIn: 1, OccurredAt: time.Now().UTC()},
		{ID: 3, AgentType: "unknown-agent", TokensIn: 1, OccurredAt: time.Now().UTC()},
	}}
	idx := NewIndexer(repo, ledger, staticResolver{ok: false}, accounts, nil)
	idx.run(ctx)

	for _, key := range []string{"anthropic-key", "openai-key", "agent:unknown-agent"} {
		rows, _ := repo.ListObservations(ctx, key, time.Time{})
		if len(rows) != 1 {
			t.Fatalf("expected 1 row for %q, got %d", key, len(rows))
		}
	}
}

func appendFile(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open append: %v", err)
	}
	defer func() { _ = file.Close() }()
	if _, err := file.WriteString(content); err != nil {
		t.Fatalf("append: %v", err)
	}
}

func waitForDone(t *testing.T, idx *Indexer, _ context.Context) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if idx.Status().State != IndexRunning {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("index job did not finish")
}
