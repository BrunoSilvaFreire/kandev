"use client";

import { useTranslation } from "react-i18next";
import type { FilterClause, FilterOp } from "@/lib/state/slices/ui/sidebar-view-types";
import { ViewFilterEditor } from "@/components/view-model/view-filter-editor";
import { VIEW_DIMENSION_METAS } from "@/lib/view-model/dimensions";
import { SUPPORTED_DIMENSIONS } from "@/lib/view-model/views";
import { getDimensionEnumOptions, getDimensionMeta, getOpLabel } from "./filter-dimension-registry";
import { useFilterValueOptions } from "./use-filter-value-options";

type Props = {
  clause: FilterClause;
  onChange: (next: FilterClause) => void;
  onRemove: () => void;
  quickFilter?: { visible: boolean; onToggle: (visible: boolean) => void };
};

export function FilterClauseEditor({ clause, onChange, onRemove, quickFilter }: Props) {
  const { t } = useTranslation();
  const meta = getDimensionMeta(clause.dimension);
  const dynamicOptions = useFilterValueOptions(clause.dimension);
  const options = getDimensionEnumOptions(meta) ?? dynamicOptions;

  return (
    <ViewFilterEditor
      clause={clause}
      dimensions={SUPPORTED_DIMENSIONS.sidebar.map((dimension) => {
        const meta = VIEW_DIMENSION_METAS[dimension];
        return {
          dimension: dimension as FilterClause["dimension"],
          ...meta,
          placeholder: meta.placeholderKey ? t(meta.placeholderKey) : undefined,
        };
      })}
      getMeta={getDimensionMeta}
      getDimensionLabel={(dimension) => t(getDimensionMeta(dimension).labelKey)}
      getOpLabel={(op, valueKind) => getOpLabel(op as FilterOp, valueKind)}
      optionsForDimension={() => options}
      onChange={onChange}
      onRemove={onRemove}
      quickFilter={quickFilter}
    />
  );
}
