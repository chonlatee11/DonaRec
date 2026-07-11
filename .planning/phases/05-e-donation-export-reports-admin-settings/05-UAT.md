---
status: diagnosed
phase: 05-e-donation-export-reports-admin-settings
source: [05-VERIFICATION.md]
started: 2026-07-07T16:00:00Z
updated: 2026-07-11T00:00:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Screen 7 (/e-donation) — Export tab + Aging tab
expected: Export tab — filter + count preview + amber PII-warning confirm dialog + streamed .xlsx/.csv download; zero-count disables export. Aging tab — bucket stat cards, tri-state select-all, per-row + bulk mark/unmark keyed; marked rows drop out of the unkeyed buckets on refetch.
result: issue
reported: "Console Error MISSING_MESSAGE: Could not resolve `aging.tabAging` in messages for locale `th`. components/AgingTable.tsx (305:45)"
severity: minor

### 2. Screen 8 (/reports) + 5th Settings tab (e-Donation config)
expected: Reports screen renders real totals/breakdown for all three roles (Maker included, no 403); date-range filter defaults to current fiscal year; summary cards (total/count/average) + month/day breakdown toggle; PII-free export downloads immediately with NO confirm dialog. Settings 5th tab (EdonationConfigTab) loads current edonation_config, saves field-mapping + near_due_days edits, and a near_due_days change is reflected in the Aging bucket thresholds on the next request.
result: pass

## Summary

total: 2
passed: 1
issues: 1
pending: 0
skipped: 0
blocked: 0

## Gaps

- truth: "Aging table sr-only caption renders the localized 'Keyed-Status Tracking' label"
  status: failed
  reason: "User reported: Console Error MISSING_MESSAGE: Could not resolve `aging.tabAging` in messages for locale `th` (AgingTable.tsx:305)"
  severity: minor
  test: 1
  root_cause: "AgingTable.tsx:305 calls t('tabAging') with t=useTranslations('aging'), resolving aging.tabAging — but the tabAging key exists only under the eDonationExport namespace (th.json:298 / en.json:298), not under aging{}. Missing in BOTH locales."
  artifacts:
    - path: "donnarec-web/components/AgingTable.tsx"
      issue: "line 305 t('tabAging') resolves to aging.tabAging which is undefined"
    - path: "donnarec-web/messages/th.json"
      issue: "aging{} namespace (line 323+) lacks tabAging key"
    - path: "donnarec-web/messages/en.json"
      issue: "aging{} namespace (line 323+) lacks tabAging key"
  missing:
    - "Add tabAging to the aging{} namespace in both th.json and en.json (or point the caption at eDonationExport.tabAging)"
