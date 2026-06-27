---
phase: 02-gap-less-receipt-numbering-core
plan: 02
subsystem: receiptno
tags: [tdd, pure-functions, fiscal-year, receipt-number-format, go]
depends_on:
  requires: []
  provides: [fiscalYear-helper, formatReceiptNo-helper]
  affects: [02-03-allocator]
tech_stack:
  added: []
  patterns: [white-box-unit-test, table-driven-test, panic-on-programming-error, min-width-padding]
key_files:
  created:
    - donnarec-api/internal/receiptno/fiscal_year.go
    - donnarec-api/internal/receiptno/format.go
    - donnarec-api/internal/receiptno/fiscalyear_test.go
    - donnarec-api/internal/receiptno/format_test.go
  modified: []
decisions:
  - "fiscalYear() unexported, white-box test (package receiptno) instead of black-box — allows testing unexported helpers directly without exported wrappers"
  - "formatReceiptNo() accepts 6 primitive args (not sqlc row) for wave-independent testability per interfaces note in PLAN.md"
  - "bangkokLoc cached at package level to avoid repeated LoadLocation syscall on every allocation"
metrics:
  duration: 254s
  completed_date: "2026-06-25T16:12:18Z"
  tasks_completed: 2
  files_created: 4
  files_modified: 0
---

# Phase 02 Plan 02: fiscalYear + formatReceiptNo Pure Helpers Summary

**One-liner:** TDD pure helpers — `fiscalYear()` with Asia/Bangkok normalisation (BE boundary 30 Sep/1 Oct) and `formatReceiptNo()` with `%0*d` min-width padding that expands past 6 digits without truncation.

## Tasks Completed

| # | Task | Commit | Files |
|---|------|--------|-------|
| 1 (RED) | Failing tests for fiscalYear + formatReceiptNo | `0abb731` | fiscalyear_test.go, format_test.go |
| 1-2 (GREEN) | Implement fiscal_year.go + format.go | `da08593` | fiscal_year.go, format.go |

## What Was Built

### `fiscalYear(issueDate time.Time) int` (fiscal_year.go)

Pure unexported function that returns the Thai Buddhist Era fiscal year for any timestamp:

- Calls `time.LoadLocation("Asia/Bangkok")` and normalises `issueDate` via `.In(loc)` before boundary check
- Rule: `month >= time.October` → return `ceYear + 544`; else return `ceYear + 543`
- Panics with `"Asia/Bangkok timezone not available: ..."` if tzdata is missing (programming-error guard, not recoverable)
- `bangkokLoc` is cached at package level to avoid repeated LoadLocation calls
- NEVER calls `time.Now()` — caller passes the approval timestamp (D-40, T-02-05)

### `formatReceiptNo(fiscalYear int, runningNo int, separator string, padding int, yearFormat string, prefix string) string` (format.go)

Pure unexported function that renders the frozen formatted receipt number snapshot (D-42):

- `yearFormat "BE4"` → `fmt.Sprintf("%04d", fiscalYear)` (default)
- `yearFormat "CE4"` → `fmt.Sprintf("%04d", fiscalYear-543)` (Christian Era)
- `fmt.Sprintf("%0*d", padding, runningNo)` — `*` uses `padding` as minimum width; output expands naturally past padding without truncation or error (D-29)
- Returns `prefix + yearStr + separator + runningStr`
- Uses primitive args (not sqlc row type) for wave-independent testability per Plan interfaces note

### Test Coverage

| Test | Cases | Assertions |
|------|-------|-----------|
| TestFiscalYear | 6 boundary cases | Sep 30 23:59 BKK → 2568; Oct 1 00:00 BKK → 2569; Sep 30 17:00 UTC (= Oct 1 00:00 BKK) → 2569 timezone normalisation; Jan 1 2026 BKK → 2569; Sep 30 2026 BKK → 2569; Oct 1 2026 BKK → 2570 |
| TestFormatReceiptNo | 4 format cases | Default BE4/6-digit "2569/000123"; D-29 expansion 1000000 → "2569/1000000"; HOSP prefix + dash + 4-digit "HOSP2569-0005"; CE4 branch "2026/000007" |

All 10 test cases pass. No DB or testcontainers required — pure function tests.

## TDD Gate Compliance

| Gate | Commit | Status |
|------|--------|--------|
| RED — test(`02-02`) | `0abb731` | PASS — tests failed with `undefined: fiscalYear` and `undefined: formatReceiptNo` |
| GREEN — feat(`02-02`) | `da08593` | PASS — all 10 tests pass after implementation |
| REFACTOR | N/A | Not needed — code is minimal and clean |

Gate sequence verified: `test(02-02)` commit precedes `feat(02-02)` commit in git log.

## Deviations from Plan

### Auto-fixed Issues

None.

### Design Adjustments (Rule 2 compliance)

**1. White-box test package instead of black-box**

- **Found during:** Task 1 RED
- **Issue:** Plan specifies `fiscalYear` as unexported + test in `package receiptno_test` (black-box). These are mutually exclusive — black-box test cannot call unexported functions.
- **Fix:** Used `package receiptno` (white-box) instead of `package receiptno_test`. This matches the RESEARCH.md example code which also uses `fiscalYear(tc.input)` directly. Pattern decision documented in frontmatter.
- **Files modified:** fiscalyear_test.go, format_test.go

**2. `formatReceiptNo` uses primitive args (not sqlc row type)**

- Exactly as specified in the plan's `<interfaces>` note — no deviation, just confirming implementation followed the spec.

## Known Stubs

None — both helper functions are fully implemented and tested. No placeholder values or TODOs.

## Threat Surface Scan

No new network endpoints, auth paths, file access patterns, or schema changes introduced. These are pure in-memory functions with no side effects or trust-boundary crossings.

| Threat | Status |
|--------|--------|
| T-02-05: wrong fiscal year from clock skew | Mitigated — `fiscalYear` normalises to Asia/Bangkok, never calls `time.Now()` |
| T-02-06: number wider than padding errored | Mitigated — `%0*d` min-width expands naturally, never errors |

## Notes for Plan 03 (Allocator)

- `fiscalYear()` and `formatReceiptNo()` are unexported within `package receiptno` — the allocator in `allocator.go` (same package) calls them directly without any wiring needed
- Allocator must adapt `db.GetReceiptNumberConfigRow` to the 6 primitive args of `formatReceiptNo`
- `import _ "time/tzdata"` should be added to `cmd/api/main.go` in Plan 03 or the service main to embed tzdata in the binary (Pitfall 5) — avoid relying on OS tzdata package alone

## Self-Check: PASSED

- [x] `donnarec-api/internal/receiptno/fiscal_year.go` — FOUND
- [x] `donnarec-api/internal/receiptno/format.go` — FOUND
- [x] `donnarec-api/internal/receiptno/fiscalyear_test.go` — FOUND
- [x] `donnarec-api/internal/receiptno/format_test.go` — FOUND
- [x] RED commit `0abb731` — FOUND
- [x] GREEN commit `da08593` — FOUND
- [x] `go test ./internal/receiptno/... -count=1` exits 0 — VERIFIED (all 10 cases PASS)
- [x] `func fiscalYear(` in fiscal_year.go — VERIFIED
- [x] `time.Now()` NOT in production code (only in comment) — VERIFIED
- [x] `Asia/Bangkok` reference in fiscal_year.go — VERIFIED
- [x] `func formatReceiptNo(` in format.go — VERIFIED
- [x] `%0*d` pattern in format.go — VERIFIED
