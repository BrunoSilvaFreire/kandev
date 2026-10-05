package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
)

func TestStepExit_LatestStepExitSessionID(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		setup      func(t *testing.T, repo *Repository) (taskID string, fromStepID string)
		wantResult string
	}{
		{
			name: "no rows returns empty string",
			setup: func(t *testing.T, repo *Repository) (string, string) {
				taskID := "task-no-exits"
				createStepTransitionsTestTask(t, repo, taskID, "wf-1", "step-init")
				return taskID, "step-never-exited"
			},
			wantResult: "",
		},
		{
			name: "two exits from the same step newest wins",
			setup: func(t *testing.T, repo *Repository) (string, string) {
				taskID := "task-two-exits"
				task := createStepTransitionsTestTask(t, repo, taskID, "wf-1", "step-plan")

				for _, sid := range []string{"session-older", "session-mid", "session-newer"} {
					if err := repo.CreateTaskSession(ctx, &models.TaskSession{
						ID: sid, TaskID: taskID, State: models.TaskSessionStateRunning,
					}); err != nil {
						t.Fatalf("CreateTaskSession(%s): %v", err, sid)
					}
				}

				// Exit 1: step-plan -> step-impl with session-older
				ctxOlder := steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
					Trigger:   steptelemetry.TriggerMCPMove,
					ActorKind: steptelemetry.ActorAgent,
					SessionID: "session-older",
				})
				task.WorkflowStepID = "step-impl"
				if err := repo.UpdateTask(ctxOlder, task); err != nil {
					t.Fatalf("first exit failed: %v", err)
				}

				time.Sleep(5 * time.Millisecond)

				// Move back: step-impl -> step-plan with session-intermediate
				ctxMid := steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
					Trigger:   steptelemetry.TriggerMCPMove,
					ActorKind: steptelemetry.ActorAgent,
					SessionID: "session-mid",
				})
				task.WorkflowStepID = "step-plan"
				if err := repo.UpdateTask(ctxMid, task); err != nil {
					t.Fatalf("move back failed: %v", err)
				}

				time.Sleep(5 * time.Millisecond)

				// Exit 2: step-plan -> step-impl with session-newer
				ctxNewer := steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
					Trigger:   steptelemetry.TriggerMCPMove,
					ActorKind: steptelemetry.ActorAgent,
					SessionID: "session-newer",
				})
				task.WorkflowStepID = "step-impl"
				if err := repo.UpdateTask(ctxNewer, task); err != nil {
					t.Fatalf("second exit failed: %v", err)
				}

				return taskID, "step-plan"
			},
			wantResult: "session-newer",
		},
		{
			name: "null session skipped and older non-null exit returned",
			setup: func(t *testing.T, repo *Repository) (string, string) {
				taskID := "task-null-skipped"
				task := createStepTransitionsTestTask(t, repo, taskID, "wf-1", "step-review")

				if err := repo.CreateTaskSession(ctx, &models.TaskSession{
					ID: "session-valid", TaskID: taskID, State: models.TaskSessionStateRunning,
				}); err != nil {
					t.Fatalf("CreateTaskSession(session-valid): %v", err)
				}

				// Exit 1: step-review -> step-fix with non-null session
				ctxValid := steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
					Trigger:   steptelemetry.TriggerMCPMove,
					ActorKind: steptelemetry.ActorAgent,
					SessionID: "session-valid",
				})
				task.WorkflowStepID = "step-fix"
				if err := repo.UpdateTask(ctxValid, task); err != nil {
					t.Fatalf("first exit failed: %v", err)
				}

				time.Sleep(5 * time.Millisecond)

				// Move back: step-fix -> step-review
				ctxBack := steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
					Trigger:   steptelemetry.TriggerManualMove,
					ActorKind: steptelemetry.ActorHuman,
				})
				task.WorkflowStepID = "step-review"
				if err := repo.UpdateTask(ctxBack, task); err != nil {
					t.Fatalf("move back failed: %v", err)
				}

				time.Sleep(5 * time.Millisecond)

				// Exit 2: step-review -> step-done with NULL session (no SessionID in attribution)
				ctxNull := steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
					Trigger:   steptelemetry.TriggerManualMove,
					ActorKind: steptelemetry.ActorHuman,
				})
				task.WorkflowStepID = "step-done"
				if err := repo.UpdateTask(ctxNull, task); err != nil {
					t.Fatalf("null exit failed: %v", err)
				}

				return taskID, "step-review"
			},
			wantResult: "session-valid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newStepTransitionsTestRepo(t)
			taskID, fromStepID := tc.setup(t, repo)

			got, err := repo.LatestStepExitSessionID(ctx, taskID, fromStepID)
			if err != nil {
				t.Fatalf("LatestStepExitSessionID(%q, %q) error: %v", taskID, fromStepID, err)
			}
			if got != tc.wantResult {
				t.Fatalf("LatestStepExitSessionID(%q, %q) = %q, want %q", taskID, fromStepID, got, tc.wantResult)
			}
		})
	}
}
