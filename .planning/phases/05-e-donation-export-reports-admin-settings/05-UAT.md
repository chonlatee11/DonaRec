---
status: testing
phase: 05-e-donation-export-reports-admin-settings
source: [05-VERIFICATION.md]
started: 2026-07-07T16:00:00Z
updated: 2026-07-07T16:00:00Z
---

## Current Test

number: 1
name: Screen 7 (/e-donation) manual UI walkthrough — Export tab + Aging tab
expected: |
  Export downloads a real file after confirming the amber PII warning dialog; zero-count
  state disables export; the Aging table's 3 bucket stat cards filter correctly; tri-state
  select-all works; per-row + bulk mark/unmark updates rows and they drop out of the unkeyed
  buckets on refetch.
awaiting: user response

## Tests

### 1. Screen 7 (/e-donation) — Export tab + Aging tab
expected: Export tab — filter + count preview + amber PII-warning confirm dialog + streamed .xlsx/.csv download; zero-count disables export. Aging tab — bucket stat cards, tri-state select-all, per-row + bulk mark/unmark keyed; marked rows drop out of the unkeyed buckets on refetch.
result: [pending]

### 2. Screen 8 (/reports) + 5th Settings tab (e-Donation config)
expected: Reports screen renders real totals/breakdown for all three roles (Maker included, no 403); date-range filter defaults to current fiscal year; summary cards (total/count/average) + month/day breakdown toggle; PII-free export downloads immediately with NO confirm dialog. Settings 5th tab (EdonationConfigTab) loads current edonation_config, saves field-mapping + near_due_days edits, and a near_due_days change is reflected in the Aging bucket thresholds on the next request.
result: [pending]

## Summary

total: 2
passed: 0
issues: 0
pending: 2
skipped: 0
blocked: 0

## Gaps
