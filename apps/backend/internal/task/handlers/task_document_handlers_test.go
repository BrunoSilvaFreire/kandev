package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func newDocumentTestHandlers(t *testing.T) *TaskHandlers {
	t.Helper()
	h, repo := newPlanTestHandlersWithRepo(t)
	h.documentService = service.NewDocumentService(repo, h.logger)
	h.documentCatalog = service.NewDocumentCatalogService(h.documentService, h.planService, h.logger)
	return h
}

func TestTaskDocumentsCatalog_MergesPlanAndDocuments(t *testing.T) {
	h := newDocumentTestHandlers(t)
	ctx := context.Background()

	if _, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID: planTaskID, Title: "Plan", Content: "# Plan", CreatedBy: "user",
	}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := h.documentService.CreateOrUpdateDocument(ctx, planTaskID, "architecture", "custom", "Architecture", "# Arch", "agent", "Agent"); err != nil {
		t.Fatalf("create document: %v", err)
	}

	out, err := h.wsListTaskDocumentsCatalog(ctx, planMsg(t, ws.ActionTaskDocumentsCatalog, `{"task_id":"`+planTaskID+`"}`))
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if out.Type != ws.MessageTypeResponse {
		t.Fatalf("type = %q, want response (payload %s)", out.Type, out.Payload)
	}
	var body struct {
		Groups []dto.TaskDocumentCatalogGroupDTO `json:"groups"`
	}
	if err := json.Unmarshal(out.Payload, &body); err != nil {
		t.Fatalf("unmarshal catalog: %v", err)
	}
	var keys []string
	for _, group := range body.Groups {
		for _, entry := range group.Entries {
			keys = append(keys, entry.Key)
		}
	}
	if len(keys) != 2 || keys[0] != "plan" || keys[1] != "architecture" {
		t.Fatalf("catalog keys = %v, want [plan architecture]", keys)
	}
}

func TestTaskDocumentGet_PlanKey(t *testing.T) {
	h := newDocumentTestHandlers(t)
	ctx := context.Background()

	if _, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID: planTaskID, Title: "Plan", Content: "# Plan body", CreatedBy: "user",
	}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	out, err := h.wsGetTaskDocument(ctx, planMsg(t, ws.ActionTaskDocumentGet, `{"task_id":"`+planTaskID+`","key":"plan"}`))
	if err != nil {
		t.Fatalf("get plan document: %v", err)
	}
	var detail dto.TaskDocumentDetailDTO
	if err := json.Unmarshal(out.Payload, &detail); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if !detail.IsPlan || detail.Key != "plan" {
		t.Fatalf("detail = %+v, want plan entry", detail)
	}
	if detail.Content != "# Plan body" {
		t.Fatalf("content = %q, want plan body", detail.Content)
	}
}

func TestTaskDocumentGet_UnknownKey(t *testing.T) {
	h := newDocumentTestHandlers(t)
	out, err := h.wsGetTaskDocument(context.Background(),
		planMsg(t, ws.ActionTaskDocumentGet, `{"task_id":"`+planTaskID+`","key":"missing"}`))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if out.Type != ws.MessageTypeError {
		t.Fatalf("type = %q, want error", out.Type)
	}
}

func TestTaskDocumentRevisions_PlanKey(t *testing.T) {
	h := newDocumentTestHandlers(t)
	ctx := context.Background()

	if _, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID: planTaskID, Title: "Plan", Content: "# Plan", CreatedBy: "user",
	}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := h.planService.UpdatePlan(ctx, service.UpdatePlanRequest{
		TaskID: planTaskID, Title: "Plan", Content: "# Plan v2", CreatedBy: "user",
	}); err != nil {
		t.Fatalf("update plan: %v", err)
	}

	out, err := h.wsListTaskDocumentRevisions(ctx, planMsg(t, ws.ActionTaskDocumentRevisionsList, `{"task_id":"`+planTaskID+`","key":"plan","limit":1}`))
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	var body struct {
		Revisions []dto.TaskDocumentRevisionDTO `json:"revisions"`
	}
	if err := json.Unmarshal(out.Payload, &body); err != nil {
		t.Fatalf("unmarshal revisions: %v", err)
	}
	if len(body.Revisions) != 1 {
		t.Fatalf("revisions = %d, want 1 (limit)", len(body.Revisions))
	}
	if body.Revisions[0].Content != "" {
		t.Fatalf("revision list leaked content")
	}
}

func TestTaskDocumentRevisions_CursorPagination(t *testing.T) {
	h := newDocumentTestHandlers(t)
	ctx := context.Background()

	for _, author := range []string{"A", "B", "C"} {
		if _, err := h.documentService.CreateOrUpdateDocument(ctx, planTaskID, "arch", "custom", "Arch", "v", "agent", author); err != nil {
			t.Fatalf("create revision %s: %v", author, err)
		}
	}

	first := listDocumentRevisionNumbers(t, h, ctx, `{"task_id":"`+planTaskID+`","key":"arch","limit":2}`)
	if len(first) != 2 || first[0] != 3 || first[1] != 2 {
		t.Fatalf("first page = %v, want [3 2]", first)
	}

	second := listDocumentRevisionNumbers(t, h, ctx, `{"task_id":"`+planTaskID+`","key":"arch","limit":2,"before_revision":2}`)
	if len(second) != 1 || second[0] != 1 {
		t.Fatalf("second page = %v, want [1]", second)
	}
}

func TestTaskDocumentRevisions_DefaultLimitIsBounded(t *testing.T) {
	h := newDocumentTestHandlers(t)
	ctx := context.Background()

	if service.NormalizeDocumentRevisionPageSize(0) != service.DefaultDocumentRevisionPageSize {
		t.Fatalf("zero limit must normalize to the server default")
	}
	if service.NormalizeDocumentRevisionPageSize(1_000_000) != service.MaxDocumentRevisionPageSize {
		t.Fatalf("oversized limit must clamp to the server maximum")
	}

	if _, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID: planTaskID, Title: "Plan", Content: "# Plan", CreatedBy: "user",
	}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	out, err := h.wsListTaskDocumentRevisions(ctx, planMsg(t, ws.ActionTaskDocumentRevisionsList,
		`{"task_id":"`+planTaskID+`","key":"plan"}`))
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	var body struct {
		Revisions []dto.TaskDocumentRevisionDTO `json:"revisions"`
	}
	if err := json.Unmarshal(out.Payload, &body); err != nil {
		t.Fatalf("unmarshal revisions: %v", err)
	}
	if len(body.Revisions) != 1 {
		t.Fatalf("plan revisions = %d, want 1", len(body.Revisions))
	}
}

func listDocumentRevisionNumbers(t *testing.T, h *TaskHandlers, ctx context.Context, payload string) []int {
	t.Helper()
	out, err := h.wsListTaskDocumentRevisions(ctx, planMsg(t, ws.ActionTaskDocumentRevisionsList, payload))
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if out.Type != ws.MessageTypeResponse {
		t.Fatalf("type = %q, want response (payload %s)", out.Type, out.Payload)
	}
	var body struct {
		Revisions []dto.TaskDocumentRevisionDTO `json:"revisions"`
	}
	if err := json.Unmarshal(out.Payload, &body); err != nil {
		t.Fatalf("unmarshal revisions: %v", err)
	}
	numbers := make([]int, 0, len(body.Revisions))
	for _, rev := range body.Revisions {
		numbers = append(numbers, rev.RevisionNumber)
	}
	return numbers
}
