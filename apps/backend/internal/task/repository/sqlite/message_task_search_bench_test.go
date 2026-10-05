package sqlite

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// WP3 search-performance gate: repository query p95 must be <= 200 ms on the
// reference dataset (100,000 messages across 50 sessions, limit 50). This is
// recorded evidence, not a normal CI assertion, so it only runs when
// KANDEV_TASK_SEARCH_BENCH=1.
const (
	taskSearchBenchMessages = 100_000
	taskSearchBenchSessions = 50
	taskSearchBenchWarmups  = 3
	taskSearchBenchSamples  = 20
	taskSearchBenchBudget   = 200 * time.Millisecond
)

func TestTaskSearchBenchmark(t *testing.T) {
	if os.Getenv("KANDEV_TASK_SEARCH_BENCH") != "1" {
		t.Skip("set KANDEV_TASK_SEARCH_BENCH=1 to run the task-search benchmark")
	}
	repo := newTaskSearchRepo(t)
	seedTaskSearch(t, repo, benchmarkSessionIDs()...)
	if err := seedBenchMessages(t, repo); err != nil {
		t.Fatalf("seed bench messages: %v", err)
	}
	var count int
	if err := repo.ro.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM task_session_messages").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	t.Logf("dataset: %d messages across %d sessions, driver=%s",
		count, taskSearchBenchSessions, repo.ro.DriverName())
	for _, plan := range explainTaskSearch(t, repo, "notpresentzzz") {
		t.Logf("EXPLAIN QUERY PLAN: %s", plan)
	}
	for _, term := range []string{"notpresentzzz", "the"} {
		runTaskSearchScenario(t, repo, term)
	}
}

func benchmarkSessionIDs() []string {
	ids := make([]string, taskSearchBenchSessions)
	for i := range ids {
		ids[i] = fmt.Sprintf("sess-%02d", i)
	}
	return ids
}

func seedBenchMessages(t *testing.T, repo *Repository) error {
	t.Helper()
	ctx := context.Background()
	// One turn per session so every hit joins to a real turn row; half carry a
	// step stamp and half are legacy, mirroring a real backfilled task.
	for i := 0; i < taskSearchBenchSessions; i++ {
		metadata := map[string]interface{}{}
		if i%2 == 0 {
			metadata[models.TurnMetaKeyWorkflowStepIDAtStart] = "step-plan"
		}
		turnID := fmt.Sprintf("turn-%02d", i)
		if err := repo.CreateTurn(ctx, &models.Turn{
			ID: turnID, TaskSessionID: fmt.Sprintf("sess-%02d", i), TaskID: "task-search",
			Metadata: metadata,
		}); err != nil {
			return err
		}
	}

	tx, err := repo.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO task_session_messages
		(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, 'message', '{}', ?, ?)`)
	if err != nil {
		return err
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < taskSearchBenchMessages; i++ {
		session := i % taskSearchBenchSessions
		content := "common word content"
		if i%2 == 0 {
			content = "common the word content"
		}
		at := base.Add(time.Duration(i) * time.Millisecond)
		id := fmt.Sprintf("msg-%06d", i)
		if _, err := stmt.ExecContext(ctx,
			id, fmt.Sprintf("sess-%02d", session), "task-search", fmt.Sprintf("turn-%02d", session),
			"agent", "", content, at, at,
		); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func explainTaskSearch(t *testing.T, repo *Repository, query string) []string {
	t.Helper()
	opts := models.SearchTaskMessagesOptions{Query: query, ActiveSessionID: "sess-00", Limit: 50}
	sqlText, args := buildTaskSearchQuery(repo.ro.DriverName(), "task-search", opts, "%"+query+"%", 50)
	rows, err := repo.ro.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+sqlText, args...)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var plans []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatalf("scan explain: %v", err)
		}
		plans = append(plans, detail)
	}
	return plans
}

func runTaskSearchScenario(t *testing.T, repo *Repository, term string) {
	t.Helper()
	opts := models.SearchTaskMessagesOptions{Query: term, ActiveSessionID: "sess-00", Limit: 50}
	run := func() time.Duration {
		start := time.Now()
		hits, _, err := repo.SearchTaskMessages(context.Background(), "task-search", opts)
		if err != nil {
			t.Fatalf("search %q: %v", term, err)
		}
		_ = hits
		return time.Since(start)
	}
	for i := 0; i < taskSearchBenchWarmups; i++ {
		run()
	}
	samples := make([]time.Duration, 0, taskSearchBenchSamples)
	for i := 0; i < taskSearchBenchSamples; i++ {
		samples = append(samples, run())
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	p50 := samples[len(samples)/2]
	p95 := samples[(len(samples)*95)/100]
	max := samples[len(samples)-1]
	verdict := "within"
	if p95 > taskSearchBenchBudget {
		verdict = "OVER"
	}
	t.Logf("term=%q samples=%s", term, formatDurations(samples))
	t.Logf("term=%q p50=%s p95=%s max=%s budget=%s verdict=%s",
		term, p50, p95, max, taskSearchBenchBudget, verdict)
	if p95 > taskSearchBenchBudget {
		t.Errorf("task-search p95 %s exceeds budget %s; stop frontend rollout and add native FTS", p95, taskSearchBenchBudget)
	}
}

func formatDurations(values []time.Duration) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = value.String()
	}
	return strings.Join(parts, ",")
}
