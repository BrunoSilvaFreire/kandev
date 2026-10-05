package providerusage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func newTestRepository(t *testing.T) *Repository {
	t.Helper()
	path := filepath.Join(t.TempDir(), "usage.db")
	db, err := sqlx.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	repo, err := New(db, db)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	return repo
}

func pct(v float64) *float64 { return &v }
func toks(v int64) *int64    { return &v }

func TestObservationRoundTripAndIdempotency(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	when := time.Now().UTC().Truncate(time.Second)

	measured := Observation{
		AccountKey: "acct-1", Provider: "anthropic", WindowLabel: "5-hour",
		Kind: KindMeasured, UtilizationPct: pct(42), ResetAt: when.Add(time.Hour),
		ObservedAt: when, Source: SourceKandev,
	}
	if err := repo.InsertObservations(ctx, []Observation{measured, measured}); err != nil {
		t.Fatalf("insert measured: %v", err)
	}

	tokenRow := Observation{
		AccountKey: "acct-1", Provider: "anthropic", WindowLabel: "tokens",
		Kind: KindTokens, WeightedTokens: toks(100), Model: "claude", ObservedAt: when, Source: SourceKandev,
	}
	if err := repo.InsertObservations(ctx, []Observation{tokenRow, tokenRow}); err != nil {
		t.Fatalf("insert tokens: %v", err)
	}

	rows, err := repo.ListObservations(ctx, "acct-1", when.Add(-time.Hour))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (measured de-duped, tokens summed), got %d: %+v", len(rows), rows)
	}
	var tokenTotal int64
	for _, row := range rows {
		if row.Kind == KindTokens {
			tokenTotal = *row.WeightedTokens
		}
	}
	if tokenTotal != 200 {
		t.Fatalf("expected tokens summed to 200, got %d", tokenTotal)
	}
}

func TestListObservationsSince(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	old := time.Now().UTC().Add(-48 * time.Hour)
	if err := repo.InsertObservations(ctx, []Observation{{
		AccountKey: "a", Provider: "p", WindowLabel: "5-hour", Kind: KindMeasured,
		UtilizationPct: pct(1), ObservedAt: old, Source: SourceKandev,
	}}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	rows, err := repo.ListObservations(ctx, "a", old.Add(time.Hour))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected the old row to be filtered, got %d", len(rows))
	}
}

func TestLatestLimitHitReturnsNewest(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	old := time.Now().UTC().Add(-time.Hour)
	newer := old.Add(time.Minute)
	if err := repo.InsertObservations(ctx, []Observation{
		{AccountKey: "a", Provider: "openai", WindowLabel: "limit", Kind: KindLimitHit, ResetAt: old.Add(time.Hour), ObservedAt: old, Source: SourceRoutingError},
		{AccountKey: "a", Provider: "openai", WindowLabel: "limit", Kind: KindLimitHit, ResetAt: newer.Add(time.Hour), ObservedAt: newer, Source: SourceRoutingError},
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	observed, reset, ok, err := repo.LatestLimitHit(ctx, "a")
	if err != nil || !ok || !observed.Equal(newer) || !reset.Equal(newer.Add(time.Hour)) {
		t.Fatalf("latest = %v/%v/%v/%v", observed, reset, ok, err)
	}
}

func TestPrune(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	old := time.Now().UTC().Add(-100 * 24 * time.Hour)
	if err := repo.InsertObservations(ctx, []Observation{{
		AccountKey: "a", Provider: "p", WindowLabel: "5-hour", Kind: KindMeasured,
		UtilizationPct: pct(1), ObservedAt: old, Source: SourceKandev,
	}}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := repo.Prune(ctx, time.Now().UTC().Add(-90*24*time.Hour)); err != nil {
		t.Fatalf("prune: %v", err)
	}
	rows, _ := repo.ListObservations(ctx, "a", old.Add(-time.Hour))
	if len(rows) != 0 {
		t.Fatalf("expected pruned row to be gone, got %d", len(rows))
	}
}

func TestCursorRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	if _, ok, err := repo.GetCursor(ctx, "kandev:task_usage_events"); err != nil || ok {
		t.Fatalf("expected no cursor, got ok=%v err=%v", ok, err)
	}
	cursor := FileCursor{PathHash: "kandev:task_usage_events", Source: SourceKandev, ByteOffset: 5000, Size: 1}
	if err := repo.PutCursor(ctx, cursor); err != nil {
		t.Fatalf("put cursor: %v", err)
	}
	cursor.ByteOffset = 9000
	if err := repo.PutCursor(ctx, cursor); err != nil {
		t.Fatalf("update cursor: %v", err)
	}
	got, ok, err := repo.GetCursor(ctx, "kandev:task_usage_events")
	if err != nil || !ok {
		t.Fatalf("get cursor: ok=%v err=%v", ok, err)
	}
	if got.ByteOffset != 9000 {
		t.Fatalf("expected offset 9000, got %d", got.ByteOffset)
	}
}

func TestSourcesAndDelete(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)
	sources, err := repo.ListSources(ctx)
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	if len(sources) != 0 {
		t.Fatalf("expected no rows by default, got %+v", sources)
	}
	if SourceEnabled(sources, SourceClaudeLocal) {
		t.Fatal("local source must default to off")
	}
	if !SourceEnabled(sources, SourceKandev) {
		t.Fatal("kandev source must always be enabled")
	}

	if err := repo.SetSource(ctx, SourceClaudeLocal, true); err != nil {
		t.Fatalf("set source: %v", err)
	}
	if err := repo.InsertObservations(ctx, []Observation{{
		AccountKey: "a", Provider: "anthropic", WindowLabel: "tokens", Kind: KindTokens,
		WeightedTokens: toks(5), ObservedAt: time.Now().UTC(), Source: SourceClaudeLocal,
	}}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := repo.PutCursor(ctx, FileCursor{PathHash: "claude:x", Source: SourceClaudeLocal}); err != nil {
		t.Fatalf("put cursor: %v", err)
	}
	if err := repo.DeleteSource(ctx, SourceClaudeLocal); err != nil {
		t.Fatalf("delete source: %v", err)
	}
	rows, _ := repo.ListObservations(ctx, "a", time.Time{})
	if len(rows) != 0 {
		t.Fatalf("expected observations deleted with the source, got %d", len(rows))
	}
	if _, ok, _ := repo.GetCursor(ctx, "claude:x"); ok {
		t.Fatal("expected cursor deleted with the source")
	}
}
