---
phase: 05-e-donation-export-reports-admin-settings
plan: 07
subsystem: ui
tags: [nextjs, react, tanstack-query, next-intl, bff]

requires:
  - phase: 05-e-donation-export-reports-admin-settings
    provides: "05-05 GET /api/reports/summary + GET /api/reports/export (no RBAC gate, D-71) — PII-free month/day donation aggregate"
  - phase: 05-e-donation-export-reports-admin-settings
    provides: "05-02 GET/PUT /api/admin/edonation-config (adminGroup RequireRoles(Admin)) — field-mapping/cash-type-label/near-due-days config route"
  - phase: 05-e-donation-export-reports-admin-settings
    provides: "05-06 BFF proxy conventions (binary passthrough, bffForward, isCheckerOrAdminViewer), AppShell 'รายงานสรุป' nav link pre-wired, lib/edonation.ts client-fetcher module"
provides:
  - "/reports route (Screen 8) — all-staff, no RBAC gate: explicit-apply date-range filter (default current fiscal year), 3 summary cards (total/count/average), month/day breakdown table with segmented toggle, PII-free Excel/CSV export with NO confirmation dialog"
  - "BFF routes: GET /api/bff/reports (JSON passthrough), GET /api/bff/reports/export (binary xlsx/csv passthrough), GET/PUT /api/bff/edonation-config (JSON passthrough)"
  - "5th SettingsTabs tab (EdonationConfigTab) — Admin-only editor for e-Donation field mapping (column order + Thai/English headers), cash_type_label, and near_due_days aging threshold, self-contained save (independent of the top-level 'save all tabs' button)"
affects: []

tech-stack:
  added: []
  patterns:
    - "app/reports/page.tsx is a client-component page (no Server Component + client-child split like /admin/settings or /e-donation) — legitimate because Screen 8 has no server-side RBAC redirect to perform (D-71: zero PII, no gate), so it goes straight into TanStack-Query-backed interactive state without an extra wrapper component."
    - "EdonationConfigTab is deliberately NOT wired into SettingsTabs' top-level 'save all tabs' button — it persists a different config store (edonation_config) than the receipt template/images/tax-text/number-format tabs (settings), so it owns its own query/mutation/save button, mirroring TemplateEditor's dirty-state/save/toast shape as an independent action."

key-files:
  created:
    - donnarec-web/app/api/bff/reports/route.ts
    - donnarec-web/app/api/bff/reports/export/route.ts
    - donnarec-web/app/api/bff/edonation-config/route.ts
    - donnarec-web/app/reports/page.tsx
    - donnarec-web/components/ReportSummaryCards.tsx
    - donnarec-web/components/ReportBreakdownTable.tsx
    - donnarec-web/components/EdonationConfigTab.tsx
    - donnarec-web/lib/reports.ts
  modified:
    - donnarec-web/components/SettingsTabs.tsx
    - donnarec-web/lib/edonation.ts
    - donnarec-web/messages/th.json
    - donnarec-web/messages/en.json

key-decisions:
  - "Added donnarec-web/lib/reports.ts (not in the plan's files_modified list) as the typed client-fetcher module for the reports BFF routes, mirroring 05-06's lib/edonation.ts precedent — fetchReportSummary/downloadReportExport/buildReportQuery plus a currentFiscalYearDateRange() helper (mirrors lib/receipt-number-format.ts's currentFiscalYearBE() Oct-rollover rule) used as the Screen 8 filter bar's default range."
  - "One shared useQuery (queryKey [\"reportSummary\", fromStr, toStr, groupBy]) drives both ReportSummaryCards and ReportBreakdownTable from app/reports/page.tsx — avoids a second network round trip when the month/day toggle changes, since the backend computes top-line totals from the same breakdown rows regardless of granularity."
  - "EdonationConfigTab's field-mapping editor supports header_th/header_en text edits plus up/down move buttons for reordering columns (not drag-and-drop) — sufficient to satisfy D-75's 'column order' requirement without adding a new drag-and-drop dependency for a low-frequency admin action; column_key itself stays read-only since it's the RowValues lookup identifier Go's FieldMapping.RowValues keys off."
  - "Report export (Screen 8) has NO confirmation dialog and NO zero-count disable rule — contrast with Screen 7's audited PII export: D-70 confirms zero PII in this data, and Go's report Export handler always returns 200 with at least a 'รวมทั้งหมด' totals row even for an empty date range (no 404-on-empty-result path to guard against), so there is nothing to warn about or disable for."

requirements-completed: [FR-32]

coverage:
  - id: D1
    description: "Screen 8 (/reports): all-staff route with no RBAC gate, explicit-apply date-range filter (default current fiscal year), summary cards (total/count/average) + month/day breakdown table backed by GET /api/bff/reports, and PII-free Excel/CSV export via GET /api/bff/reports/export with no confirmation dialog"
    requirement: "FR-32"
    verification:
      - kind: other
        ref: "npx tsc --noEmit (clean); npx eslint app/reports components/ReportSummaryCards.tsx components/ReportBreakdownTable.tsx app/api/bff/reports lib/reports.ts (clean, 0 errors); npx next build (route /reports + both BFF routes generated); npx vitest run (44/44 existing tests still pass, no regressions)"
        status: pass
    human_judgment: true
    rationale: "Automated checks prove type-safety/build correctness but not actual runtime data population against a live Go backend, real fiscal-year-default rendering, or the group-by toggle's live re-fetch behavior — the plan's own <verification> section defers the manual UI walkthrough of Screen 8 to /gsd-verify-work."
  - id: D2
    description: "5th SettingsTabs tab (EdonationConfigTab): loads the current edonation_config via GET /api/bff/edonation-config and saves field-mapping/cash_type_label/near_due_days edits via PUT, with dirty-state gating, success/error toast, and no bypass of Go's RequireRoles(Admin) authority"
    requirement: "FR-32"
    verification:
      - kind: other
        ref: "npx tsc --noEmit + npx eslint components/EdonationConfigTab.tsx components/SettingsTabs.tsx app/api/bff/edonation-config lib/edonation.ts (clean, 0 errors); npx next build (route /admin/settings + /api/bff/edonation-config generated)"
        status: pass
    human_judgment: true
    rationale: "No automated test exercises the real load -> edit -> save -> toast -> aging-threshold-takes-effect flow against a live backend — the plan's own <verification> section defers the manual UI walkthrough of the config tab to /gsd-verify-work."

duration: ~3min commit-to-commit (Task 1 22:06:58 -> Task 2 22:10:05, Asia/Bangkok); substantial prior context-loading/design time (reading 05-02/05-05/05-06 SUMMARYs, UI-SPEC, existing component/BFF conventions) not counted, per 05-06's documented precedent for this same phrasing
completed: 2026-07-07
status: complete
---

# Phase 5 Plan 07: Donation Summary Reports (Screen 8) + e-Donation Config Admin Tab Summary

**All-staff `/reports` screen (explicit-apply date-range filter, summary cards, month/day breakdown, PII-free no-confirmation export) plus a 5th Settings tab letting an Admin edit the e-Donation field mapping and aging threshold without a deploy — both BFF-proxied over the live 05-02/05-05 Go endpoints, no stubs.**

## Performance

- **Duration:** ~3 min commit-to-commit (see frontmatter `duration` note)
- **Started:** 2026-07-07T22:06:58+07:00 (Task 1 commit)
- **Completed:** 2026-07-07T22:10:05+07:00 (Task 2 commit)
- **Tasks:** 2
- **Files modified:** 12 (8 created, 4 modified)

## Accomplishments

- Two BFF Route Handlers under `app/api/bff/reports/**`: `route.ts` (GET) is a thin `bffForward` passthrough to Go `GET /api/reports/summary`; `export/route.ts` (GET) reads `goRes.arrayBuffer()` and forwards the xlsx/csv bytes + `Content-Type`/`Content-Disposition` verbatim — never JSON-parsing the binary stream (mirrors `app/api/bff/edonation/export/route.ts`'s established binary-response discipline from 05-06). Both obtain the Keycloak Bearer server-side — the access token never reaches the browser.
- `app/reports/page.tsx` (Client Component, no Server Component RBAC wrapper needed per D-71): page heading "รายงานสรุปการบริจาค", an explicit-apply Calendar-Popover date-range filter defaulting to the current fiscal year (`lib/reports.ts`'s `currentFiscalYearDateRange()`, mirroring `lib/receipt-number-format.ts`'s Oct-rollover rule), a single shared `useQuery` (`["reportSummary", from, to, groupBy]`) driving both `ReportSummaryCards` (3 cards: total amount / receipt count / average per receipt, `Intl.NumberFormat('th-TH')`, full-context `aria-label`s) and `ReportBreakdownTable` (month/day segmented toggle reusing Phase 4's `TemplateEditor` HTML/PDF-toggle pattern, right-aligned comma-formatted columns, 31-row/page `Pagination` for daily grouping, no cap for monthly), and an export row (`ส่งออกรายงาน Excel (.xlsx)` / `ส่งออกรายงาน CSV`, outline variant) that downloads directly from `GET /api/bff/reports/export` with **no confirmation dialog** (D-70: zero PII, contrast with Screen 7's audited export).
- `app/api/bff/edonation-config/route.ts`: GET+PUT `bffForward` passthrough to Go `GET`/`PUT /api/admin/edonation-config`. `components/EdonationConfigTab.tsx`: a self-contained Admin editor (own `useQuery`/`useMutation`/dirty-state/save button, `TemplateEditor`-pattern success/error toast) for the e-Donation export field mapping (per-column Thai/English header text inputs + up/down reorder buttons, `column_key` read-only), `cash_type_label` (D-65), and `near_due_days` (D-68 aging threshold) — added as `SettingsTabs`' 5th tab (`components/SettingsTabs.tsx`), deliberately independent of the existing top-level "Save" button since it PUTs a different config store (`edonation_config`, not receipt `settings`).
- `lib/reports.ts` (new, beyond the plan's file list — see Deviations) and `lib/edonation.ts` (extended) provide the typed client fetchers: `fetchReportSummary`/`downloadReportExport`/`buildReportQuery`/`reportExportFileName`/`currentFiscalYearDateRange` for Screen 8; `fetchEdonationConfig`/`saveEdonationConfig` + `EdonationConfig`/`FieldMappingColumn` types for the config tab.
- `messages/th.json`/`en.json`: new `reports.*` top-level namespace (filter/cards/breakdown/export copy) and `settings.tabs.edonation` + `settings.edonation.*` (heading/save/field-mapping/cash-type/near-due-days copy).

## Task Commits

Each task was committed atomically:

1. **Task 1: Report BFF routes + Reports screen (filter, summary cards, breakdown table)** - `e73e90e` (feat)
2. **Task 2: e-Donation config admin tab (D-75) in Settings** - `93e49d0` (feat)

_Note: both tasks add entries to `messages/th.json`/`en.json`. Because the two tasks' i18n additions live in non-overlapping regions of the same JSON files (a new `reports` top-level key for Task 1; `settings.tabs.edonation` + `settings.edonation.*` for Task 2), each task's commit was built by reverting and re-applying only that task's JSON region — the two commits are cleanly separable (verified via `git show <hash> -- messages/th.json`), not a squashed combination._

## Files Created/Modified

- `donnarec-web/app/api/bff/reports/route.ts` - JSON passthrough BFF route for the summary
- `donnarec-web/app/api/bff/reports/export/route.ts` - binary xlsx/csv passthrough BFF route
- `donnarec-web/app/api/bff/edonation-config/route.ts` - GET/PUT passthrough BFF route
- `donnarec-web/app/reports/page.tsx` - Screen 8 route: filter + cards + breakdown + export
- `donnarec-web/components/ReportSummaryCards.tsx` - 3 stat cards (total/count/average)
- `donnarec-web/components/ReportBreakdownTable.tsx` - month/day breakdown table + toggle
- `donnarec-web/components/EdonationConfigTab.tsx` - 5th Settings tab: field mapping editor
- `donnarec-web/lib/reports.ts` - typed client fetchers + fiscal-year-default helper (new)
- `donnarec-web/components/SettingsTabs.tsx` - adds the 5th "e-Donation" tab
- `donnarec-web/lib/edonation.ts` - extends with config types + fetchEdonationConfig/saveEdonationConfig
- `donnarec-web/messages/th.json` / `en.json` - `reports.*` + `settings.tabs.edonation`/`settings.edonation.*` namespaces

## Decisions Made

See `key-decisions` in the frontmatter above:
- Added `lib/reports.ts` as the shared typed client-fetcher module (not in the plan's file list), mirroring 05-06's `lib/edonation.ts` precedent.
- One shared `useQuery` drives both the summary cards and the breakdown table from `app/reports/page.tsx`.
- Field-mapping reordering uses up/down buttons, not drag-and-drop.
- Report export has no confirmation dialog and no zero-count disable rule, unlike Screen 7's export.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Added `lib/reports.ts` as a shared typed client-fetcher module**
- **Found during:** Task 1 (Reports screen implementation)
- **Issue:** The plan's `files_modified` list does not include a `lib/reports.ts`, but `app/reports/page.tsx` needs typed fetchers (`fetchReportSummary`, `downloadReportExport`) and a fiscal-year-default helper. Inlining this logic directly in the page component would violate the codebase's established `lib/donations.ts`/`lib/edonation.ts` client-fetcher convention.
- **Fix:** Added `donnarec-web/lib/reports.ts` mirroring `lib/edonation.ts`'s conventions (plain `Error` with a Thai message on failure, `{data:...}` envelope unwrapping) — single source of truth for the Reports screen's data access, plus `currentFiscalYearDateRange()` reusing `lib/receipt-number-format.ts`'s canonical fiscal-year rollover rule.
- **Files modified:** `donnarec-web/lib/reports.ts` (new)
- **Verification:** `npx tsc --noEmit` + `npx eslint .` clean; `npx next build` succeeds; `app/reports/page.tsx` imports and uses it without duplication.
- **Committed in:** `e73e90e` (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 missing-critical addition, Rule 2)
**Impact on plan:** Purely additive scaffolding needed to implement the plan's own explicit requirement (Screen 8's filter/cards/breakdown/export data flow) without code duplication. No scope creep — no new BFF routes or backend calls beyond what the plan specifies.

## Issues Encountered

None. `npx tsc --noEmit`, `npx eslint .`, `npx next build`, and `npx vitest run` (44/44 tests, 7 files) were all clean after both tasks, with no regressions from the new routes/components.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- Phase 5 (e-Donation Export, Reports & Admin Settings) now has all 7 plans code-complete: 05-01 through 05-04 (backend substrate/export/backup/aging), 05-05 (reports backend), 05-06 (Screen 7 UI), 05-07 (Screen 8 UI + config tab).
- The `/reports` and `/admin/settings` (5th tab) screens are fully wired end-to-end against the live 05-02/05-05 Go backend endpoints (no stubs, no mock data) — ready for `/gsd-verify-work`'s manual UI walkthrough, which both this plan's `<verification>` section and 05-06's precedent explicitly defer from the executor.
- Saving `near_due_days` via the new config tab is structurally reflected by the Aging view's bucket threshold with no further FE work: Go's `Aging` handler already re-reads `h.cfg.GetConfig(ctx)` (including `NearDueDays`) on every request (05-02/05-04), so the config tab's PUT takes effect on the very next `GET /api/edonation/aging` call.
- No blockers.

---
*Phase: 05-e-donation-export-reports-admin-settings*
*Completed: 2026-07-07*

## Self-Check: PASSED

All 12 code files (8 created + 4 modified: `SettingsTabs.tsx`, `lib/edonation.ts`,
`th.json`, `en.json`) verified present on disk; this SUMMARY.md verified present;
both task commits (`e73e90e`, `93e49d0`) verified present in git history.
