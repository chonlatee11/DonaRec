---
phase: 05-e-donation-export-reports-admin-settings
plan: 08
subsystem: ui
tags: [i18n, next-intl, aging-table]

requires:
  - phase: 05-e-donation-export-reports-admin-settings
    provides: AgingTable.tsx and aging/eDonationExport message namespaces (plans 05-06, 05-07)
provides:
  - aging.tabAging i18n key present in both th.json and en.json, closing the 05-UAT.md Test 1 MISSING_MESSAGE gap
affects: [05-e-donation-export-reports-admin-settings]

tech-stack:
  added: []
  patterns: []

key-files:
  created: []
  modified:
    - donnarec-web/messages/th.json
    - donnarec-web/messages/en.json

key-decisions:
  - "Reused identical wording from eDonationExport.tabAging rather than inventing new copy, keeping the aging table's sr-only caption consistent with the existing e-Donation export tab label."
  - "Placed the new key adjacent to keyedStatusNotKeyedBadge inside aging{} to keep keyed-status-related labels grouped, per plan instruction."

patterns-established: []

requirements-completed: [FR-31]

coverage:
  - id: D1
    description: "aging.tabAging key added to both th.json and en.json inside the aging{} namespace, resolving AgingTable.tsx:305's t('tabAging') call with no MISSING_MESSAGE error"
    requirement: "FR-31"
    verification:
      - kind: unit
        ref: "node -e guard: aging.tabAging present as non-empty string in both th.json and en.json"
        status: pass
    human_judgment: true
    rationale: "Automated guard only proves the JSON key exists and is non-empty; confirming the runtime console is free of the MISSING_MESSAGE error and that the sr-only caption renders correctly requires a human UI walkthrough of the Aging tab in both locales."

duration: 6min
completed: 2026-07-11
status: complete
---

# Phase 05 Plan 08: Aging Tab i18n Gap Closure Summary

**Added missing `aging.tabAging` message key to both th.json and en.json, closing the sole UAT gap from 05-UAT.md Test 1 (MISSING_MESSAGE console error on the Aging tab's sr-only caption).**

## Performance

- **Duration:** 6 min
- **Started:** 2026-07-11T08:xx:xxZ
- **Completed:** 2026-07-11T08:xx:xxZ
- **Tasks:** 1
- **Files modified:** 2

## Accomplishments
- Added `"tabAging": "ติดตามสถานะคีย์"` to the `aging{}` namespace in `donnarec-web/messages/th.json`
- Added `"tabAging": "Keyed-Status Tracking"` to the `aging{}` namespace in `donnarec-web/messages/en.json`
- `AgingTable.tsx:305`'s `t("tabAging")` call under `useTranslations("aging")` now resolves cleanly in both locales — no code change needed, no other keys touched

## Task Commits

Each task was committed atomically:

1. **Task 1: Add tabAging key to the aging namespace in both locale catalogs** - `5495f34` (fix)

**Plan metadata:** commit intentionally deferred — orchestrator owns STATE.md/ROADMAP.md updates and the final metadata commit after the wave completes (per this plan's execution instructions).

## Files Created/Modified
- `donnarec-web/messages/th.json` - added `aging.tabAging` = "ติดตามสถานะคีย์" (one line, adjacent to `keyedStatusNotKeyedBadge`)
- `donnarec-web/messages/en.json` - added `aging.tabAging` = "Keyed-Status Tracking" (one line, adjacent to `keyedStatusNotKeyedBadge`)

## Decisions Made
- Reused the exact wording already present at `eDonationExport.tabAging` in both locale files, per plan instruction, rather than writing new copy — keeps the sr-only caption text consistent across the two e-Donation surfaces that both describe "keyed status tracking."
- No change to `AgingTable.tsx` — its `useTranslations("aging")` binding and `t("tabAging")` call at line 305 were already correct; the only defect was the missing catalog key.

## Deviations from Plan

None - plan executed exactly as written. Single-task gap-closure plan, no auto-fixes needed, no blockers encountered.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Self-Check

- `donnarec-web/messages/th.json` - FOUND, contains `aging.tabAging`
- `donnarec-web/messages/en.json` - FOUND, contains `aging.tabAging`
- Commit `5495f34` - FOUND in git log
- Verification guard `node -e "..."` - exits 0, prints "OK: aging.tabAging present in both locales"

## Next Phase Readiness
- 05-UAT.md Test 1's sole outstanding gap is closed. Both locale catalogs are valid JSON with the required key.
- No blockers for phase 05 completion from this plan.
- Note: this worktree's branch (`worktree-agent-a20b935b7e88bdf7d`) was created from a stale base commit (`74baadf`, pre-dating all phase-05 commits) and lacked the phase-05 plan/summary files, including `05-08-PLAN.md` itself. It was fast-forwarded (`git merge --ff-only`) to `adc874f` (tip of `gsd/phase-05-e-donation-export-reports-admin-settings`) before this plan could be read and executed. No conflicts; fast-forward only, no rebase/merge commit created.

---
*Phase: 05-e-donation-export-reports-admin-settings*
*Completed: 2026-07-11*
