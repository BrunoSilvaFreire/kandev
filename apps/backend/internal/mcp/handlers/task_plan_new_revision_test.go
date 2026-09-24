package handlers

import (
	"context"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestMCPUpdateTaskPlan_NewRevisionSkipsCoalescing verifies that new_revision
// keeps a superseded plan addressable even when the same author rewrites it
// inside the coalesce window, where a plain update merges in place.
func TestMCPUpdateTaskPlan_NewRevisionSkipsCoalescing(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()

	write := func(handler func(context.Context, *ws.Message) (*ws.Message, error), action string, payload map[string]any) string {
		t.Helper()
		out, err := handler(ctx, mcpPlanMsg(t, action, mustMarshalPlanPayload(t, payload)))
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if out.Type == ws.MessageTypeError {
			t.Fatalf("%s returned error: %s", action, string(out.Payload))
		}
		version, _ := decodeMCPPlanPayload(t, out)["version"].(string)
		return version
	}

	version := write(h.handleCreateTaskPlan, ws.ActionMCPCreateTaskPlan,
		map[string]any{"task_id": mcpPlanTaskID, "content": "plan one draft"})
	version = write(h.handleUpdateTaskPlan, ws.ActionMCPUpdateTaskPlan,
		map[string]any{"task_id": mcpPlanTaskID, "content": "plan one final", "expected_version": version})

	revisions, err := h.planService.ListRevisions(ctx, mcpPlanTaskID)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revisions) != 1 {
		t.Fatalf("plain same-author update should coalesce: got %d revisions, want 1", len(revisions))
	}

	write(h.handleUpdateTaskPlan, ws.ActionMCPUpdateTaskPlan, map[string]any{
		"task_id": mcpPlanTaskID, "content": "plan two revised", "expected_version": version, "new_revision": true,
	})

	revisions, err = h.planService.ListRevisions(ctx, mcpPlanTaskID)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revisions) != 2 {
		t.Fatalf("new_revision update should append: got %d revisions, want 2", len(revisions))
	}
	rev1, err := h.planService.GetRevision(ctx, revisionWithNumber(t, revisions, 1).ID)
	if err != nil {
		t.Fatalf("GetRevision(rev1): %v", err)
	}
	if rev1.Content != "plan one final" {
		t.Fatalf("superseded revision content = %q, want %q", rev1.Content, "plan one final")
	}
}
