package handlers

import (
	"context"
	"testing"
	"time"

	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// TestHandleWriteTaskDocument_RecordsCallerSessionProvenance pins that an
// in-session agent write records the trusted principal's task, session, and the
// task's workflow step at write time on the appended revision.
func TestHandleWriteTaskDocument_RecordsCallerSessionProvenance(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-doc", Name: "Doc"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-doc", WorkspaceID: "ws-doc", Name: "Doc"}))
	caller := &models.Task{
		ID: "task-doc-writer", WorkspaceID: "ws-doc", WorkflowID: "wf-doc", WorkflowStepID: "step-write",
		Title: "Writer", State: v1.TaskStateInProgress,
	}
	require.NoError(t, repo.CreateTask(ctx, caller))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-writer", TaskID: caller.ID, AgentProfileID: "profile-1",
		State: models.TaskSessionStateRunning, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}))

	docs := service.NewDocumentService(repo, testLogger(t))
	handoff := service.NewHandoffService(repo, repo, docs, nil, nil, testLogger(t))
	h := &Handlers{taskSvc: svc, handoffSvc: handoff, logger: testLogger(t).WithFields()}

	scopedCtx := mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		CallerTaskID:    caller.ID,
		CallerSessionID: "session-writer",
	})
	msg := makeWSMessage(t, ws.ActionMCPWriteTaskDocument, map[string]string{
		"task_id": caller.ID, "caller_task_id": caller.ID, "document_key": "spec",
		"type": "custom", "title": "Spec", "content": "body",
	})
	resp, err := h.handleWriteTaskDocument(scopedCtx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	revisions, err := docs.ListRevisions(ctx, caller.ID, "spec", 1)
	require.NoError(t, err)
	require.Len(t, revisions, 1)
	rev := revisions[0]
	require.NotNil(t, rev.SourceTaskID)
	require.Equal(t, caller.ID, *rev.SourceTaskID)
	require.NotNil(t, rev.SourceSessionID)
	require.Equal(t, "session-writer", *rev.SourceSessionID)
	require.NotNil(t, rev.SourceWorkflowStepID)
	require.Equal(t, "step-write", *rev.SourceWorkflowStepID)
}

// TestHandleWriteTaskDocument_NoPrincipalLeavesLegacyProvenance pins that a
// write without a trusted in-session principal records no source instead of
// inferring one.
func TestHandleWriteTaskDocument_NoPrincipalLeavesLegacyProvenance(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-doc2", Name: "Doc2"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-doc2", WorkspaceID: "ws-doc2", Name: "Doc2"}))
	caller := &models.Task{
		ID: "task-doc-legacy", WorkspaceID: "ws-doc2", WorkflowID: "wf-doc2", WorkflowStepID: "step-write",
		Title: "Writer", State: v1.TaskStateInProgress,
	}
	require.NoError(t, repo.CreateTask(ctx, caller))

	docs := service.NewDocumentService(repo, testLogger(t))
	handoff := service.NewHandoffService(repo, repo, docs, nil, nil, testLogger(t))
	h := &Handlers{taskSvc: svc, handoffSvc: handoff, logger: testLogger(t).WithFields()}

	msg := makeWSMessage(t, ws.ActionMCPWriteTaskDocument, map[string]string{
		"task_id": caller.ID, "caller_task_id": caller.ID, "document_key": "spec",
		"type": "custom", "title": "Spec", "content": "body",
	})
	resp, err := h.handleWriteTaskDocument(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	revisions, err := docs.ListRevisions(ctx, caller.ID, "spec", 1)
	require.NoError(t, err)
	require.Len(t, revisions, 1)
	require.Nil(t, revisions[0].SourceTaskID)
	require.Nil(t, revisions[0].SourceSessionID)
	require.Nil(t, revisions[0].SourceWorkflowStepID)
}
