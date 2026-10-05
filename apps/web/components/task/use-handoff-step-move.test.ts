import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
  moveTask: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mocks.request }),
}));
vi.mock("@/lib/api", () => ({
  moveTask: (...args: unknown[]) => mocks.moveTask(...args),
}));

import { useHandoffStepMove } from "./use-handoff-step-move";

const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const WORKFLOW_ID = "workflow-1";
const STEP_ID = "step-b";

beforeEach(() => {
  vi.clearAllMocks();
  mocks.request.mockResolvedValue(undefined);
  mocks.moveTask.mockResolvedValue(undefined);
});

describe("useHandoffStepMove", () => {
  it("promotes the session to primary", async () => {
    const { result } = renderHook(() => useHandoffStepMove());

    let promoted = false;
    await act(async () => {
      promoted = await result.current.promotePrimary(SESSION_ID);
    });

    expect(promoted).toBe(true);
    expect(mocks.request).toHaveBeenCalledWith(
      "session.set_primary",
      { session_id: SESSION_ID },
      10_000,
    );
    expect(mocks.toast).not.toHaveBeenCalled();
  });

  it("returns false and toasts when primary promotion fails", async () => {
    mocks.request.mockRejectedValueOnce(new Error("nope"));
    const { result } = renderHook(() => useHandoffStepMove());

    let promoted = true;
    await act(async () => {
      promoted = await result.current.promotePrimary(SESSION_ID);
    });

    expect(promoted).toBe(false);
    expect(mocks.toast).toHaveBeenCalledWith({
      title: "task:handoffPrimaryFailed",
      variant: "error",
    });
  });

  it("moves the task through the move API", async () => {
    const { result } = renderHook(() => useHandoffStepMove());

    await act(async () => {
      await result.current.moveToStep(TASK_ID, SESSION_ID, WORKFLOW_ID, STEP_ID);
    });

    expect(mocks.moveTask).toHaveBeenCalledWith(TASK_ID, {
      workflow_id: WORKFLOW_ID,
      workflow_step_id: STEP_ID,
    });
    expect(mocks.toast).not.toHaveBeenCalled();
  });

  it("keeps the session and toasts when the move fails", async () => {
    mocks.moveTask.mockRejectedValueOnce(new Error("move failed"));
    const { result } = renderHook(() => useHandoffStepMove());

    await act(async () => {
      await result.current.moveToStep(TASK_ID, SESSION_ID, WORKFLOW_ID, STEP_ID);
    });

    expect(mocks.toast).toHaveBeenCalledWith({
      title: "task:handoffMoveFailed",
      variant: "error",
    });
  });
});
