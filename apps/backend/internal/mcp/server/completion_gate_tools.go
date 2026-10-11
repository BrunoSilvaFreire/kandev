package mcp

import (
	"context"
	"encoding/json"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerCompletionGateTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("get_task_completion_gate_kandev",
			mcp.WithDescription("Get the current completion gate snapshot, declared criteria, revision, blockers, and whether completion is blocked for a task. When a move or completion signal was accepted but the transition later failed, transition_failure carries its code, reasons, and remediation until a later transition succeeds."),
			mcp.WithString("task_id", mcp.Description("Optional task ID. Defaults to your current task when omitted.")),
		),
		s.wrapHandler("get_task_completion_gate_kandev", s.getTaskCompletionGateHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema("set_task_completion_criteria_kandev",
			"Register or update task completion criteria for a task under an expected revision (typically called by Architect from an approved plan). Cannot weaken or remove unmet criteria without human confirmation.",
			taskCompletionCriteriaToolSchema(),
		),
		s.wrapHandler("set_task_completion_criteria_kandev", s.setTaskCompletionCriteriaHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema("enroll_task_plan_increments_kandev",
			"Bind declared plan increments from an approved plan revision to task completion criteria (called by Architect upon plan approval).",
			enrollTaskPlanIncrementsToolSchema(),
		),
		s.wrapHandler("enroll_task_plan_increments_kandev", s.enrollTaskPlanIncrementsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema("verify_task_completion_criterion_kandev",
			"Submit verification evidence for one declared completion criterion (typically called by Reviewer on PASS). Automatically records the calling agent's session as the verifier.",
			taskCompletionEvidenceToolSchema(),
		),
		s.wrapHandler("verify_task_completion_criterion_kandev", s.verifyTaskCompletionCriterionHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema("record_plan_minor_approval_kandev",
			"Record an audited Architect minor-revision plan approval atop a human-approved baseline. Accepts only a plan head that appends exactly one trailing '## Architect clarifications (advisory)' section with unchanged criterion IDs, descriptions, and evidence subjects; material changes require human approval. Re-enrolls the unchanged increments against the exact head.",
			recordPlanMinorApprovalToolSchema(),
		),
		s.wrapHandler("record_plan_minor_approval_kandev", s.recordPlanMinorApprovalHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema("manage_task_completion_criteria_kandev",
			"Manage task completion criteria and verification evidence. Supports operations: get, set, enroll, verify, minor_approval.",
			manageTaskCompletionCriteriaToolSchema(),
		),
		s.wrapHandler("manage_task_completion_criteria_kandev", s.manageTaskCompletionCriteriaHandler()),
	)
}

func taskCompletionCriteriaToolSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "task_id": {
      "type": "string",
      "description": "Optional task ID. Defaults to your current task when omitted."
    },
    "expected_revision": {
      "type": "integer",
      "description": "The current revision returned by get_task_completion_gate_kandev (starts at 0 if no criteria exist)."
    },
    "plan_revision_id": {
      "type": "string",
      "description": "Optional ID of the approved plan revision declaring these criteria."
    },
    "criteria": {
      "type": "array",
      "description": "The declared criteria increments to register.",
      "items": {
        "type": "object",
        "properties": {
          "id": {
            "type": "string",
            "description": "Stable criterion or increment ID (e.g. 'I1', 'I2')."
          },
          "description": {
            "type": "string",
            "description": "Human-readable description of what this increment verifies."
          },
          "evidence_subject": {
            "type": "object",
            "description": "Optional subject identity; defaults to {kind: 'plan_increment', id: <criterion_id>}.",
            "properties": {
              "kind": { "type": "string" },
              "id": { "type": "string" }
            }
          }
        },
        "required": ["id", "description"]
      }
    }
  },
  "required": ["expected_revision", "criteria"]
}`)
}

func enrollTaskPlanIncrementsToolSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "task_id": {
      "type": "string",
      "description": "Optional task ID. Defaults to your current task when omitted."
    },
    "expected_revision": {
      "type": "integer",
      "description": "The current revision returned by get_task_completion_gate_kandev (starts at 0 if no criteria exist)."
    },
    "plan_revision_id": {
      "type": "string",
      "description": "The ID of the approved plan revision declaring these increments."
    },
    "increments": {
      "type": "array",
      "description": "The declared plan increments to enroll as completion criteria.",
      "items": {
        "type": "object",
        "properties": {
          "id": {
            "type": "string",
            "description": "Stable increment ID (e.g. 'I1', 'I2')."
          },
          "description": {
            "type": "string",
            "description": "Human-readable description of what this increment verifies."
          }
        },
        "required": ["id", "description"]
      }
    }
  },
  "required": ["expected_revision", "plan_revision_id", "increments"]
}`)
}

func recordPlanMinorApprovalToolSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "task_id": {
      "type": "string",
      "description": "Optional task ID. Defaults to your current task when omitted."
    },
    "expected_revision": {
      "type": "integer",
      "description": "The current completion-gate revision returned by get_task_completion_gate_kandev."
    },
    "plan_revision_id": {
      "type": "string",
      "description": "Optional assertion of the current plan head revision ID; a mismatch is rejected."
    },
    "summary": {
      "type": "string",
      "description": "Concise summary of the advisory clarification appended to the plan."
    },
    "attestation": {
      "type": "string",
      "description": "Architect attestation that the change is a non-normative clarification with unchanged increments."
    },
    "increments": {
      "type": "array",
      "description": "The unchanged plan increments to re-enroll (IDs and descriptions must match the enrolled criteria).",
      "items": {
        "type": "object",
        "properties": {
          "id": { "type": "string" },
          "description": { "type": "string" }
        },
        "required": ["id", "description"]
      }
    }
  },
  "required": ["expected_revision", "summary", "attestation", "increments"]
}`)
}

func taskCompletionEvidenceToolSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "task_id": {
      "type": "string",
      "description": "Optional task ID. Defaults to your current task when omitted."
    },
    "expected_revision": {
      "type": "integer",
      "description": "The current gate revision returned by get_task_completion_gate_kandev."
    },
    "criterion_id": {
      "type": "string",
      "description": "The criterion identifier to verify (e.g. 'I1')."
    },
    "evidence": {
      "type": "object",
      "description": "Verification evidence details.",
      "properties": {
        "summary": {
          "type": "string",
          "description": "Summary of verification results, tests passed, or artifact findings."
        },
        "reference": {
          "type": "string",
          "description": "Optional reference (e.g. test run command, artifact key, or commit hash)."
        },
        "subject": {
          "type": "object",
          "description": "Optional subject identity; defaults to {kind: 'plan_increment', id: <criterion_id>}.",
          "properties": {
            "kind": { "type": "string" },
            "id": { "type": "string" },
            "revision": { "type": "string" }
          }
        }
      },
      "required": ["summary"]
    }
  },
  "required": ["expected_revision", "criterion_id", "evidence"]
}`)
}

func manageTaskCompletionCriteriaToolSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "operation": {
      "type": "string",
      "enum": ["get", "set", "enroll", "verify", "minor_approval"],
      "description": "Operation to perform: get (read snapshot), set (register criteria), enroll (bind plan increments), verify (submit evidence), minor_approval (record an audited minor plan revision approval)."
    },
    "action": {
      "type": "string",
      "enum": ["get", "set", "enroll", "verify", "minor_approval"],
      "description": "Alias for operation."
    },
    "task_id": {
      "type": "string",
      "description": "Optional task ID. Defaults to your current task when omitted."
    },
    "expected_revision": {
      "type": "integer",
      "description": "The current revision returned by get_task_completion_gate_kandev (required for set, enroll, and verify)."
    },
    "plan_revision_id": {
      "type": "string",
      "description": "Optional ID of the approved plan revision declaring these criteria (for set or enroll)."
    },
    "criteria": {
      "type": "array",
      "description": "The declared criteria increments to register (for set).",
      "items": {
        "type": "object",
        "properties": {
          "id": { "type": "string" },
          "description": { "type": "string" },
          "evidence_subject": { "type": "object" }
        },
        "required": ["id", "description"]
      }
    },
    "increments": {
      "type": "array",
      "description": "The declared plan increments to enroll as completion criteria (for enroll).",
      "items": {
        "type": "object",
        "properties": {
          "id": { "type": "string" },
          "description": { "type": "string" }
        },
        "required": ["id", "description"]
      }
    },
    "criterion_id": {
      "type": "string",
      "description": "The criterion identifier to verify (for verify)."
    },
    "summary": {
      "type": "string",
      "description": "Concise summary of the advisory clarification (for minor_approval)."
    },
    "attestation": {
      "type": "string",
      "description": "Architect attestation that the minor revision is non-normative with unchanged increments (for minor_approval)."
    },
    "evidence": {
      "type": "object",
      "description": "Verification evidence details (for verify).",
      "properties": {
        "summary": { "type": "string" },
        "reference": { "type": "string" },
        "subject": { "type": "object" }
      },
      "required": ["summary"]
    }
  }
}`)
}

func (s *Server) getTaskCompletionGateHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := s.resolveTaskID(req)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		payload := map[string]interface{}{
			"task_id": taskID,
		}
		return s.forwardToBackend(ctx, ws.ActionMCPGetTaskCompletionGate, payload)
	}
}

func (s *Server) setTaskCompletionCriteriaHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := s.resolveTaskID(req)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		args := req.GetArguments()
		expectedRevision, ok := args["expected_revision"]
		if !ok || expectedRevision == nil {
			return mcp.NewToolResultError("expected_revision is required"), nil
		}
		criteria, ok := args["criteria"]
		if !ok || criteria == nil {
			return mcp.NewToolResultError("criteria is required"), nil
		}
		payload := map[string]interface{}{
			"task_id":           taskID,
			"expected_revision": expectedRevision,
			"criteria":          criteria,
			"sender_session_id": s.sessionID,
		}
		if planRevID := req.GetString("plan_revision_id", ""); planRevID != "" {
			payload["plan_revision_id"] = planRevID
		}
		return s.forwardToBackend(ctx, ws.ActionMCPSetTaskCompletionCriteria, payload)
	}
}

func (s *Server) enrollTaskPlanIncrementsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := s.resolveTaskID(req)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		args := req.GetArguments()
		expectedRevision, ok := args["expected_revision"]
		if !ok || expectedRevision == nil {
			return mcp.NewToolResultError("expected_revision is required"), nil
		}
		planRevID, err := req.RequireString("plan_revision_id")
		if err != nil || planRevID == "" {
			return mcp.NewToolResultError("plan_revision_id is required"), nil
		}
		increments, ok := args["increments"]
		if !ok || increments == nil {
			return mcp.NewToolResultError("increments is required"), nil
		}
		payload := map[string]interface{}{
			"task_id":           taskID,
			"expected_revision": expectedRevision,
			"plan_revision_id":  planRevID,
			"increments":        increments,
			"sender_session_id": s.sessionID,
		}
		return s.forwardToBackend(ctx, ws.ActionMCPEnrollTaskPlanIncrements, payload)
	}
}

func (s *Server) recordPlanMinorApprovalHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := s.resolveTaskID(req)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		args := req.GetArguments()
		expectedRevision, ok := args["expected_revision"]
		if !ok || expectedRevision == nil {
			return mcp.NewToolResultError("expected_revision is required"), nil
		}
		summary, err := req.RequireString("summary")
		if err != nil || summary == "" {
			return mcp.NewToolResultError("summary is required"), nil
		}
		attestation, err := req.RequireString("attestation")
		if err != nil || attestation == "" {
			return mcp.NewToolResultError("attestation is required"), nil
		}
		increments, ok := args["increments"]
		if !ok || increments == nil {
			return mcp.NewToolResultError("increments is required"), nil
		}
		payload := map[string]interface{}{
			"task_id":           taskID,
			"expected_revision": expectedRevision,
			"summary":           summary,
			"attestation":       attestation,
			"increments":        increments,
			"sender_session_id": s.sessionID,
		}
		if planRevID := req.GetString("plan_revision_id", ""); planRevID != "" {
			payload["plan_revision_id"] = planRevID
		}
		return s.forwardToBackend(ctx, ws.ActionMCPRecordPlanMinorApproval, payload)
	}
}

func (s *Server) verifyTaskCompletionCriterionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := s.resolveTaskID(req)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		criterionID, err := req.RequireString("criterion_id")
		if err != nil {
			return mcp.NewToolResultError("criterion_id is required"), nil
		}
		args := req.GetArguments()
		expectedRevision, ok := args["expected_revision"]
		if !ok || expectedRevision == nil {
			return mcp.NewToolResultError("expected_revision is required"), nil
		}
		evidence, ok := args["evidence"]
		if !ok || evidence == nil {
			return mcp.NewToolResultError("evidence is required"), nil
		}
		payload := map[string]interface{}{
			"task_id":           taskID,
			"expected_revision": expectedRevision,
			"criterion_id":      criterionID,
			"evidence":          evidence,
			"sender_session_id": s.sessionID,
		}
		return s.forwardToBackend(ctx, ws.ActionMCPVerifyTaskCompletionCriterion, payload)
	}
}

func (s *Server) manageTaskCompletionCriteriaHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := s.resolveTaskID(req)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		args := req.GetArguments()
		op := req.GetString("operation", "")
		if op == "" {
			op = req.GetString("action", "")
		}
		payload := map[string]interface{}{
			"operation":         op,
			"task_id":           taskID,
			"sender_session_id": s.sessionID,
		}
		for k, v := range args {
			if k != "task_id" {
				payload[k] = v
			}
		}
		return s.forwardToBackend(ctx, ws.ActionMCPManageTaskCompletionCriteria, payload)
	}
}
