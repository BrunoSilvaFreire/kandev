"use client";

import { Fragment } from "react";
import { IconStar, IconStarFilled, IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@kandev/ui/select";
import { cn } from "@/lib/utils";
import type { ViewFilterOp, ViewFilterValue } from "@/lib/view-model/types";
import {
  FilterMultiSelect,
  type MultiSelectOption,
} from "@/components/task/sidebar-filter/filter-multi-select";
import {
  buildOptionGroups,
  hasGroupedOptions,
} from "@/components/task/sidebar-filter/filter-option-groups";

export type ViewFilterEditorValueKind = "boolean" | "enum" | "text";

/** The metadata the shared editor needs; both view registries project into it. */
export type ViewFilterEditorMeta<D extends string = string> = {
  dimension: D;
  valueKind: ViewFilterEditorValueKind;
  ops: readonly ViewFilterOp[];
  defaultOp: ViewFilterOp;
  defaultValue: ViewFilterValue;
  placeholder?: string;
};

export type ViewFilterEditorOption = MultiSelectOption;

export type ViewFilterEditorTestIds = {
  row: string;
  dimension: string;
  op: string;
  value: string;
  textValue?: string;
  remove: string;
};

export type ViewFilterEditorProps<D extends string, Op extends ViewFilterOp> = {
  clause: { id: string; dimension: D; op: Op; value: ViewFilterValue };
  dimensions: readonly ViewFilterEditorMeta<D>[];
  getMeta: (dimension: D) => ViewFilterEditorMeta<D>;
  getDimensionLabel: (dimension: D) => string;
  getOpLabel: (op: Op, valueKind: ViewFilterEditorValueKind) => string;
  optionsForDimension: (dimension: D) => ViewFilterEditorOption[];
  onChange: (clause: { id: string; dimension: D; op: Op; value: ViewFilterValue }) => void;
  onRemove: () => void;
  /** When provided, renders the per-dimension "Show as quick filter" toggle. */
  quickFilter?: {
    visible: boolean;
    onToggle: (visible: boolean) => void;
  };
  mobile?: boolean;
  testIds?: Partial<ViewFilterEditorTestIds>;
};

const DEFAULT_TEST_IDS: ViewFilterEditorTestIds = {
  row: "filter-clause-row",
  dimension: "filter-dimension-select",
  op: "filter-op-select",
  value: "filter-value-select",
  textValue: "filter-value-input",
  remove: "filter-clause-remove",
};

/**
 * The one clause editor every Home view uses. It owns dimension / operator /
 * value rendering plus the optional Quick Filter pin, and reads all metadata
 * from whichever view registry the caller projects in.
 */
// eslint-disable-next-line max-lines-per-function -- One cohesive clause row keeps dimension, operator, value and pin together.
export function ViewFilterEditor<D extends string, Op extends ViewFilterOp>({
  clause,
  dimensions,
  getMeta,
  getDimensionLabel,
  getOpLabel,
  optionsForDimension,
  onChange,
  onRemove,
  quickFilter,
  mobile = false,
  testIds: customTestIds,
}: ViewFilterEditorProps<D, Op>) {
  const { t } = useTranslation();
  const testIds = { ...DEFAULT_TEST_IDS, ...customTestIds };
  const meta = getMeta(clause.dimension);
  const options = optionsForDimension(clause.dimension);

  function changeDimension(dimension: D) {
    const nextMeta = getMeta(dimension);
    onChange({ ...clause, dimension, op: nextMeta.defaultOp as Op, value: nextMeta.defaultValue });
  }

  function changeOp(op: Op) {
    onChange({ ...clause, op, value: normalizeFilterValue(clause.value, meta, op) });
  }

  return (
    <div
      className={cn("py-1", mobile && "[&_button]:min-h-11 [&_input]:min-h-11")}
      data-testid={testIds.row}
      data-clause-id={clause.id}
    >
      <div className="flex items-center gap-1.5">
        <Select value={clause.dimension} onValueChange={(value) => changeDimension(value as D)}>
          <SelectTrigger className="w-32 shrink-0 text-xs" data-testid={testIds.dimension}>
            <SelectValue>{getDimensionLabel(clause.dimension)}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            {dimensions.map((dimension) => (
              <SelectItem key={dimension.dimension} value={dimension.dimension} className="text-xs">
                {getDimensionLabel(dimension.dimension)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={clause.op} onValueChange={(value) => changeOp(value as Op)}>
          <SelectTrigger className="w-24 shrink-0 text-xs" data-testid={testIds.op}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {meta.ops.map((op) => (
              <SelectItem key={op} value={op} className="text-xs">
                {getOpLabel(op as Op, meta.valueKind)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <FilterValueInput
          clause={clause}
          meta={meta}
          options={options}
          valueTestId={
            meta.valueKind === "text" ? (testIds.textValue ?? testIds.value) : testIds.value
          }
          onChange={(value) => onChange({ ...clause, value })}
          valuePlaceholder={meta.placeholder ?? t("task:value")}
          selectValuePlaceholder={t("task:selectValue")}
          noOptionsLabel={t("task:noOptions")}
        />

        {quickFilter ? (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="shrink-0 cursor-pointer text-muted-foreground hover:text-foreground"
            onClick={() => quickFilter.onToggle(!quickFilter.visible)}
            aria-pressed={quickFilter.visible}
            aria-label={t(quickFilter.visible ? "task:quickFilterHide" : "task:quickFilterShow")}
            data-testid="filter-quick-toggle"
          >
            {quickFilter.visible ? (
              <IconStarFilled className="h-3.5 w-3.5" />
            ) : (
              <IconStar className="h-3.5 w-3.5" />
            )}
          </Button>
        ) : null}

        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="shrink-0 cursor-pointer text-muted-foreground hover:text-foreground"
          onClick={onRemove}
          data-testid={testIds.remove}
          aria-label={t("task:removeFilter")}
        >
          <IconX className="h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  );
}

function normalizeFilterValue<D extends string, Op extends ViewFilterOp>(
  value: ViewFilterValue,
  meta: ViewFilterEditorMeta<D>,
  op: Op,
): ViewFilterValue {
  if (meta.valueKind === "boolean") return true;
  if (meta.valueKind === "enum" && (op === "in" || op === "not_in")) {
    if (Array.isArray(value)) return value;
    if (value) return [String(value)];
    return [];
  }
  return Array.isArray(value) ? (value[0] ?? "") : String(value);
}

function FilterValueInput<D extends string, Op extends ViewFilterOp>({
  clause,
  meta,
  options,
  valueTestId,
  onChange,
  valuePlaceholder,
  selectValuePlaceholder,
  noOptionsLabel,
}: {
  clause: { id: string; dimension: D; op: Op; value: ViewFilterValue };
  meta: ViewFilterEditorMeta<D>;
  options: ViewFilterEditorOption[];
  valueTestId: string;
  onChange: (value: ViewFilterValue) => void;
  valuePlaceholder: string;
  selectValuePlaceholder: string;
  noOptionsLabel: string;
}) {
  if (meta.valueKind === "boolean") return null;

  if (meta.valueKind === "text") {
    return (
      <Input
        value={String(clause.value ?? "")}
        onChange={(event) => onChange(event.target.value)}
        placeholder={valuePlaceholder}
        className="min-w-0 flex-1 text-xs"
        data-testid={valueTestId}
      />
    );
  }

  const multi = clause.op === "in" || clause.op === "not_in";
  if (multi) {
    const selected = Array.isArray(clause.value) ? clause.value.map(String) : [];
    return <FilterMultiSelect options={options} selected={selected} onChange={onChange} />;
  }

  const current = String(clause.value ?? "");
  return (
    <Select value={current} onValueChange={onChange}>
      <SelectTrigger className="min-w-0 flex-1 text-xs" data-testid={valueTestId}>
        <SelectValue placeholder={selectValuePlaceholder} />
      </SelectTrigger>
      <SelectContent>
        {options.length === 0 ? (
          <SelectItem value="__empty__" disabled className="text-xs">
            {noOptionsLabel}
          </SelectItem>
        ) : (
          <GroupedSelectItems options={options} />
        )}
      </SelectContent>
    </Select>
  );
}

function GroupedSelectItems({ options }: { options: ViewFilterEditorOption[] }) {
  if (!hasGroupedOptions(options)) {
    return options.map((option) => <SelectOption key={option.value} option={option} />);
  }

  return buildOptionGroups(options).map((group, index) => (
    <Fragment key={group.heading || `__ungrouped__${index}`}>
      {index > 0 && <SelectSeparator />}
      <SelectGroup>
        {group.heading && <SelectLabel>{group.heading}</SelectLabel>}
        {group.items.map((option) => (
          <SelectOption key={option.value} option={option} />
        ))}
      </SelectGroup>
    </Fragment>
  ));
}

function SelectOption({ option }: { option: ViewFilterEditorOption }) {
  return (
    <SelectItem value={option.value} className="text-xs">
      <span className="flex items-center gap-1.5">
        {option.color && (
          <span className={cn("block h-2 w-2 shrink-0 rounded-full", option.color)} />
        )}
        <span className="truncate">{option.label}</span>
      </span>
    </SelectItem>
  );
}
