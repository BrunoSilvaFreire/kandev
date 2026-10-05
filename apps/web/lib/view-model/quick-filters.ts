import type { HomeViewId } from "./types";
import { viewSupportsDimension } from "./views";

/**
 * Default dimensions a Home view pins as Quick Filters when the user has not
 * configured any. Order is display order. Each view's supported set is applied
 * by `resolveQuickFilterDimensions`, so a default that a view does not support
 * is dropped rather than rendered.
 */
export const DEFAULT_QUICK_FILTER_DIMENSIONS: readonly string[] = [
  "repositoryGroup",
  "repository",
  "workflow",
  "state",
];

/**
 * The dimensions to render as Quick Filters for a view: the user's configured
 * list when it has entries, otherwise the defaults. Unknown dimensions and
 * dimensions the view does not support are ignored; valid entries are kept.
 */
export function resolveQuickFilterDimensions(
  view: HomeViewId,
  configured: readonly string[] | undefined,
): string[] {
  const source = configured && configured.length > 0 ? configured : DEFAULT_QUICK_FILTER_DIMENSIONS;
  return source.filter((dimension) => viewSupportsDimension(view, dimension));
}
