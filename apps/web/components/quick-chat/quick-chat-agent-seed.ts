import type { AgentProfileOption } from "@/lib/state/slices";
import { isSelectableAgentProfile } from "@/lib/state/slices/settings/types";

/** The candidate id when that profile exists and is selectable, otherwise "". */
function selectableProfileId(
  agentProfiles: AgentProfileOption[],
  candidateId: string | null | undefined,
  dynamicRoutingEnabled: boolean,
): string {
  if (!candidateId) return "";
  const profile = agentProfiles.find((item) => item.id === candidateId);
  if (!profile || !isSelectableAgentProfile(profile, dynamicRoutingEnabled)) return "";
  return candidateId;
}

/**
 * Seed the agent for a new Quick Chat.
 *
 * The utility agent profile wins when it is selectable, because Quick Chats are
 * the fast path and should default to the user's utility agent. The workspace
 * default is the fallback; an explicit user choice overrides both in the setup
 * component, which owns the selected value after seeding.
 */
export function seedQuickChatAgentProfileId(args: {
  agentProfiles: AgentProfileOption[];
  utilityAgentProfileId: string | null | undefined;
  workspaceDefaultAgentProfileId: string | null | undefined;
  dynamicRoutingEnabled: boolean;
}): string {
  const utility = selectableProfileId(
    args.agentProfiles,
    args.utilityAgentProfileId,
    args.dynamicRoutingEnabled,
  );
  if (utility) return utility;
  return selectableProfileId(
    args.agentProfiles,
    args.workspaceDefaultAgentProfileId,
    args.dynamicRoutingEnabled,
  );
}
