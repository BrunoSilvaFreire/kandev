package mcp

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// relatedTasksResponse is a related-task graph shaped like the backend's, with
// one description long enough that dropping it is visible in the payload size.
func relatedTasksResponse(description string) map[string]interface{} {
	return map[string]interface{}{
		"task": map[string]interface{}{
			"id": "task-A", "title": "Parent work", "state": "RUNNING", "description": description,
		},
		"parent": map[string]interface{}{
			"id": "task-P", "title": "Epic", "state": "RUNNING", "description": description,
		},
		"children": []interface{}{
			map[string]interface{}{
				"id": "task-C1", "title": "Child one", "state": "CREATED",
				"description": description, "document_keys": []interface{}{"spec"},
			},
			map[string]interface{}{
				"id": "task-C2", "title": "Child two", "state": "DONE", "description": description,
			},
		},
		"siblings": []interface{}{
			map[string]interface{}{
				"id": "task-S1", "title": "Sibling task", "state": "RUNNING", "description": description,
			},
		},
		"blockers":   []interface{}{},
		"blocked_by": []interface{}{},
	}
}

func relatedTasksText(t *testing.T, args map[string]interface{}, description string) string {
	t.Helper()
	backend := &testBackend{response: relatedTasksResponse(description)}
	s := newTaskModeServer(t, backend, "task-A")

	result := callTool(t, s, "list_related_tasks_kandev", args)
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	return text.Text
}

func TestListRelatedTasks_OmitsDescriptionsByDefault(t *testing.T) {
	description := "a long task description that dominates the response size"

	text := relatedTasksText(t, map[string]interface{}{}, description)

	assert.NotContains(t, text, description)
	assert.NotContains(t, text, `"description"`)
	// The compact projection must still carry everything the caller navigates by.
	for _, want := range []string{"task-A", "task-P", "task-C1", "task-C2", "task-S1", "Child one", "CREATED", "spec"} {
		assert.Contains(t, text, want)
	}
}

func TestListRelatedTasks_VerboseKeepsDescriptions(t *testing.T) {
	description := "Depends on: task-C1"

	text := relatedTasksText(t, map[string]interface{}{"verbose": true}, description)

	assert.Contains(t, text, description)
	assert.Contains(t, text, `"description"`)
}

func TestListRelatedTasks_ToolSchemaExposesVerbose(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-A")

	tool, ok := s.mcpServer.ListTools()["list_related_tasks_kandev"]
	require.True(t, ok)

	schema, err := json.Marshal(tool.Tool.InputSchema)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(schema, &parsed))

	props, ok := parsed["properties"].(map[string]interface{})
	require.True(t, ok)
	verbose, ok := props["verbose"].(map[string]interface{})
	require.True(t, ok, "verbose must be advertised so callers can opt into descriptions")
	assert.Equal(t, "boolean", verbose["type"])

	required, _ := parsed["required"].([]interface{})
	assert.NotContains(t, required, "verbose")
	assert.Contains(t, tool.Tool.Description, "verbose=true")
}

// documentToolPayload runs a document tool and returns the payload the handler
// forwarded to the backend as a string map, failing when the call errored.
// The list/get handlers send a map[string]string while write sends a
// map[string]interface{}, so the payload is normalized through JSON.
func documentToolPayload(t *testing.T, toolName string, args map[string]interface{}, callerTaskID string) map[string]string {
	t.Helper()
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, callerTaskID)

	result := callTool(t, s, toolName, args)
	require.False(t, result.IsError, "tool %q returned an error: %v", toolName, result.Content)

	data, err := json.Marshal(backend.lastPayload)
	require.NoError(t, err)
	payload := map[string]string{}
	require.NoError(t, json.Unmarshal(data, &payload))
	return payload
}

func TestTaskDocumentTools_DefaultToCallerTask(t *testing.T) {
	cases := []struct {
		tool string
		args map[string]interface{}
	}{
		{"list_task_documents_kandev", map[string]interface{}{}},
		{"get_task_document_kandev", map[string]interface{}{"document_key": "spike"}},
		{"write_task_document_kandev", map[string]interface{}{"document_key": "spike", "content": "body"}},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			payload := documentToolPayload(t, tc.tool, tc.args, "task-A")
			assert.Equal(t, "task-A", payload["task_id"], "omitted task_id must resolve to the caller's task")
			assert.Equal(t, "task-A", payload["caller_task_id"])
		})
	}
}

func TestTaskDocumentTools_SelfResolvesToCallerTask(t *testing.T) {
	payload := documentToolPayload(t, "write_task_document_kandev", map[string]interface{}{
		"task_id":      "self",
		"document_key": "spike",
		"content":      "body",
	}, "task-A")
	assert.Equal(t, "task-A", payload["task_id"])
	assert.Equal(t, "task-A", payload["caller_task_id"])
}

func TestTaskDocumentTools_ExplicitTargetIsForwarded(t *testing.T) {
	payload := documentToolPayload(t, "list_task_documents_kandev", map[string]interface{}{
		"task_id": "task-B",
	}, "task-A")
	assert.Equal(t, "task-B", payload["task_id"], "an explicit target must not be overwritten by the default")
	assert.Equal(t, "task-A", payload["caller_task_id"])
}

func TestTaskDocumentTools_EmptyTaskContextErrors(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "")

	result := callTool(t, s, "list_task_documents_kandev", map[string]interface{}{})
	require.True(t, result.IsError, "no task context must fail closed rather than look up a blank id")
	require.NotEmpty(t, result.Content)
}

func TestTaskDocumentTools_AccessDeniedSurfacesAsError(t *testing.T) {
	backend := &testBackend{err: errors.New("access_denied: caller may not reach target task")}
	s := newTaskModeServer(t, backend, "task-A")

	result := callTool(t, s, "write_task_document_kandev", map[string]interface{}{
		"task_id":      "stranger",
		"document_key": "spike",
		"content":      "body",
	})
	require.True(t, result.IsError, "a backend access denial must surface as a tool error")
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, "access_denied")
}

// toolRequiredNames returns the schema's `required` names for a registered tool.
func toolRequiredNames(t *testing.T, s *Server, toolName string) []interface{} {
	t.Helper()
	st, ok := s.mcpServer.ListTools()[toolName]
	require.True(t, ok, "tool %q not registered", toolName)
	schema, err := json.Marshal(st.Tool.InputSchema)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(schema, &parsed))
	required, _ := parsed["required"].([]interface{})
	return required
}

func TestTaskDocumentTools_TaskIDIsOptionalInSchema(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-A")
	for _, toolName := range []string{
		"list_task_documents_kandev",
		"get_task_document_kandev",
		"write_task_document_kandev",
	} {
		t.Run(toolName, func(t *testing.T) {
			props := toolInputProperties(t, s, toolName)
			taskID, ok := props["task_id"].(map[string]interface{})
			require.True(t, ok, "task_id must be advertised")
			assert.NotContains(t, props, "caller_task_id")
			assert.NotContains(t, toolRequiredNames(t, s, toolName), "task_id",
				"task_id must be optional so it can default to the caller's task")
			assert.Contains(t, taskID["description"], "Defaults to the current task")
		})
	}
}
