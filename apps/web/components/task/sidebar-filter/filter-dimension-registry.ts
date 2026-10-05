import { t } from "@/lib/i18n";
import { getViewDimensionMeta } from "@/lib/view-model/dimensions";
import type { FilterDimension, FilterOp } from "@/lib/state/slices/ui/sidebar-view-types";

export type DimensionValueKind = "boolean" | "enum" | "text";

// `labelKey` / `placeholderKey` hold catalog keys rather than copy: this table
// is module scope, so a resolved `t()` here would freeze at the boot locale.
// The `value` fields are persisted filter values and stay in English. The
// substantive metadata (kind, operators, defaults) comes from the shared
// `lib/view-model/dimensions` registry so every Home view agrees.
export type DimensionMeta = {
  dimension: FilterDimension;
  labelKey: string;
  valueKind: DimensionValueKind;
  ops: FilterOp[];
  enumOptions?: Array<{ value: string; labelKey: string }>;
  placeholderKey?: string;
  defaultOp: FilterOp;
  defaultValue: string | string[] | boolean;
};

/** The dimensions the sidebar offers, in display order, with their display copy. */
const SIDEBAR_DIMENSIONS: ReadonlyArray<{ dimension: FilterDimension; labelKey: string }> = [
  { dimension: "archived", labelKey: "task:filterDimensionArchived" },
  { dimension: "isPRReview", labelKey: "task:filterDimensionPrReview" },
  { dimension: "isIssueWatch", labelKey: "task:filterDimensionIssueWatch" },
  { dimension: "hasDiff", labelKey: "task:filterDimensionHasDiff" },
  { dimension: "hasPR", labelKey: "task:filterDimensionHasPr" },
  { dimension: "state", labelKey: "task:filterDimensionState" },
  { dimension: "workflow", labelKey: "task:filterDimensionWorkflow" },
  { dimension: "workflowStep", labelKey: "task:filterDimensionWorkflowStep" },
  { dimension: "executorType", labelKey: "task:filterDimensionExecutorType" },
  { dimension: "repository", labelKey: "task:filterDimensionRepository" },
  { dimension: "repositoryGroup", labelKey: "task:filterDimensionRepositoryGroup" },
  { dimension: "titleMatch", labelKey: "task:filterDimensionTitle" },
];

export const DIMENSION_METAS: DimensionMeta[] = SIDEBAR_DIMENSIONS.map(
  ({ dimension, labelKey }) => {
    const shared = getViewDimensionMeta(dimension);
    return {
      dimension,
      labelKey,
      valueKind: shared.valueKind,
      ops: [...shared.ops],
      enumOptions: shared.fixedOptions ? [...shared.fixedOptions] : undefined,
      placeholderKey: shared.placeholderKey,
      defaultOp: shared.defaultOp,
      defaultValue: shared.defaultValue,
    };
  },
);

export function getDimensionMeta(dim: FilterDimension): DimensionMeta {
  const meta = DIMENSION_METAS.find((m) => m.dimension === dim);
  if (!meta) throw new Error(`Unknown filter dimension: ${dim}`);
  return meta;
}

const OP_LABEL_KEYS: Record<FilterOp, string> = {
  is: "task:filterOpIs",
  is_not: "task:filterOpIsNot",
  in: "task:filterOpIn",
  not_in: "task:filterOpNotIn",
  matches: "task:filterOpContains",
  not_matches: "task:filterOpDoesNotContain",
};

// Module-level `t` is fine here: these run from render, not at import.
export function getOpLabel(op: FilterOp, valueKind: DimensionValueKind): string {
  if (valueKind === "boolean") {
    if (op === "is") return t("task:filterOpShow");
    if (op === "is_not") return t("task:filterOpHide");
  }
  return t(OP_LABEL_KEYS[op]);
}

/** Resolves a dimension's fixed enum options to displayable labels. */
export function getDimensionEnumOptions(
  meta: DimensionMeta,
): Array<{ value: string; label: string }> | undefined {
  return meta.enumOptions?.map((o) => ({ value: o.value, label: t(o.labelKey) }));
}
