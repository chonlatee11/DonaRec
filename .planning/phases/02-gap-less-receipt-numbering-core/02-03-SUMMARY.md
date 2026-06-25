---
phase: 02-gap-less-receipt-numbering-core
plan: "03"
subsystem: receiptno
tags: [tdd, allocator, gap-less, receipt-number, caller-managed-tx, integration-test]
dependency_graph:
  requires:
    - 02-01 (migration 000004 + sqlc: LockCounterForUpdate, InitCounterRow, IncrementCounter, GetReceiptNumberConfig, InsertReceiptNumberLedger)
    - 02-02 (helpers: fiscalYear(), formatReceiptNo())
  provides:
    - Allocator struct + NewAllocator(*db.Queries) constructor
    - AllocatedReceipt{FiscalYear, RunningNo, Formatted, AllocatedAt}
    - Allocate(ctx, tx, issueDate) — single code path for receipt number issuance
  affects:
    - Phase 3 (issuance tx will call alloc.Allocate inside db.WithTx)
    - 02-04 (concurrency tests will exercise this same Allocate implementation)
tech_stack:
  added: []
  patterns:
    - caller-managed-tx (D-33): Allocate accepts pgx.Tx, never commits/rolls-back
    - FOR-UPDATE + ErrNoRows init path (Pitfall 1 mitigation): InitCounterRow then re-lock
    - queries.WithTx(tx) bind pattern: all DB calls go through qtx inside caller's tx
    - frozen-snapshot (D-42): Formatted stored in ledger = value returned to caller
    - nil-guard panic in constructor (programming-error guard)
key_files:
  created:
    - donnarec-api/internal/receiptno/allocator.go
    - donnarec-api/internal/receiptno/allocator_test.go
  modified: []
decisions:
  - "Allocator.queries field is *db.Queries (concrete) not db.Querier (interface) — because (*db.Queries).WithTx returns *db.Queries, not Querier; binding qtx := a.queries.WithTx(tx) requires the concrete type (Key Observation #1, 02-PATTERNS.md)"
  - "Test package is receiptno_test (black-box) — Allocator, AllocatedReceipt, NewAllocator are exported; fiscalYear/formatReceiptNo remain unexported and tested separately in white-box tests from Plan 02"
metrics:
  duration: "256s"
  completed_date: "2026-06-25T16:21:21Z"
  tasks_completed: 1
  tasks_total: 1
  files_created: 2
  files_modified: 0
---

# Phase 02 Plan 03: Gap-less Allocator Service (caller-managed tx) Summary

**One-liner:** `Allocator.Allocate(ctx, pgx.Tx, issueDate)` — caller-managed, single receipt-number code path using FOR-UPDATE counter lock → init-on-first-year → increment → config read → format → ledger insert, all within caller's tx, proven by 5 integration tests against a real PostgreSQL container.

## What Was Built

### `Allocator` struct + `NewAllocator` + `AllocatedReceipt` (allocator.go)

The sole code path (D-35) that assigns a gap-less receipt number within a caller's transaction:

**`AllocatedReceipt`** — frozen snapshot returned to caller:
- `FiscalYear int` — Thai BE fiscal year (e.g. 2569)
- `RunningNo int` — sequential number within the year (e.g. 1)
- `Formatted string` — frozen formatted string from the ledger row (e.g. "2569/000001")
- `AllocatedAt time.Time` — DB-side `now()` from the INSERT, not `time.Now()`

**`NewAllocator(queries *db.Queries)`** — constructor with nil-guard panic (programming-error guard)

**`func (a *Allocator) Allocate(ctx context.Context, tx pgx.Tx, issueDate time.Time) (AllocatedReceipt, error)`**

Allocation steps inside the caller's tx (caller never sees commit/rollback):

| Step | Operation | Decision |
|------|-----------|----------|
| 1 | `fiscalYear(issueDate)` — pure fn, no DB, no clock | D-40 |
| 2 | `qtx := a.queries.WithTx(tx)` — bind to caller's tx | D-33 |
| 3 | `LockCounterForUpdate` — FOR UPDATE serializes concurrent allocators | NFR-04 |
| 3a | ErrNoRows → `InitCounterRow` (ON CONFLICT DO NOTHING) then re-lock | D-41, Pitfall 1 |
| 4 | `IncrementCounter` — UPDATE RETURNING while lock is held | |
| 5 | `GetReceiptNumberConfig` — read format config inside same tx | D-32 |
| 6 | `formatReceiptNo(fy, next, sep, pad, yearFmt, prefix)` | D-42 |
| 7 | `InsertReceiptNumberLedger` — UNIQUE backstop fires here | D-37 |
| 8 | Return `AllocatedReceipt` from ledger row (frozen snapshot) | D-42 |

**Anti-patterns verified absent (acceptance grep + code review):**
- `pgxpool.Pool` NOT in function signature (Pitfall 2 / T-02-07)
- `MAX()` NOT in any query call (Pitfall 3 / T-02-08)
- `tx.Commit` / `tx.Rollback` NOT called (D-33 caller-managed)
- `time.Now()` NOT called (D-40 issueDate from caller)

### Integration Tests (allocator_test.go)

Package `receiptno_test` (black-box), all tests require testcontainers PostgreSQL 17:

| Test | Scenario | Assertions |
|------|----------|-----------|
| `TestAllocator_SingleAllocate` | First allocation FY 2569 | RunningNo=1, Formatted="2569/000001", AllocatedAt set, ledger row matches |
| `TestAllocator_SequentialGapless` | Three sequential allocations FY 2569 | RunningNo 1,2,3 with no gaps; ledger has exactly 3 rows in order |
| `TestAllocator_NewFiscalYearStartsAtOne` | First allocation FY 2571 (new year) | RunningNo=1, counter table has row with last_running_no=1 |
| `TestAllocator_MultiYearIsolation` | 2 allocs in FY 2569, 1 in FY 2570 | FY 2570 starts at 1 independently; FY 2569 ledger has 1,2 only |
| `TestAllocator_DefaultConfigFormat` | Default config seed | "2569/000001" = BE4 + "/" + 6-pad; Formatted matches ledger (D-42) |

All 5 tests green in `go test ./internal/receiptno/... -count=1` (duration ~13s with testcontainers).

## Commits

| Phase | Task | Commit | Description |
|-------|------|--------|-------------|
| RED | Test (allocator_test.go) | `38c1044` | test(02-03): add failing integration tests for Allocator.Allocate |
| GREEN | Implementation (allocator.go) | `7d9b400` | feat(02-03): implement Allocator.Allocate caller-managed allocator |

TDD gate sequence: `test(02-03)` commit `38c1044` precedes `feat(02-03)` commit `7d9b400`.

## TDD Gate Compliance

| Gate | Commit | Status |
|------|--------|--------|
| RED — test(`02-03`) | `38c1044` | PASS — failed to compile: `undefined: receiptno.NewAllocator` |
| GREEN — feat(`02-03`) | `7d9b400` | PASS — all 5 TestAllocator_* pass against live PostgreSQL 17 |
| REFACTOR | N/A | Not needed — implementation is minimal and clean |

## Deviations from Plan

### Design Adjustments

**1. Allocator.queries field is `*db.Queries` (not `db.Querier`)**
- **Found during:** Implementation
- **Issue:** Key Observation #1 in 02-PATTERNS.md noted that `(*Queries).WithTx` returns `*Queries`, not `Querier` — if the field were `db.Querier`, the call `a.queries.WithTx(tx)` would fail to compile because `Querier` does not have a `WithTx` method.
- **Fix:** Used `*db.Queries` as the field type. `NewAllocator` accepts `*db.Queries` directly. This matches how `audit/service.go` works (also uses `*db.Queries`).
- **Impact:** No change to external interface. Callers pass `db.New(pool)` which returns `*db.Queries` — identical to audit service pattern.

None of the above are bugs — these are design clarifications discovered during implementation.

## Known Stubs

None — `allocator.go` is fully implemented and all five integration tests confirm functional correctness against a real PostgreSQL 17 container. No placeholder values, no hardcoded mock data, no TODOs in production code.

## Threat Surface Scan

No new network endpoints, auth paths, file access patterns, or schema changes. This plan adds application logic only (within the existing `receiptno` package).

Threat register coverage (per plan threat_model):

| Threat ID | Status |
|-----------|--------|
| T-02-07: gap from separate tx | Mitigated — Allocate accepts `pgx.Tx`, not `*pgxpool.Pool`; grep confirms |
| T-02-08: duplicate via MAX+1 | Mitigated — counter FOR UPDATE + increment; no MAX query; grep confirms |
| T-02-09: number reserved on draft | Mitigated — number born only at ledger INSERT inside issuance tx (D-35) |
| T-02-SC: package installs | N/A — no new packages; all deps already in go.mod |

## Self-Check: PASSED

Files exist:
- [x] `donnarec-api/internal/receiptno/allocator.go` — FOUND
- [x] `donnarec-api/internal/receiptno/allocator_test.go` — FOUND

Commits exist:
- [x] `38c1044` — test(02-03): add failing integration tests for Allocator.Allocate — FOUND
- [x] `7d9b400` — feat(02-03): implement Allocator.Allocate caller-managed allocator — FOUND

TDD gate:
- [x] RED commit (`38c1044`) precedes GREEN commit (`7d9b400`) in git log
- [x] RED was genuinely failing (`undefined: receiptno.NewAllocator` compile error)
- [x] GREEN: all 10 tests in `./internal/receiptno/...` pass (6 existing from 02-02 + 5 new allocator tests — wait, actually 10 total: 6 helper + 5 allocator = 11? Let me recount: TestFiscalYear (6 subtests) + TestFormatReceiptNo (4 subtests) + 5 TestAllocator_* = 15 test cases, all PASS)

Verification:
- [x] `cd donnarec-api && go test ./internal/receiptno/... -count=1` exits 0
- [x] `cd donnarec-api && go build ./...` exits 0
- [x] `func (a *Allocator) Allocate(ctx context.Context, tx pgx.Tx` — FOUND in allocator.go
- [x] `pgxpool.Pool` only in comments, NOT in production code
- [x] `MAX(` only in comments, NOT in production code
- [x] `tx.Commit`, `tx.Rollback`, `time.Now()` only in comments, NOT in production code
- [x] `fiscalYear(issueDate)` called in Allocate
- [x] `formatReceiptNo(` called in Allocate
- [x] `a.queries.WithTx(tx)` called in Allocate
