import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { applyWorkflowUpdateAction, previewWorkflowUpdateAction } from "@/app/actions/workspaces";
import { WorkflowUpdateDialog } from "./workflow-update-dialog";

const toast = vi.fn();
vi.mock("@/app/actions/workspaces", () => ({
  applyWorkflowUpdateAction: vi.fn(),
  previewWorkflowUpdateAction: vi.fn(),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const WORKFLOW_ID = "workflow-1";
const WORKSPACE_ID = "workspace-1";

describe("WorkflowUpdateDialog", () => {
  it("requires choosing a definition before previewing a multi-definition file", async () => {
    render(
      <WorkflowUpdateDialog
        workspace={{ id: WORKSPACE_ID } as never}
        workflow={{ id: WORKFLOW_ID, name: "Roll", source: "manual" } as never}
        open
        onOpenChange={vi.fn()}
        onUpdated={vi.fn()}
      />,
    );
    const file = new File(
      [
        `version: 3\ntype: kandev_workflow\nworkflows:\n  - name: First\n    steps: []\n  - name: Second\n    steps: []`,
      ],
      "workflows.yaml",
      { type: "text/yaml" },
    );
    fireEvent.change(screen.getByLabelText("workflows:updateChooseFile"), {
      target: { files: [file] },
    });
    await screen.findByLabelText("workflows:updateSelectDefinition");
    const preview = screen.getByRole("button", { name: "workflows:updatePreview" });
    expect((preview as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(preview);
    expect(previewWorkflowUpdateAction).not.toHaveBeenCalled();
  });

  it("previews an explicitly selected definition and applies only after confirmation", async () => {
    vi.mocked(previewWorkflowUpdateAction).mockResolvedValue({
      workflow_id: WORKFLOW_ID,
      version: "digest-1",
      can_apply: true,
      errors: [],
      changes: [{ kind: "modified", path: "steps.step-key.prompt", before: "old", after: "new" }],
    });
    vi.mocked(applyWorkflowUpdateAction).mockResolvedValue();
    const onOpenChange = vi.fn();
    const onUpdated = vi.fn();
    render(
      <WorkflowUpdateDialog
        workspace={{ id: WORKSPACE_ID } as never}
        workflow={{ id: WORKFLOW_ID, name: "Roll", source: "manual" } as never}
        open
        onOpenChange={onOpenChange}
        onUpdated={onUpdated}
      />,
    );

    const file = new File(
      [
        `version: 3\ntype: kandev_workflow\nworkflows:\n  - name: First\n    steps: []\n  - name: Second\n    steps: []`,
      ],
      "workflows.yaml",
      { type: "text/yaml" },
    );
    fireEvent.change(screen.getByLabelText("workflows:updateChooseFile"), {
      target: { files: [file] },
    });
    await screen.findByLabelText("workflows:updateSelectDefinition");
    fireEvent.change(screen.getByLabelText("workflows:updateSelectDefinition"), {
      target: { value: "1" },
    });
    fireEvent.click(screen.getByRole("button", { name: "workflows:updatePreview" }));

    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByText(/steps\.step-key\.prompt/);
    expect(previewWorkflowUpdateAction).toHaveBeenCalledWith(
      WORKSPACE_ID,
      WORKFLOW_ID,
      expect.stringContaining("Second"),
      1,
    );
    expect(applyWorkflowUpdateAction).not.toHaveBeenCalled();
    fireEvent.click(within(dialog).getByRole("button", { name: "workflows:updateConfirm" }));
    await waitFor(() =>
      expect(applyWorkflowUpdateAction).toHaveBeenCalledWith(
        WORKSPACE_ID,
        WORKFLOW_ID,
        expect.stringContaining("Second"),
        "digest-1",
        1,
      ),
    );
    expect(onUpdated).toHaveBeenCalledOnce();
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("allows cancellation without applying a preview", async () => {
    const onOpenChange = vi.fn();
    render(
      <WorkflowUpdateDialog
        workspace={{ id: WORKSPACE_ID } as never}
        workflow={{ id: WORKFLOW_ID, name: "Roll" } as never}
        open
        onOpenChange={onOpenChange}
        onUpdated={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "common:cancel" }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(applyWorkflowUpdateAction).not.toHaveBeenCalled();
  });
});
