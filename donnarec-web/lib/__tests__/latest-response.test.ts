import { describe, it, expect } from "vitest";
import { createLatestGuard } from "@/lib/latest-response";

/**
 * createLatestGuard — hermetic unit tests (WR-04, 04-REVIEW.md).
 *
 * TemplateEditor's debounced live-preview fetch has no request-id/AbortController
 * guard today: if two fetches are in-flight at once (user edits, then edits again
 * before the first response returns) and the network delivers the OLDER request's
 * response LAST, it silently overwrites the newer preview with stale HTML. These
 * tests prove the guard's core invariant — only the most-recently-issued id is ever
 * "current" — independent of any network/timing, so TemplateEditor can wire
 * `if (!guard.isCurrent(id)) return;` around each async response handler.
 */
describe("createLatestGuard", () => {
  it("marks the most recently issued id as current", () => {
    const guard = createLatestGuard();
    const first = guard.next();
    const second = guard.next();

    expect(guard.isCurrent(first)).toBe(false);
    expect(guard.isCurrent(second)).toBe(true);
  });

  it("rejects a stale (earlier-issued) id even if its response resolves LAST", () => {
    const guard = createLatestGuard();
    const staleId = guard.next(); // request A issued first (in-flight)
    const freshId = guard.next(); // request B issued second (user edited again)

    // Simulate B's response arriving FIRST — still current.
    expect(guard.isCurrent(freshId)).toBe(true);
    // Simulate A's (stale) response arriving LATER — network makes no ordering
    // guarantee — it must be rejected even though it resolves after B.
    expect(guard.isCurrent(staleId)).toBe(false);
  });

  it("treats a single in-flight request as current until a newer one is issued", () => {
    const guard = createLatestGuard();
    const onlyId = guard.next();

    expect(guard.isCurrent(onlyId)).toBe(true);
  });
});
