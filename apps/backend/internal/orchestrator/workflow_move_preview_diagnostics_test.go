package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/entryroute"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestBuildWorkflowMovePreview_PreservesDiagnosticNotices(t *testing.T) {
	current := &models.TaskSession{
		ID:             "session-current",
		TaskID:         "task-1",
		AgentProfileID: "profile-analysis",
		State:          models.TaskSessionStateWaitingForInput,
		IsPrimary:      true,
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "codex",
			"model":      "gpt-5.6-luna",
		},
		Metadata: map[string]interface{}{
			models.SessionMetaKeyRuntimeConfigOverrides: models.SessionRuntimeConfig{Model: "gpt-5.6-astra"},
		},
	}
	retained := buildWorkflowMovePreview(workflowMovePreviewInput{
		TaskID:          "task-1",
		SourceSession:   current,
		Sessions:        []*models.TaskSession{current},
		Destination:     &wfmodels.WorkflowStep{ID: "step-review"},
		TargetProfileID: "profile-analysis",
		StartPolicy:     models.WorkflowProfileSessionStartPolicyReuse,
	})
	if len(retained.Notices) != 1 || retained.Notices[0].Code != "retained_model_override" {
		t.Fatalf("retained notices = %#v, want retained_model_override", retained.Notices)
	}

	startedAt := time.Now().UTC()
	ambiguousSessions := []*models.TaskSession{
		{
			ID:             "session-current",
			TaskID:         "task-1",
			AgentProfileID: "profile-analysis",
			State:          models.TaskSessionStateWaitingForInput,
			IsPrimary:      true,
			StartedAt:      startedAt,
			AgentProfileSnapshot: map[string]interface{}{
				"agent_name": "codex",
			},
		},
		{
			ID:             "session-other",
			TaskID:         "task-1",
			AgentProfileID: "profile-other",
			State:          models.TaskSessionStateIdle,
			StartedAt:      startedAt,
		},
	}
	ambiguous := buildWorkflowMovePreview(workflowMovePreviewInput{
		TaskID:        "task-1",
		SourceSession: ambiguousSessions[0],
		Sessions:      ambiguousSessions,
		Destination: &wfmodels.WorkflowStep{
			ID: "step-review",
			Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
				Type: wfmodels.OnEnterConfigureSession,
				Config: map[string]interface{}{"rules": []interface{}{map[string]interface{}{
					"agent_name": "codex",
					"operation":  "set",
					"model":      "gpt-5.6-astra",
				}}},
			}}},
		},
		TargetProfileID: "profile-analysis",
		StartPolicy:     models.WorkflowProfileSessionStartPolicyReuse,
	})
	if len(ambiguous.Notices) != 1 || ambiguous.Notices[0].Code != "missing_original_snapshot" {
		t.Fatalf("missing-original notices = %#v, want missing_original_snapshot", ambiguous.Notices)
	}
}

func TestPreviewWorkflowMove_DoesNotBackfillLegacyInitialSessionMetadata(t *testing.T) {
	ctx := t.Context()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-preview", "session-preview", "step-current")

	session, err := repo.GetTaskSession(ctx, "session-preview")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileID = "profile-analysis"
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	if err := repo.SetSessionPrimary(ctx, session.ID); err != nil {
		t.Fatalf("set primary session: %v", err)
	}
	if _, err := repo.RemoveTaskMetadataKey(ctx, "task-preview", models.MetaKeyWorkflowInitialSession); err != nil {
		t.Fatalf("remove initial-session marker: %v", err)
	}

	steps := newMockStepGetter()
	steps.steps["step-current"] = &wfmodels.WorkflowStep{ID: "step-current", WorkflowID: "wf1", Position: 0}
	steps.steps["step-target"] = &wfmodels.WorkflowStep{
		ID:                        "step-target",
		WorkflowID:                "wf1",
		Position:                  1,
		SessionTarget:             &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetInitial},
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
	}
	agent := &mockAgentManager{resolveProfileInfo: &executor.AgentProfileInfo{
		ProfileID:   "profile-analysis",
		ProfileName: "Analysis",
		AgentName:   "codex",
		Model:       "gpt-5.6-luna",
	}}
	svc := createTestServiceWithAgent(repo, steps, newMockTaskRepo(), agent)

	if _, err := svc.PreviewWorkflowMove(ctx, WorkflowMovePreviewRequest{
		TaskID:         "task-preview",
		WorkflowID:     "wf1",
		WorkflowStepID: "step-target",
	}); err != nil {
		t.Fatalf("PreviewWorkflowMove: %v", err)
	}

	task, err := repo.GetTask(ctx, "task-preview")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if _, ok := task.Metadata[models.MetaKeyWorkflowInitialSession]; ok {
		t.Fatal("preview backfilled the legacy initial-session marker")
	}
}

func TestPreviewWorkflowMove_NoSessionLaunchGatesMatchActualMove(t *testing.T) {
	t.Run("step without auto-start stays idle", func(t *testing.T) {
		ctx := t.Context()
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateTask(ctx, &models.Task{
			ID: "task-no-auto-start", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step-source",
			Title: "Task", State: v1.TaskStateCreated, CreatedAt: now, UpdatedAt: now,
			Metadata: map[string]interface{}{models.MetaKeyAgentProfileID: "profile-task"},
		}))

		steps := newMockStepGetter()
		steps.steps["step-source"] = &wfmodels.WorkflowStep{ID: "step-source", WorkflowID: "wf1"}
		steps.steps["step-target"] = &wfmodels.WorkflowStep{
			ID: "step-target", WorkflowID: "wf1", AgentProfileID: "profile-task",
		}
		svc := createTestServiceWithAgent(repo, steps, newMockTaskRepo(), &mockAgentManager{
			resolveProfileInfo: &executor.AgentProfileInfo{ProfileID: "profile-task", ProfileName: "Task profile", Model: "gpt-5.6-luna"},
		})

		preview, err := svc.PreviewWorkflowMove(ctx, WorkflowMovePreviewRequest{
			TaskID: "task-no-auto-start", WorkflowID: "wf1", WorkflowStepID: "step-target",
		})
		if err != nil {
			t.Fatalf("PreviewWorkflowMove: %v", err)
		}
		if preview.Outcome != WorkflowMovePreviewOutcomeNoSession || preview.Recipient != nil {
			t.Fatalf("preview = %#v, want no_session without a recipient", preview)
		}

		svc.handleTaskMovedNoSession(ctx, watcher.TaskMovedEventData{TaskID: "task-no-auto-start", ToStepID: "step-target"})
		sessions, err := repo.ListTaskSessions(ctx, "task-no-auto-start")
		if err != nil {
			t.Fatalf("ListTaskSessions: %v", err)
		}
		if len(sessions) != 0 {
			t.Fatalf("actual move created %d sessions, want idle task", len(sessions))
		}
	})

	t.Run("skip prompt without instructions suppresses auto-start", func(t *testing.T) {
		ctx := t.Context()
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
		options := &workflowmove.EntryOptions{SkipStepPrompt: true}
		encoded, err := workflowmove.EncodeEntryOptionsJSON(options)
		if err != nil {
			t.Fatalf("EncodeEntryOptionsJSON: %v", err)
		}
		requireNoError(t, repo.CreateTask(ctx, &models.Task{
			ID: "task-skip-prompt", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step-source",
			Title: "Task", State: v1.TaskStateCreated, CreatedAt: now, UpdatedAt: now,
			Metadata: map[string]interface{}{
				models.MetaKeyAgentProfileID: "profile-task",
				models.MetaKeyWorkflowMovePending: map[string]interface{}{
					"from_step_id": "step-source", "move_id": "move-1", "options": string(encoded),
				},
			},
		}))

		steps := newMockStepGetter()
		steps.steps["step-source"] = &wfmodels.WorkflowStep{ID: "step-source", WorkflowID: "wf1"}
		steps.steps["step-target"] = &wfmodels.WorkflowStep{
			ID: "step-target", WorkflowID: "wf1",
			Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}},
		}
		svc := createTestServiceWithAgent(repo, steps, newMockTaskRepo(), &mockAgentManager{
			resolveProfileInfo: &executor.AgentProfileInfo{ProfileID: "profile-task", ProfileName: "Task profile", Model: "gpt-5.6-luna"},
		})

		preview, err := svc.PreviewWorkflowMove(ctx, WorkflowMovePreviewRequest{
			TaskID: "task-skip-prompt", WorkflowID: "wf1", WorkflowStepID: "step-target", EntryOptions: options,
		})
		if err != nil {
			t.Fatalf("PreviewWorkflowMove: %v", err)
		}
		if preview.Outcome != WorkflowMovePreviewOutcomeNoSession || preview.Recipient != nil {
			t.Fatalf("preview = %#v, want no_session without a recipient", preview)
		}

		svc.handleTaskMovedNoSession(ctx, watcher.TaskMovedEventData{TaskID: "task-skip-prompt", ToStepID: "step-target"})
		sessions, err := repo.ListTaskSessions(ctx, "task-skip-prompt")
		if err != nil {
			t.Fatalf("ListTaskSessions: %v", err)
		}
		if len(sessions) != 0 {
			t.Fatalf("actual move created %d sessions, want idle task", len(sessions))
		}
		task, err := repo.GetTask(ctx, "task-skip-prompt")
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if _, pending := task.Metadata[models.MetaKeyWorkflowMovePending]; pending {
			t.Fatal("actual move left the one-shot marker after suppressing auto-start")
		}
	})

	t.Run("allowed auto-start retains the task profile fallback", func(t *testing.T) {
		ctx := t.Context()
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
		requireNoError(t, repo.CreateTask(ctx, &models.Task{
			ID: "task-profile-fallback", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step-source",
			Title: "Task", State: v1.TaskStateCreated, CreatedAt: now, UpdatedAt: now,
			Metadata: map[string]interface{}{models.MetaKeyAgentProfileID: "profile-task"},
		}))

		steps := newMockStepGetter()
		steps.steps["step-source"] = &wfmodels.WorkflowStep{ID: "step-source", WorkflowID: "wf1"}
		steps.steps["step-target"] = &wfmodels.WorkflowStep{
			ID: "step-target", WorkflowID: "wf1",
			Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}},
		}
		svc := createTestServiceWithAgent(repo, steps, newMockTaskRepo(), &mockAgentManager{
			resolveProfileInfo: &executor.AgentProfileInfo{
				ProfileID: "profile-task", ProfileName: "Task profile", Model: "gpt-5.6-luna",
			},
		})

		preview, err := svc.PreviewWorkflowMove(ctx, WorkflowMovePreviewRequest{
			TaskID: "task-profile-fallback", WorkflowID: "wf1", WorkflowStepID: "step-target",
		})
		if err != nil {
			t.Fatalf("PreviewWorkflowMove: %v", err)
		}
		if preview.Outcome != WorkflowMovePreviewOutcomeCreateNew {
			t.Fatalf("preview outcome = %q, want create_new", preview.Outcome)
		}
		if preview.Recipient == nil || preview.Recipient.ProfileID != "profile-task" {
			t.Fatalf("preview recipient = %#v, want task profile fallback", preview.Recipient)
		}
		if preview.Model.After.ID != "gpt-5.6-luna" {
			t.Fatalf("preview model = %#v, want task profile model", preview.Model.After)
		}
	})
}

func TestPreviewStepAgentProfileUsesTagSelectionReadOnly(t *testing.T) {
	task := &models.Task{ID: "t1", WorkflowID: "workflow-1"}
	step := &wfmodels.WorkflowStep{
		ID: "review", WorkflowID: "workflow-1", AgentProfileID: "profile-fallback",
		AllowedTags: []string{"review"},
	}
	selector := &recordingOrchestratorSelector{profile: "profile-quota"}
	svc := &Service{workflowEntryProfileSelector: selector}

	got, err := svc.previewStepAgentProfile(context.Background(), step, task, false)
	if err != nil {
		t.Fatalf("previewStepAgentProfile: %v", err)
	}
	if got != "profile-quota" {
		t.Fatalf("preview profile = %q, want the advisory tag candidate profile-quota", got)
	}
	if _, ok := models.LoadWorkflowSessionRoute(task.Metadata); ok {
		t.Fatal("preview must not write a workflow-session route")
	}
}

func TestPreviewStepAgentProfilePropagatesTagSelectionError(t *testing.T) {
	task := &models.Task{ID: "t1", WorkflowID: "workflow-1"}
	step := &wfmodels.WorkflowStep{
		ID: "review", WorkflowID: "workflow-1", AgentProfileID: "profile-fallback",
		AllowedTags: []string{"review"},
	}
	sentinel := errors.New("no eligible tagged profile")
	svc := &Service{workflowEntryProfileSelector: &recordingOrchestratorSelector{err: sentinel}}

	_, err := svc.previewStepAgentProfile(context.Background(), step, task, false)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want the selector error (no fallthrough to the static fallback)", err)
	}
}

func TestPreviewStepAgentProfileTaggedWithoutSelectorFailsClosed(t *testing.T) {
	task := &models.Task{ID: "t1", WorkflowID: "workflow-1"}
	step := &wfmodels.WorkflowStep{
		ID: "review", WorkflowID: "workflow-1", AgentProfileID: "profile-fallback",
		AllowedTags: []string{"review"},
	}
	_, err := (&Service{}).previewStepAgentProfile(context.Background(), step, task, false)
	if !errors.Is(err, entryroute.ErrNoFrozenProfile) {
		t.Fatalf("err = %v, want ErrNoFrozenProfile", err)
	}
}
