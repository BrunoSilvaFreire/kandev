package acp

import (
	"testing"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestLocationsArgsFromACP(t *testing.T) {
	args := locationsArgsFromACP([]acp.ToolCallLocation{
		{Path: "/workspace/src/index.ts"},
	})
	if args == nil {
		t.Fatal("expected args map")
	}
	if got, _ := args[keyPath].(string); got != "/workspace/src/index.ts" {
		t.Fatalf("path = %q, want /workspace/src/index.ts", got)
	}
}

func TestToolCallUpdateSupplemental(t *testing.T) {
	tcu := &acp.SessionToolCallUpdate{
		Locations: []acp.ToolCallLocation{
			{Path: "/workspace/src/index.ts"},
		},
	}

	supplemental := toolCallUpdateSupplemental(tcu)
	if supplemental == nil {
		t.Fatal("expected supplemental map")
	}
	if got, _ := supplemental["path"].(string); got != "/workspace/src/index.ts" {
		t.Fatalf("path = %q, want /workspace/src/index.ts", got)
	}

	n := NewNormalizer("")
	payload := n.NormalizeToolCall("read", map[string]any{
		"kind":      "read",
		"raw_input": map[string]any{},
	})
	n.UpdatePayloadInput(payload, nil, supplemental)
	if got := payload.ReadFile().FilePath; got != "/workspace/src/index.ts" {
		t.Fatalf("ReadFile.FilePath = %q, want /workspace/src/index.ts", got)
	}

	searchPayload := n.NormalizeToolCall("search", map[string]any{
		"kind":      "search",
		"raw_input": map[string]any{},
	})
	n.UpdatePayloadInput(searchPayload, nil, supplemental)
	if got := searchPayload.CodeSearch().Path; got != "/workspace/src/index.ts" {
		t.Fatalf("CodeSearch.Path = %q, want /workspace/src/index.ts", got)
	}
}

// @covers AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.1
func TestConvertToolCallResultUpdateAgentPlanCarriesToolCallID(t *testing.T) {
	adapter := newTestAdapter()
	t.Cleanup(func() { _ = adapter.Close() })

	adapter.convertToolCallResultUpdate("session-1", &acp.SessionToolCallUpdate{
		ToolCallId: "plan-call-1",
		RawInput:   map[string]any{"plan": "# Plan\n\n1. Read"},
	})

	event := <-adapter.updatesCh
	if event.Type != streams.EventTypeAgentPlan {
		t.Fatalf("event type = %q, want %q", event.Type, streams.EventTypeAgentPlan)
	}
	if event.ToolCallID != "plan-call-1" {
		t.Fatalf("tool call id = %q, want plan-call-1", event.ToolCallID)
	}
}

func TestPathFromLocationSlice(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{
			name: "[]any shape from initial tool_call",
			input: []any{
				map[string]any{"path": "/a.go"},
			},
			want: "/a.go",
		},
		{
			name: "[]map[string]any shape from tool_call_update supplemental",
			input: []map[string]any{
				{"path": "/b.go"},
			},
			want: "/b.go",
		},
		{name: "empty slice", input: []any{}, want: ""},
		{name: "nil", input: nil, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pathFromLocationSlice(tt.input); got != tt.want {
				t.Fatalf("pathFromLocationSlice() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHandleToolCallPreCompletedNotTrackedActive(t *testing.T) {
	adapter := newTestAdapter()
	t.Cleanup(func() { _ = adapter.Close() })

	tc := &acp.SessionUpdateToolCall{
		ToolCallId: "reconcile-1",
		Kind:       "edit",
		Status:     acp.ToolCallStatus(toolStatusCompleted),
		RawInput: map[string]any{
			"TargetFile": "/workspace/main.go",
		},
	}

	ev := adapter.convertToolCallUpdate("session-1", tc)
	if ev == nil {
		t.Fatal("expected event from convertToolCallUpdate")
	}
	if ev.ToolStatus != toolStatusCompleted {
		t.Fatalf("expected ToolStatus %q, got %q", toolStatusCompleted, ev.ToolStatus)
	}

	adapter.mu.Lock()
	_, active := adapter.activeToolCalls["reconcile-1"]
	adapter.mu.Unlock()

	if active {
		t.Fatal("expected pre-completed tool call not to be tracked in activeToolCalls")
	}
}

func TestConvertToolCallUpdateEnrichModifyFileFromContents(t *testing.T) {
	adapter := newTestAdapter()
	t.Cleanup(func() { _ = adapter.Close() })

	oldText := "old\n"
	tc := &acp.SessionUpdateToolCall{
		ToolCallId: "edit-1",
		Kind:       "edit",
		Status:     acp.ToolCallStatus(toolStatusCompleted),
		RawInput: map[string]any{
			"TargetFile": "/workspace/file.go",
		},
		Content: []acp.ToolCallContent{
			{
				Diff: &acp.ToolCallContentDiff{
					Path:    "/workspace/file.go",
					OldText: &oldText,
					NewText: "new\n",
				},
			},
		},
	}

	ev := adapter.convertToolCallUpdate("session-1", tc)
	if ev == nil {
		t.Fatal("expected event from convertToolCallUpdate")
	}
	mf := ev.NormalizedPayload.ModifyFile()
	if mf == nil {
		t.Fatal("expected ModifyFile payload")
	}
	if mf.FilePath != "/workspace/file.go" {
		t.Fatalf("expected FilePath /workspace/file.go, got %q", mf.FilePath)
	}
	if len(mf.Mutations) == 0 || mf.Mutations[0].Diff == "" {
		t.Fatalf("expected diff in Mutations[0], got %+v", mf.Mutations)
	}
}
