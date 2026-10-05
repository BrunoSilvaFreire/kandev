package providerusage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type ledgerFixture struct {
	taskID     string
	sessionID  string
	agentType  string
	model      string
	provider   string
	tokensIn   int64
	tokensOut  int64
	cachedRead int64
	cost       int64
	at         time.Time
}

func breakdownTestRepository(t *testing.T) *Repository {
	t.Helper()
	repo := newTestRepository(t)
	statements := []string{
		`CREATE TABLE tasks (id TEXT PRIMARY KEY, title TEXT)`,
		`CREATE TABLE task_sessions (id TEXT PRIMARY KEY, name TEXT)`,
		`CREATE TABLE task_usage_events (
			task_id TEXT NOT NULL,
			session_id TEXT,
			agent_type TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			tokens_in BIGINT NOT NULL DEFAULT 0,
			tokens_cached_read BIGINT,
			tokens_cached_write BIGINT,
			tokens_out BIGINT,
			tokens_total BIGINT NOT NULL DEFAULT 0,
			cost_subcents BIGINT NOT NULL DEFAULT 0,
			occurred_at TIMESTAMP NOT NULL
		)`,
	}
	for _, statement := range statements {
		if _, err := repo.writer.Exec(statement); err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	return repo
}

func seedBreakdownTask(t *testing.T, repo *Repository, id, title string) {
	t.Helper()
	if _, err := repo.writer.Exec(`INSERT INTO tasks (id, title) VALUES (?, ?)`, id, title); err != nil {
		t.Fatalf("insert task: %v", err)
	}
}

func seedBreakdownSession(t *testing.T, repo *Repository, id, name string) {
	t.Helper()
	if _, err := repo.writer.Exec(`INSERT INTO task_sessions (id, name) VALUES (?, ?)`, id, name); err != nil {
		t.Fatalf("insert session: %v", err)
	}
}

func seedLedgerEvent(t *testing.T, repo *Repository, event ledgerFixture) {
	t.Helper()
	total := event.tokensIn + event.tokensOut + event.cachedRead
	if _, err := repo.writer.Exec(
		`INSERT INTO task_usage_events
			(task_id, session_id, agent_type, model, provider, tokens_in, tokens_out,
			 tokens_cached_read, tokens_total, cost_subcents, occurred_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.taskID, event.sessionID, event.agentType, event.model, event.provider,
		event.tokensIn, event.tokensOut, event.cachedRead, total, event.cost, event.at,
	); err != nil {
		t.Fatalf("insert usage event: %v", err)
	}
}

func seedStandardLedger(t *testing.T, repo *Repository) time.Time {
	t.Helper()
	now := time.Now().UTC().Truncate(24 * time.Hour).Add(12 * time.Hour)
	seedBreakdownTask(t, repo, "task-a", "Alpha task")
	seedBreakdownSession(t, repo, "sess-a", "Alpha session")
	seedBreakdownTask(t, repo, "task-b", "Beta task")
	seedBreakdownSession(t, repo, "sess-b", "Beta session")
	events := []ledgerFixture{
		{taskID: "task-a", sessionID: "sess-a", agentType: AgentTypeClaude, model: "claude-sonnet", provider: "", tokensIn: 10, tokensOut: 5, cachedRead: 2, cost: 100, at: now.Add(-time.Hour)},
		{taskID: "task-a", sessionID: "sess-a", agentType: AgentTypeClaude, model: "claude-opus", provider: "", tokensIn: 20, tokensOut: 10, cachedRead: 3, cost: 200, at: now.Add(-2 * time.Hour)},
		{taskID: "task-b", sessionID: "sess-b", agentType: AgentTypeCodex, model: "gpt-5", provider: "openai", tokensIn: 7, tokensOut: 3, cachedRead: 1, cost: 50, at: now.Add(-3 * time.Hour)},
	}
	for _, event := range events {
		seedLedgerEvent(t, repo, event)
	}
	return now
}

func TestBreakdownGroupByDimensions(t *testing.T) {
	ctx := context.Background()
	repo := breakdownTestRepository(t)
	now := seedStandardLedger(t, repo)
	since := now.Add(-24 * time.Hour)

	cases := map[string]int{
		BreakdownGroupSession:  2,
		BreakdownGroupTask:     2,
		BreakdownGroupProvider: 2,
		BreakdownGroupAgent:    2,
		BreakdownGroupModel:    3,
		BreakdownGroupDay:      1,
	}
	for groupBy, want := range cases {
		response, err := repo.Breakdown(ctx, BreakdownQuery{GroupBy: groupBy}, since)
		if err != nil {
			t.Fatalf("group_by %s: %v", groupBy, err)
		}
		if len(response.Rows) != want {
			t.Fatalf("group_by %s: rows = %d, want %d: %+v", groupBy, len(response.Rows), want, response.Rows)
		}
	}
}

func TestBreakdownSessionRowCarriesIdentity(t *testing.T) {
	ctx := context.Background()
	repo := breakdownTestRepository(t)
	now := seedStandardLedger(t, repo)

	response, err := repo.Breakdown(ctx, BreakdownQuery{GroupBy: BreakdownGroupSession}, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("breakdown: %v", err)
	}
	var alpha *BreakdownRow
	for i := range response.Rows {
		if response.Rows[i].Key == "sess-a" {
			alpha = &response.Rows[i]
		}
	}
	if alpha == nil {
		t.Fatalf("no sess-a row: %+v", response.Rows)
	}
	if alpha.TaskID != "task-a" || alpha.Label != "Alpha task Alpha session" {
		t.Fatalf("identity = %q/%q", alpha.TaskID, alpha.Label)
	}
	if alpha.Provider != ProviderAnthropic || alpha.Model != "claude-sonnet" || alpha.Models != 2 {
		t.Fatalf("provider/model = %q/%q models=%d", alpha.Provider, alpha.Model, alpha.Models)
	}
	if alpha.TokensTotal != 50 || alpha.Events != 2 || alpha.CostSubcents != 300 {
		t.Fatalf("aggregate = %+v", alpha)
	}
}

func TestBreakdownSortsAndTieBreak(t *testing.T) {
	ctx := context.Background()
	repo := breakdownTestRepository(t)
	now := seedStandardLedger(t, repo)
	since := now.Add(-24 * time.Hour)

	tokensDesc, err := repo.Breakdown(ctx, BreakdownQuery{GroupBy: BreakdownGroupSession, Sort: BreakdownSortTokens, Order: BreakdownOrderDesc}, since)
	if err != nil {
		t.Fatalf("tokens desc: %v", err)
	}
	if tokensDesc.Rows[0].Key != "sess-a" {
		t.Fatalf("tokens desc first = %q", tokensDesc.Rows[0].Key)
	}
	tokensAsc, err := repo.Breakdown(ctx, BreakdownQuery{GroupBy: BreakdownGroupSession, Sort: BreakdownSortTokens, Order: BreakdownOrderAsc}, since)
	if err != nil {
		t.Fatalf("tokens asc: %v", err)
	}
	if tokensAsc.Rows[0].Key != "sess-b" {
		t.Fatalf("tokens asc first = %q", tokensAsc.Rows[0].Key)
	}
	lastDesc, err := repo.Breakdown(ctx, BreakdownQuery{GroupBy: BreakdownGroupSession, Sort: BreakdownSortLastAt, Order: BreakdownOrderDesc}, since)
	if err != nil {
		t.Fatalf("last desc: %v", err)
	}
	if lastDesc.Rows[0].Key != "sess-a" {
		t.Fatalf("last desc first = %q", lastDesc.Rows[0].Key)
	}
	events, err := repo.Breakdown(ctx, BreakdownQuery{GroupBy: BreakdownGroupSession, Sort: BreakdownSortEvents, Order: BreakdownOrderDesc}, since)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if events.Rows[0].Key != "sess-a" {
		t.Fatalf("events first = %q", events.Rows[0].Key)
	}
	cost, err := repo.Breakdown(ctx, BreakdownQuery{GroupBy: BreakdownGroupModel, Sort: BreakdownSortCost, Order: BreakdownOrderAsc}, since)
	if err != nil {
		t.Fatalf("cost: %v", err)
	}
	if cost.Rows[0].Key != "gpt-5" {
		t.Fatalf("cost asc first = %q", cost.Rows[0].Key)
	}
}

func TestBreakdownProviderFilterUsesLedgerMapping(t *testing.T) {
	ctx := context.Background()
	repo := breakdownTestRepository(t)
	now := seedStandardLedger(t, repo)

	response, err := repo.Breakdown(ctx, BreakdownQuery{Provider: ProviderAnthropic}, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("breakdown: %v", err)
	}
	if len(response.Rows) != 1 || response.Rows[0].Key != "sess-a" {
		t.Fatalf("anthropic rows = %+v", response.Rows)
	}
	if response.TotalEvents != 2 {
		t.Fatalf("total events = %d, want 2", response.TotalEvents)
	}
}

func TestBreakdownTextSearchIsCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	repo := breakdownTestRepository(t)
	now := seedStandardLedger(t, repo)

	byTitle, err := repo.Breakdown(ctx, BreakdownQuery{Q: "alpha"}, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("title search: %v", err)
	}
	if len(byTitle.Rows) != 1 || byTitle.Rows[0].Key != "sess-a" {
		t.Fatalf("title search rows = %+v", byTitle.Rows)
	}
	byModel, err := repo.Breakdown(ctx, BreakdownQuery{Q: "GPT-5"}, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("model search: %v", err)
	}
	if len(byModel.Rows) != 1 || byModel.Rows[0].Key != "sess-b" {
		t.Fatalf("model search rows = %+v", byModel.Rows)
	}
}

func TestBreakdownFiltersByModelAgentTaskSession(t *testing.T) {
	ctx := context.Background()
	repo := breakdownTestRepository(t)
	now := seedStandardLedger(t, repo)
	since := now.Add(-24 * time.Hour)

	for _, query := range []BreakdownQuery{
		{Model: "gpt-5"},
		{AgentType: AgentTypeCodex},
		{TaskID: "task-b"},
		{SessionID: "sess-b"},
	} {
		response, err := repo.Breakdown(ctx, query, since)
		if err != nil {
			t.Fatalf("filter %+v: %v", query, err)
		}
		if response.TotalEvents != 1 {
			t.Fatalf("filter %+v events = %d", query, response.TotalEvents)
		}
	}
}

func TestBreakdownLeftJoinKeepsDeletedTaskRows(t *testing.T) {
	ctx := context.Background()
	repo := breakdownTestRepository(t)
	now := seedStandardLedger(t, repo)
	seedBreakdownSession(t, repo, "sess-ghost", "Ghost session")
	seedLedgerEvent(t, repo, ledgerFixture{
		taskID: "task-deleted", sessionID: "sess-ghost", agentType: AgentTypeClaude,
		model: "claude-sonnet", tokensIn: 1, cost: 9, at: now.Add(-time.Hour),
	})

	response, err := repo.Breakdown(ctx, BreakdownQuery{SessionID: "sess-ghost"}, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("breakdown: %v", err)
	}
	if len(response.Rows) != 1 {
		t.Fatalf("rows = %+v", response.Rows)
	}
	if response.Rows[0].Label != "Ghost session" || response.Rows[0].TaskID != "task-deleted" {
		t.Fatalf("deleted-task row = %+v", response.Rows[0])
	}
}

func TestBreakdownRangeCutoffAndPagination(t *testing.T) {
	ctx := context.Background()
	repo := breakdownTestRepository(t)
	now := time.Now().UTC().Truncate(time.Second)
	seedBreakdownTask(t, repo, "task-a", "Alpha")
	seedBreakdownSession(t, repo, "sess-a", "Alpha")
	for i := 0; i < 5; i++ {
		seedLedgerEvent(t, repo, ledgerFixture{
			taskID: "task-a", sessionID: "sess-a", agentType: AgentTypeClaude,
			model: "m", tokensIn: int64(i + 1), at: now.Add(-time.Duration(i+1) * time.Hour),
		})
	}
	// Outside the 24h window.
	seedLedgerEvent(t, repo, ledgerFixture{
		taskID: "task-a", sessionID: "sess-a", agentType: AgentTypeClaude,
		model: "m", tokensIn: 999, at: now.Add(-48 * time.Hour),
	})

	page, err := repo.Breakdown(ctx, BreakdownQuery{GroupBy: BreakdownGroupModel, Limit: 1, Offset: 0}, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("breakdown: %v", err)
	}
	if page.TotalEvents != 5 || page.TotalTokens != 15 {
		t.Fatalf("totals = events %d tokens %d", page.TotalEvents, page.TotalTokens)
	}
	if len(page.Rows) != 1 {
		t.Fatalf("page rows = %d", len(page.Rows))
	}
	if page.TotalRows != 1 {
		t.Fatalf("total rows = %d", page.TotalRows)
	}
}

func TestBreakdownFacets(t *testing.T) {
	ctx := context.Background()
	repo := breakdownTestRepository(t)
	now := seedStandardLedger(t, repo)

	response, err := repo.Breakdown(ctx, BreakdownQuery{}, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("breakdown: %v", err)
	}
	if len(response.Facets.Providers) != 2 || len(response.Facets.Models) != 3 {
		t.Fatalf("facets = %+v", response.Facets)
	}
	if len(response.Facets.AgentTypes) != 2 {
		t.Fatalf("agent facets = %+v", response.Facets.AgentTypes)
	}
}

func TestBreakdownFacetsSkipBlankValues(t *testing.T) {
	ctx := context.Background()
	repo := breakdownTestRepository(t)
	now := seedStandardLedger(t, repo)
	seedLedgerEvent(t, repo, ledgerFixture{
		taskID: "task-unknown", sessionID: "sess-unknown", agentType: "", provider: "",
		model: "unknown-model", tokensIn: 1, at: now.Add(-time.Hour),
	})

	response, err := repo.Breakdown(ctx, BreakdownQuery{}, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("breakdown: %v", err)
	}
	for _, provider := range response.Facets.Providers {
		if provider == "" {
			t.Fatalf("blank provider leaked into facets: %+v", response.Facets)
		}
	}
	for _, agent := range response.Facets.AgentTypes {
		if agent == "" {
			t.Fatalf("blank agent leaked into facets: %+v", response.Facets)
		}
	}
}

func TestBreakdownNormalizeRejectsUnknownEnums(t *testing.T) {
	for _, query := range []BreakdownQuery{
		{GroupBy: "bogus"},
		{Sort: "bogus"},
		{Order: "sideways"},
	} {
		if err := query.normalize(); err == nil {
			t.Fatalf("query %+v: expected error", query)
		}
	}
	query := BreakdownQuery{Limit: 5000, Offset: -3}
	if err := query.normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if query.Limit != breakdownMaxLimit || query.Offset != 0 || query.GroupBy != BreakdownGroupSession {
		t.Fatalf("normalized = %+v", query)
	}
}

func TestProviderCaseExprMatchesLedgerProvider(t *testing.T) {
	repo := breakdownTestRepository(t)
	agentTypes := []string{
		AgentTypeClaude, AgentTypeCodex, AgentTypeAgy, AgentTypeAntigravity,
		AgentTypeGemini, "unknown-acp",
	}
	for _, agentType := range agentTypes {
		for _, provider := range []string{"", "custom-provider"} {
			var got string
			err := repo.reader.QueryRow(
				"SELECT "+providerCaseExpr()+" FROM (SELECT ? AS agent_type, ? AS provider) e",
				agentType, provider,
			).Scan(&got)
			if err != nil {
				t.Fatalf("query %s/%s: %v", agentType, provider, err)
			}
			if want := ledgerProvider(agentType, provider); got != want {
				t.Fatalf("agent=%s provider=%s: sql %q != ledger %q", agentType, provider, got, want)
			}
		}
	}
}

func newBreakdownService(t *testing.T) *Service {
	t.Helper()
	repo := breakdownTestRepository(t)
	indexer := NewIndexer(repo, &fakeLedger{}, staticResolver{}, Accounts{}, nil)
	return NewService(repo, &fakeLive{}, twoProfileAgents(), Accounts{Anthropic: "a", OpenAI: "o"}, indexer, testLogger(t))
}

func TestBreakdownHandlerResponses(t *testing.T) {
	svc := newBreakdownService(t)
	router := newTestEngine(t, svc)

	ok := httptest.NewRecorder()
	router.ServeHTTP(ok, httptest.NewRequest(http.MethodGet, "/api/v1/provider-usage/breakdown?group_by=session", nil))
	if ok.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", ok.Code, ok.Body.String())
	}
	var payload BreakdownResponse
	if err := json.Unmarshal(ok.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.GroupBy != BreakdownGroupSession || payload.Rows == nil {
		t.Fatalf("payload = %+v", payload)
	}

	for _, path := range []string{
		"/api/v1/provider-usage/breakdown?group_by=bogus",
		"/api/v1/provider-usage/breakdown?sort=bogus",
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d", path, recorder.Code)
		}
	}

	clamped := httptest.NewRecorder()
	router.ServeHTTP(clamped, httptest.NewRequest(http.MethodGet, "/api/v1/provider-usage/breakdown?limit=1000", nil))
	if clamped.Code != http.StatusOK {
		t.Fatalf("clamped status = %d", clamped.Code)
	}
}
