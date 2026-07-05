/**
 * createLatestGuard — out-of-order async response guard (WR-04, 04-REVIEW.md).
 *
 * A debounced/rapid-fire async fetch (e.g. TemplateEditor's live-preview
 * fetch) has no guarantee the network delivers responses in the same order
 * requests were issued. Without a guard, a stale (earlier-issued) response
 * that happens to resolve LAST can silently overwrite a newer, still-current
 * result — the user sees an outdated preview with no indication anything is
 * wrong.
 *
 * Usage:
 *
 *   const guard = createLatestGuard();
 *   const id = guard.next();
 *   fetchSomething().then((result) => {
 *     if (!guard.isCurrent(id)) return; // a newer request has since been issued
 *     setState(result);
 *   });
 */
export interface LatestGuard {
  /** Issues a new request id, marking it as the current one. */
  next: () => number;
  /** True iff `id` is still the most recently issued id. */
  isCurrent: (id: number) => boolean;
}

export function createLatestGuard(): LatestGuard {
  let current = 0;

  return {
    next(): number {
      current += 1;
      return current;
    },
    isCurrent(id: number): boolean {
      return id === current;
    },
  };
}
