export type HandoffPreset = {
  sourceSessionId: string;
  targetProfileId: string;
  targetStepId?: string | null;
};

export function buildHandoffInitialState(handoff: HandoffPreset): {
  selectedProfileId: string;
  contextValue: string;
  targetStepId: string | null;
} {
  return {
    selectedProfileId: handoff.targetProfileId,
    contextValue: "blank",
    targetStepId: handoff.targetStepId ?? null,
  };
}
