import type { ViewFilterClause, ViewFilterValue } from "./types";

/** Reads one item's value for a dimension; `undefined` means "has no value". */
export type ViewValueAccessor<Item, D extends string> = (
  item: Item,
  dimension: D,
) => ViewFilterValue | undefined;

function toStringArray(value: ViewFilterValue): string[] {
  if (Array.isArray(value)) return value.map(String);
  return [String(value)];
}

function actualValues(actual: ViewFilterValue | undefined): Array<ViewFilterValue | undefined> {
  return Array.isArray(actual) ? actual : [actual];
}

function containsValue(actual: ViewFilterValue | undefined, expected: string): boolean {
  return actualValues(actual).some((value) => String(value) === expected);
}

function textMatches(actual: ViewFilterValue | undefined, needle: string): boolean {
  if (needle === "") return true;
  return actualValues(actual).some((value) =>
    String(value ?? "")
      .toLowerCase()
      .includes(needle),
  );
}

/**
 * Evaluate one clause against one item using the view's value accessor.
 *
 * A dimension value may be a scalar or a collection. `is`/`in` accept the
 * clause's values (OR within a clause) against either shape, so a multi-valued
 * dimension such as a task's repositories matches when any member is selected.
 */
export function evaluateViewClause<Item, D extends string>(
  item: Item,
  clause: ViewFilterClause<D>,
  getValue: ViewValueAccessor<Item, D>,
): boolean {
  const actual = getValue(item, clause.dimension);
  const values = toStringArray(clause.value);

  switch (clause.op) {
    case "is":
    case "in":
      return values.some((value) => containsValue(actual, value));
    case "is_not":
    case "not_in":
      return values.every((value) => !containsValue(actual, value));
    case "matches":
      return values.some((value) => textMatches(actual, value.toLowerCase()));
    case "not_matches":
      return values.every((value) => !textMatches(actual, value.toLowerCase()));
    default:
      return true;
  }
}

/**
 * Filter items by clauses: AND across clauses, OR within one clause's values.
 * An empty clause list keeps every item.
 */
export function applyViewFilters<Item, D extends string>(
  items: Item[],
  clauses: ViewFilterClause<D>[],
  getValue: ViewValueAccessor<Item, D>,
): Item[] {
  if (clauses.length === 0) return items;
  return items.filter((item) =>
    clauses.every((clause) => evaluateViewClause(item, clause, getValue)),
  );
}
