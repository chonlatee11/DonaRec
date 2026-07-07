---
phase: 05-e-donation-export-reports-admin-settings
plan: 06
subsystem: ui
tags: [nextjs, react, tanstack-query, tanstack-table, shadcn, radix-checkbox, next-intl, bff]

requires:
  - phase: 05-e-donation-export-reports-admin-settings
    provides: "05-02 GET /api/edonation/export (RequireAnyRole(Checker,Admin), audited xlsx/csv stream) + GET/PUT /api/admin/edonation-config"
  - phase: 05-e-donation-export-reports-admin-settings
    provides: "05-04 POST /api/edonation/keyed (bulk/per-row mark, per-record audit) + GET /api/edonation/aging (not_due/near_due/overdue buckets + counts)"
provides:
  - "/e-donation route (Screen 7) — Checker/Admin-gated, two-tab layout: Export (filter + count preview + PII-warning confirm + streamed xlsx/csv download) and Keyed-Status Tracking (bucket stat cards + selectable table + bulk/per-row mark-keyed)"
  - "BFF routes: GET /api/bff/edonation/export (binary passthrough, preserves Content-Type/Content-Disposition, never JSON-parses the stream), POST /api/bff/edonation/keyed, GET /api/bff/edonation/aging"
  - "isCheckerOrAdminViewer() in lib/session-role.ts — UX-hint nav-gating helper parallel to isAdminViewer()"
  - "lib/edonation.ts — typed client fetchers (fetchAging/setKeyed/downloadExport/buildExportQuery) shared by the Export and Aging tabs"
  - "shadcn checkbox primitive (components/ui/checkbox.tsx) — tri-state select-all for the aging table"
  - "AppShell nav items: 'ส่งออก e-Donation' (checker/admin-gated) and 'รายงานสรุป' (always visible, ready for 05-07)"
affects: [05-07]

tech-stack:
  added:
    - "@radix-ui/react-checkbox (via npx shadcn@latest add checkbox)"
  patterns:
    - "Binary BFF response passthrough (export route): read goRes.arrayBuffer() + forward Content-Type/Content-Disposition verbatim instead of bffForward's JSON-parse path — same discipline as app/api/bff/settings/preview/pdf/route.ts, applied here on a GET+query-string proxy instead of a POST."
    - "Shared TanStack Query cache key across sibling tabs (['edonationAging']): ExportPanel's count preview and AgingTable's own fetch both key off the same query, so only one network request serves both tabs and a keyed mutation's invalidateQueries refresh both views without prop drilling."
    - "AgingTable as a 'smart container': owns the query + mutation + selection/bucket-filter/pagination state and composes the presentational AgingStatCards/BulkActionBar/badges — page.tsx stays a thin Server Component RBAC gate + Tabs shell."

key-files:
  created:
    - donnarec-web/app/e-donation/page.tsx
    - donnarec-web/app/api/bff/edonation/export/route.ts
    - donnarec-web/app/api/bff/edonation/keyed/route.ts
    - donnarec-web/app/api/bff/edonation/aging/route.ts
    - donnarec-web/components/ExportPanel.tsx
    - donnarec-web/components/ExportConfirmDialog.tsx
    - donnarec-web/components/AgingStatCards.tsx
    - donnarec-web/components/AgingTable.tsx
    - donnarec-web/components/BulkActionBar.tsx
    - donnarec-web/components/KeyedStatusBadge.tsx
    - donnarec-web/components/AgingBucketBadge.tsx
    - donnarec-web/components/ui/checkbox.tsx
    - donnarec-web/lib/edonation.ts
  modified:
    - donnarec-web/components/AppShell.tsx
    - donnarec-web/lib/session-role.ts
    - donnarec-web/messages/th.json
    - donnarec-web/messages/en.json
    - donnarec-web/package.json
    - donnarec-web/package-lock.json

key-decisions:
  - "Record-count preview (Export tab) has no backend count/dry-run endpoint to call — the Go API's only 'count' source is the audited Export endpoint itself (which decrypts PII and writes an audit row per call, unacceptable to fire on every filter keystroke) or the Aging endpoint (unkeyed-only rows). Resolved by deriving an EXACT client-side count from the shared GET /api/bff/edonation/aging query's row-level approved_at, filtered by the selected date range — correct for keyed_status='not_keyed' (the UI-SPEC default/primary workflow) because Aging's row set IS that exact candidate set. For 'all'/'keyed' selections (which Aging cannot represent — it never returns keyed rows) the preview is hidden rather than showing a fabricated/undercounted number; export buttons stay enabled and the backend's real empty-result 404 remains the true zero-record safety net."
  - "AgingTable's tri-state select-all header checkbox scopes to the CURRENT PAGE's 20 visible rows, not the entire filtered result set — the conventional data-table bulk-select scope, avoiding a bulk mutation silently touching off-screen rows the user never saw."
  - "AgingTable is the 'smart container' owning the shared aging query, mutation, and all Tab B interaction state (bucket filter/selection/pagination); AgingStatCards and BulkActionBar stay presentational (props + callbacks only) — page.tsx stays a thin Server Component RBAC gate + Tabs shell, matching the plan's file list which lists no separate 'AgingPanel' wrapper."
  - "Added donnarec-web/lib/edonation.ts (not in the plan's files_modified list) as the typed client-fetcher module for the Export/Aging BFF routes, mirroring lib/donations.ts's established organization — necessary so ExportPanel and AgingTable share one fetchAging/setKeyed/downloadExport implementation instead of duplicating fetch logic inline in each component."
  - "AgingStatCards uses native <button> elements (not a div with role='button'/tabIndex) for the clickable bucket cards — a native button is inherently keyboard-operable (Enter/Space) and implicitly role='button', a stronger accessibility guarantee than manually replicating that behavior on a non-interactive element, while still satisfying the UI-SPEC's aria-pressed requirement."

requirements-completed: [FR-30, FR-31]

coverage:
  - id: D1
    description: "Screen 7 BFF proxy substrate: GET /api/bff/edonation/export streams the xlsx/csv binary through unchanged (Content-Type/Content-Disposition preserved, never JSON-parsed), POST /api/bff/edonation/keyed and GET /api/bff/edonation/aging proxy JSON via bffForward; isCheckerOrAdminViewer() gates the new AppShell nav items (checker/admin for e-Donation, always-visible for Reports); checkbox primitive installed"
    requirement: "FR-30"
    verification:
      - kind: other
        ref: "npx tsc --noEmit (clean); grep -c isCheckerOrAdminViewer lib/session-role.ts == 2; test -f components/ui/checkbox.tsx; npx next build (route /e-donation + all 3 BFF routes generated)"
        status: pass
    human_judgment: true
    rationale: "Automated checks prove type-safety/build correctness but not actual runtime binary-passthrough fidelity or real RBAC-gated nav visibility against a live Keycloak session — the plan's own <verification> section defers the manual UI walkthrough of Screen 7 to /gsd-verify-work."
  - id: D2
    description: "Export tab: date-range + keyed-status filter bar, live record-count preview, Excel/CSV export buttons that open the amber PII-warning ExportConfirmDialog before streaming the BFF download; zero-count state disables both buttons with inline no-records copy"
    requirement: "FR-30"
    verification:
      - kind: other
        ref: "npx tsc --noEmit + npx eslint app/e-donation components/ExportPanel.tsx components/ExportConfirmDialog.tsx (clean, 0 errors)"
        status: pass
    human_judgment: true
    rationale: "No automated UI/E2E test exists for the export download flow (confirm dialog -> blob download -> success toast) in this frontend layer — the plan defers manual UI walkthrough to /gsd-verify-work per the UI-SPEC."
  - id: D3
    description: "Aging tab: 3 clickable bucket stat cards (aria-pressed) toggling the table filter, tri-state select-all header checkbox, per-row keyed toggle, BulkActionBar mark/unmark/clear wired to POST /api/edonation/keyed with query invalidation + toast feedback"
    requirement: "FR-31"
    verification:
      - kind: other
        ref: "npx tsc --noEmit + npx eslint components/Aging*.tsx components/BulkActionBar.tsx components/KeyedStatusBadge.tsx (clean, 0 errors); npx vitest run (44/44 existing tests still pass, no regressions)"
        status: pass
    human_judgment: true
    rationale: "No automated test exercises the real bulk-select -> mutate -> refetch -> bucket-drop-out flow against a live backend — the plan defers manual UI walkthrough to /gsd-verify-work per the UI-SPEC (Accessibility Contract items — aria-pressed/aria-checked mixed/aria-live — also need a human/screen-reader spot check)."

duration: ~6min commit-to-commit (Task 1 21:44:07 -> Task 3 21:50:14, Asia/Bangkok); substantial prior context-loading/design time (reading 05-02/05-04/UI-SPEC contracts, existing component conventions) not counted, per 05-04's documented precedent for this same phrasing
completed: 2026-07-07
status: complete
---

# Phase 5 Plan 06: e-Donation Export + Keyed-Status/Aging UI (Screen 7) Summary

**`/e-donation` back-office screen: BFF-proxied Export tab (date-range/keyed-status filter, live count preview, amber PII-warning confirm dialog, streamed xlsx/csv download) and Aging tab (3 bucket stat cards, tri-state selectable table, bulk/per-row mark-keyed via TanStack Query) — Checker/Admin-gated, token never reaching the browser.**

## Performance

- **Duration:** ~6 min commit-to-commit (see frontmatter `duration` note)
- **Started:** 2026-07-07T21:44:07+07:00 (Task 1 commit)
- **Completed:** 2026-07-07T21:50:14+07:00 (Task 3 commit)
- **Tasks:** 3
- **Files modified:** 19 (14 created, 5 modified)

## Accomplishments

- `lib/session-role.ts` gains `isCheckerOrAdminViewer()` (parallel to `isAdminViewer()`), and `AppShell.tsx` gains two nav items: "ส่งออก e-Donation" (gated by that UX-hint helper — Go's `RequireAnyRole(Checker,Admin)` on `edonationGroup` remains the real authority) and "รายงานสรุป" (always visible, pre-wired for 05-07's Screen 8 so it needs no further `AppShell` edit).
- Three BFF Route Handlers under `app/api/bff/edonation/**`: `export` (GET) reads `goRes.arrayBuffer()` and forwards the xlsx/csv bytes + `Content-Type`/`Content-Disposition` verbatim — never JSON-parsing the binary stream (mirrors `app/api/bff/settings/preview/pdf/route.ts`'s established binary-response pattern, T-05-06-BINARY); `keyed` (POST) and `aging` (GET) are thin `bffForward` passthroughs. All three obtain the Keycloak Bearer server-side — the access token never reaches the browser (T-05-06-TOKEN).
- `components/ui/checkbox.tsx` installed via the official shadcn registry (`@radix-ui/react-checkbox` added to `package.json`), providing the tri-state indeterminate support the aging table's select-all header needs.
- `app/e-donation/page.tsx` (Server Component): redirects non-Checker/Admin viewers (UX convenience only — Go's route guard is authoritative), renders the "ส่งออก e-Donation" page heading and a two-tab `Tabs` layout hosting `ExportPanel` (Tab A) and `AgingTable` (Tab B).
- `components/ExportPanel.tsx` + `components/ExportConfirmDialog.tsx`: date-range Calendar-Popover filters + keyed-status `Select` (default "ยังไม่คีย์"), a live record-count preview derived client-side from the shared aging query (exact for the default not-keyed filter; gracefully hidden — not fabricated — for "all"/"keyed" since no backend count endpoint exists for those scopes), accent "ส่งออก Excel (.xlsx)" / outline "ส่งออก CSV" buttons that open an amber PII-warning `AlertDialog` (D-64, parallel structure to `RevealPIIDialog`/`CancelDialog`) before triggering a same-origin blob download via `lib/edonation.ts`'s `downloadExport`; zero-count state disables both buttons with inline no-records copy.
- `components/AgingStatCards.tsx`, `AgingBucketBadge.tsx`, `KeyedStatusBadge.tsx`, `BulkActionBar.tsx`, and `AgingTable.tsx` (the Tab B "smart container"): three native-`<button>` bucket cards (`aria-pressed`) toggling the table's bucket filter, a `TanStack Table` with a tri-state select-all `Checkbox` column (current-page scope), aging-bucket + keyed-status badges, a per-row inline mark/unmark toggle, a `BulkActionBar` (aria-live selection count) wired to `POST /api/bff/edonation/keyed`, and 20-row client-side pagination over the (unpaginated) `GET /api/bff/edonation/aging` response. A successful mark/unmark invalidates the shared `["edonationAging"]` TanStack Query key, refreshing both `ExportPanel`'s count preview and `AgingTable` together.
- `lib/edonation.ts` (new, beyond the plan's file list — see Deviations): typed `AgingRow`/`AgingResult`/`ExportFilters` types plus `fetchAging`/`setKeyed`/`buildExportQuery`/`downloadExport` client fetchers shared by both tabs, mirroring `lib/donations.ts`'s established client-fetcher conventions.

## Task Commits

Each task was committed atomically:

1. **Task 1: BFF proxy routes + session-role helper + nav item + checkbox primitive** - `b78bb8f` (feat)
2. **Task 2: Export tab — filter, count preview, PII-warning confirm dialog, streamed download** - `b1ae4a2` (feat)
3. **Task 3: Aging tab — bucket stat cards, selectable table, per-row toggle, bulk action bar** - `428a3e1` (feat)

## Files Created/Modified

- `donnarec-web/app/e-donation/page.tsx` - Screen 7 route: Checker/Admin RBAC gate + two-tab layout
- `donnarec-web/app/api/bff/edonation/export/route.ts` - binary xlsx/csv passthrough BFF route
- `donnarec-web/app/api/bff/edonation/keyed/route.ts` - `bffForward` passthrough for mark/unmark
- `donnarec-web/app/api/bff/edonation/aging/route.ts` - `bffForward` passthrough for the aging view
- `donnarec-web/components/ExportPanel.tsx` - Tab A: filter bar, count preview, export buttons
- `donnarec-web/components/ExportConfirmDialog.tsx` - amber PII-warning `AlertDialog`
- `donnarec-web/components/AgingStatCards.tsx` - 3 clickable bucket-count cards
- `donnarec-web/components/AgingTable.tsx` - Tab B smart container: query + mutation + table
- `donnarec-web/components/BulkActionBar.tsx` - selection count + mark/unmark/clear
- `donnarec-web/components/KeyedStatusBadge.tsx` - keyed/not-keyed `Badge` wrapper
- `donnarec-web/components/AgingBucketBadge.tsx` - bucket→color `Badge` wrapper
- `donnarec-web/components/ui/checkbox.tsx` - shadcn checkbox primitive (new)
- `donnarec-web/lib/edonation.ts` - typed client fetchers shared by both tabs (new, beyond plan file list)
- `donnarec-web/components/AppShell.tsx` - adds "ส่งออก e-Donation" + "รายงานสรุป" nav links
- `donnarec-web/lib/session-role.ts` - adds `isCheckerOrAdminViewer()`
- `donnarec-web/messages/th.json` / `en.json` - `nav.eDonation`/`nav.reports` + `eDonationExport.*`/`aging.*` namespaces
- `donnarec-web/package.json` / `package-lock.json` - `@radix-ui/react-checkbox` dependency

## Decisions Made

See `key-decisions` in the frontmatter above:
- Record-count preview derives an exact client-side count from the shared aging query for the default "not_keyed" filter; hides (does not fabricate) the preview for "all"/"keyed" since no backend count endpoint covers those scopes.
- Aging table's select-all scope is the current page (20 rows), not the whole filtered set.
- `AgingTable` is the Tab B smart container; `AgingStatCards`/`BulkActionBar` stay presentational.
- Added `lib/edonation.ts` as the shared typed client-fetcher module (not in the plan's file list).
- `AgingStatCards` uses native `<button>` elements for stronger built-in keyboard accessibility than a `div[role=button]`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Added `lib/edonation.ts` as a shared typed client-fetcher module**
- **Found during:** Task 2 (Export tab implementation)
- **Issue:** The plan's `files_modified` list does not include a `lib/edonation.ts`, but `ExportPanel` (Task 2) and `AgingTable` (Task 3) both need identical `fetchAging`/`setKeyed`/`downloadExport` BFF-calling logic. Duplicating this fetch/error-handling logic inline in two separate component files would violate the codebase's established `lib/donations.ts` client-fetcher convention and make the shared `["edonationAging"]` TanStack Query cache key (which both tabs rely on for dedup + invalidation) harder to keep consistent.
- **Fix:** Added `donnarec-web/lib/edonation.ts` mirroring `lib/donations.ts`'s conventions (plain `Error` with a Thai message on failure, `{data:...}` envelope unwrapping) — a single source of truth for both tabs' data access.
- **Files modified:** `donnarec-web/lib/edonation.ts` (new)
- **Verification:** `npx tsc --noEmit` + `npx eslint .` clean; `npx next build` succeeds; both `ExportPanel` and `AgingTable` import and use it without duplication.
- **Committed in:** `b1ae4a2` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 missing-critical addition, Rule 2)
**Impact on plan:** Purely additive scaffolding needed to implement the plan's own explicit requirement ("Export buttons... trigger a download" / "wire all into Tab B... sourced from GET /api/bff/edonation/aging") without code duplication. No scope creep — no new BFF routes, no new backend calls beyond what the plan specifies.

## Issues Encountered

None. `npx tsc --noEmit`, `npx eslint .`, and `npx next build` were all clean after every task; the full `npx vitest run` suite (44/44 tests, 7 files) was re-run after Task 3 and shows no regressions from the new components/routes.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- 05-07 (Admin Settings 5th tab for e-Donation field mapping + Screen 8 Reports) can add the `/reports` page directly — the "รายงานสรุป" nav link is already wired in `AppShell.tsx` (always visible, no RBAC gate per D-71) and needs no further `AppShell` edit.
- The Screen 7 UI is fully wired end-to-end against the live 05-02/05-04 backend endpoints (no stubs, no mock data) — ready for `/gsd-verify-work`'s manual UI walkthrough per the plan's own `<verification>` section, which explicitly defers that step from this executor.
- No blockers.

---
*Phase: 05-e-donation-export-reports-admin-settings*
*Completed: 2026-07-07*

## Self-Check: PASSED

All 17 code files (13 created + 4 modified: AppShell.tsx, session-role.ts, th.json,
en.json — plus package.json/package-lock.json) verified present on disk; this
SUMMARY.md verified present; all 3 task commits (`b78bb8f`, `b1ae4a2`, `428a3e1`)
verified present in git history.
