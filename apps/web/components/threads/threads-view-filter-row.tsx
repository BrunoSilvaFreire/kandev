"use client";

import { useTranslation } from "react-i18next";
import type { ThreadCandidate } from "@/lib/threads/thread-view-query";
import type { ThreadFilterClause } from "@/lib/state/slices/ui/thread-view-types";
import type { RepositoryGroup } from "@/lib/view-model/repository-group";
import { useRepositoryGroups } from "@/hooks/use-repository-groups";
import { ViewFilterEditor } from "@/components/view-model/view-filter-editor";
import {
  getThreadDimensionLabel,
  getThreadDimensionMeta,
  getThreadFilterOpLabel,
  getThreadFilterOptions,
  THREAD_DIMENSION_METAS,
} from "./threads-view-filter-registry";

export function ThreadsViewFilterRow({
  clause,
  candidates,
  repositoryNames,
  repositoryGroups,
  mobile,
  quickFilter,
  onChange,
  onRemove,
}: {
  clause: ThreadFilterClause;
  candidates: ThreadCandidate[];
  repositoryNames: ReadonlyMap<string, string>;
  repositoryGroups?: readonly RepositoryGroup[];
  mobile?: boolean;
  quickFilter?: { visible: boolean; onToggle: (visible: boolean) => void };
  onChange: (next: ThreadFilterClause) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const storeGroups = useRepositoryGroups();
  const groups = repositoryGroups ?? storeGroups;
  return (
    <ViewFilterEditor
      clause={clause}
      dimensions={THREAD_DIMENSION_METAS}
      getMeta={getThreadDimensionMeta}
      getDimensionLabel={(dimension) => getThreadDimensionLabel(dimension, t)}
      getOpLabel={(op) => getThreadFilterOpLabel(op, t)}
      optionsForDimension={(dimension) =>
        getThreadFilterOptions(dimension, candidates, t, repositoryNames, groups)
      }
      onChange={onChange}
      onRemove={onRemove}
      mobile={mobile}
      quickFilter={quickFilter}
      testIds={{
        row: "threads-filter-row",
        dimension: "threads-filter-dimension",
        op: "threads-filter-op",
        value: "threads-filter-value",
        textValue: "threads-filter-value",
        remove: "threads-filter-remove",
      }}
    />
  );
}
