import { useCallback, type RefObject } from "react";
import { launchSession } from "@/lib/services/session-launch-service";
import { useAppStore } from "@/components/state-provider";
import { buildStartRequest } from "@/lib/services/session-launch-helpers";
import {
  hasPendingAttachmentUploads,
  toMessageAttachments,
} from "@/components/task-create-dialog-helpers";
import type { TaskFormInputsHandle } from "@/components/task-create-dialog-types";
import type { FileAttachment } from "@/components/task/chat/file-attachment";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { SummarizeSessionResult } from "@/hooks/use-summarize-session";
import { applySummarizeSessionResult, type SummaryToastFn } from "./session-context-summary";
import { t } from "@/lib/i18n";
import { recordAgentProfileRecentUseBestEffort } from "@/lib/agent-profile-recent-use";
import type { AgentProfileRecentUseRecord } from "@/lib/agent-profile-recent-use";

type SessionContextChangeOpts = {
  promptRef: RefObject<TaskFormInputsHandle | null>;
  initialPrompt: string | null;
  summarize: (sessionId: string) => Promise<SummarizeSessionResult>;
  toast: SummaryToastFn;
  setContextValue: (v: string) => void;
  setHasPrompt: (v: boolean) => void;
};

function launchErrorDescription(error: unknown): string {
  if (error instanceof Error) return error.message;
  return t("common:unknownError");
}

function recordTaskSessionProfileUse(
  profileId: string,
  applyAgentProfileRecentUse: (
    context: "task_session",
    record: AgentProfileRecentUseRecord,
  ) => void,
) {
  recordAgentProfileRecentUseBestEffort("task_session", profileId, (record) =>
    applyAgentProfileRecentUse("task_session", record),
  );
}

type ActivateSession = (
  sessionId: string,
  taskId: string,
  tabLabel: string,
  groupId: string | undefined,
  setActiveSession: (taskId: string, sessionId: string) => void,
) => void;

function activateLaunchedSession(params: {
  sessionId: string;
  taskId: string;
  effectiveProfileId: string;
  agentProfiles: AgentProfileOption[];
  groupId?: string;
  applyAgentProfileRecentUse: (
    context: "task_session",
    record: AgentProfileRecentUseRecord,
  ) => void;
  activateSession: ActivateSession;
  setActiveSession: (taskId: string, sessionId: string) => void;
}) {
  recordTaskSessionProfileUse(params.effectiveProfileId, params.applyAgentProfileRecentUse);
  const profile = params.agentProfiles.find(
    (candidate) => candidate.id === params.effectiveProfileId,
  );
  params.activateSession(
    params.sessionId,
    params.taskId,
    profile?.label ?? t("common:agent"),
    params.groupId,
    params.setActiveSession,
  );
}

async function launchNewSession(params: {
  taskId: string;
  profileId: string;
  executorId: string;
  profileExplicit: boolean;
  prompt: string;
  attachments: FileAttachment[];
}) {
  const { request } = buildStartRequest(params.taskId, params.profileId, {
    executorId: params.executorId,
    prompt: params.prompt,
    profileExplicit: params.profileExplicit,
    attachments: toMessageAttachments(params.attachments),
  });
  const response = await launchSession(request);
  if (!response.session_id) throw new Error("Session created but no session ID returned");
  return { sessionId: response.session_id, agentProfileId: response.agent_profile_id };
}

async function promoteActivateAndMove(params: {
  sessionId: string;
  taskId: string;
  effectiveProfileId: string;
  agentProfiles: AgentProfileOption[];
  groupId?: string;
  applyAgentProfileRecentUse: (
    context: "task_session",
    record: AgentProfileRecentUseRecord,
  ) => void;
  activateSession: ActivateSession;
  setActiveSession: (taskId: string, sessionId: string) => void;
  promotePrimary?: (sessionId: string) => Promise<boolean> | boolean;
  onLaunched?: (sessionId: string) => Promise<void> | void;
  onClose: () => void;
}) {
  // Primary promotion precedes local activation so the session the move routes
  // to is the one the user sees active.
  const promoted = params.promotePrimary ? await params.promotePrimary(params.sessionId) : true;
  activateLaunchedSession(params);
  if (promoted) await params.onLaunched?.(params.sessionId);
  params.onClose();
}

export function useSessionContextChange(opts: SessionContextChangeOpts) {
  const { promptRef, initialPrompt, summarize, toast, setContextValue, setHasPrompt } = opts;
  return useCallback(
    async (value: string) => {
      if (!value) return;
      setContextValue(value);
      if (value === "copy_prompt" && initialPrompt && promptRef.current) {
        promptRef.current.setValue(initialPrompt);
        setHasPrompt(true);
      } else if (value === "blank" && promptRef.current) {
        promptRef.current.setValue("");
        setHasPrompt(false);
      } else if (value.startsWith("summarize:")) {
        const sessionId = value.slice("summarize:".length);
        const result = await summarize(sessionId);
        applySummarizeSessionResult({ result, promptRef, setContextValue, setHasPrompt, toast });
      }
    },
    [initialPrompt, promptRef, summarize, setContextValue, setHasPrompt, toast],
  );
}

function prepareSubmitInput(
  promptRef: RefObject<TaskFormInputsHandle | null>,
  contextValue: string,
  initialPrompt: string | null,
): { prompt: string; attachments: FileAttachment[] } | null {
  const typed = promptRef.current?.getValue().trim() ?? "";
  const prompt = contextValue === "copy_prompt" && !typed && initialPrompt ? initialPrompt : typed;
  if (!prompt) return null;
  const attachments = promptRef.current?.getAttachments() ?? [];
  if (hasPendingAttachmentUploads(attachments)) return null;
  return { prompt, attachments };
}

export type SessionLaunchParams = {
  promptRef: RefObject<TaskFormInputsHandle | null>;
  taskId: string;
  selectedProfileId: string;
  profileExplicit: boolean;
  executorId: string;
  contextValue: string;
  initialPrompt: string | null;
  agentProfiles: AgentProfileOption[];
  groupId?: string;
  onClose: () => void;
  toast: SummaryToastFn;
  setActiveSession: (taskId: string, sessionId: string) => void;
  activateSession: ActivateSession;
  setIsCreating: (creating: boolean) => void;
  /**
   * Promotes the new session to primary before it is activated locally. A
   * handoff uses it so the subsequent move routes to this session; returning
   * false keeps the session but skips the move. Must not throw.
   */
  promotePrimary?: (sessionId: string) => Promise<boolean> | boolean;
  /**
   * Runs after the new session is promoted and activated, before the dialog
   * closes. A handoff uses it to move the task; it must not throw.
   */
  onLaunched?: (sessionId: string) => Promise<void> | void;
};

async function executeSessionLaunch(
  params: SessionLaunchParams & {
    applyAgentProfileRecentUse: (
      context: "task_session",
      record: AgentProfileRecentUseRecord,
    ) => void;
  },
): Promise<void> {
  const input = prepareSubmitInput(params.promptRef, params.contextValue, params.initialPrompt);
  if (!input) return;
  params.setIsCreating(true);
  try {
    const { sessionId, agentProfileId } = await launchNewSession({
      taskId: params.taskId,
      profileId: params.selectedProfileId,
      executorId: params.executorId,
      profileExplicit: params.profileExplicit,
      prompt: input.prompt,
      attachments: input.attachments,
    });
    await promoteActivateAndMove({
      sessionId,
      taskId: params.taskId,
      effectiveProfileId: agentProfileId ?? params.selectedProfileId,
      agentProfiles: params.agentProfiles,
      groupId: params.groupId,
      applyAgentProfileRecentUse: params.applyAgentProfileRecentUse,
      activateSession: params.activateSession,
      setActiveSession: params.setActiveSession,
      promotePrimary: params.promotePrimary,
      onLaunched: params.onLaunched,
      onClose: params.onClose,
    });
  } catch (error) {
    params.toast({
      title: t("task:failedToCreateSession"),
      description: launchErrorDescription(error),
      variant: "error",
    });
  } finally {
    params.setIsCreating(false);
  }
}

export function useSessionLaunchSubmit(params: SessionLaunchParams) {
  const applyAgentProfileRecentUse = useAppStore((state) => state.applyAgentProfileRecentUse);
  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      return executeSessionLaunch({ ...params, applyAgentProfileRecentUse });
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps -- the caller's launch contract is the dependency
    [params, applyAgentProfileRecentUse],
  );
  return handleSubmit;
}
