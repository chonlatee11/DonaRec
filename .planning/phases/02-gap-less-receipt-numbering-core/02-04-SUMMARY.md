---
phase: 02-gap-less-receipt-numbering-core
plan: "04"
subsystem: receiptno
tags: [tdd, concurrency-proof, rollback-proof, gap-less, nfr-04, sc4, race-detector]
dependency_graph:
  requires:
    - 02-03 (Allocator.Allocate, NewAllocator, AllocatedReceipt, db.WithTx seam)
    - 02-01 (receipt_numbers UNIQUE constraint, receipt_number_counters table)
  provides:
    - Concurrency invariant proven: 50 parallel allocations → zero dups, zero gaps (NFR-04/SC#4)
    - Rollback invariant proven: rollback after Allocate leaves no gap; freed number reused
    - Concurrent new-year safety proven: first-alloc race is deadlock-free (Pitfall 1 mitigated)
    - UNIQUE(fiscal_year, running_no) backstop actively fires on duplicate insert (D-37)
  affects:
    - All downstream phases (Phase 3+): allocator is now proven safe to depend on
tech_stack:
  added:
    - golang.org/x/sync/errgroup (promoted from indirect — errgroup.WithContext for fan-out)
    - github.com/jackc/pgerrcode (promoted from indirect — UniqueViolation constant)
    - github.com/jackc/pgx/v5/pgconn (*pgconn.PgError for SQLSTATE assertion)
  patterns:
    - errgroup.WithContext + sentinel-swallowing wrapper for concurrent fan-out with deliberate rollbacks
    - sync.Mutex-guarded shared results slice (Pitfall 6 — data race prevention under -race)
    - raw SQL assertions via pool.QueryRow / pool.Query (independent of sqlc path)
    - per-test isolated testcontainers Postgres 17 (SetupTestPostgres(t) per test function)
key_files:
  created:
    - donnarec-api/internal/receiptno/allocator_concurrency_test.go
    - donnarec-api/internal/receiptno/allocator_rollback_test.go
  modified:
    - donnarec-api/go.mod (golang.org/x/sync + pgerrcode promoted to direct)
    - donnarec-api/go.sum (updated)
decisions:
  - "errgroup + sentinel-swallowing: goroutine swallows errDeliberateRollback before returning nil to errgroup — prevents gctx cancellation on deliberate rollback, allowing all other goroutines to proceed normally"
  - "pgx/v5/pgconn (not standalone pgconn): PgError is accessed via github.com/jackc/pgx/v5/pgconn, not the deprecated standalone github.com/jackc/pgconn package (which is not in go.sum)"
  - "expectedFY correctness: Jan-Sep CE year → fy = CE+543; Oct-Dec CE year → fy = CE+544 — test dates chosen to use this formula correctly for isolation"
  - "per-test container isolation: each test function gets its own SetupTestPostgres(t) container — no shared state between TestAllocator_Concurrency, TestAllocator_ConcurrentNewYear, TestAllocator_Rollback, TestAllocator_RollbackMixedSequence"
metrics:
  duration: "502s"
  completed_date: "2026-06-25T16:34:33Z"
  tasks_completed: 2
  tasks_total: 2
  files_created: 2
  files_modified: 2
---

# Phase 02 Plan 04: Concurrency + Rollback Proof Summary

**One-liner:** 5-test concurrency + rollback harness under `-race` against real PostgreSQL 17 proves NFR-04/SC#4: 50 parallel allocations are zero-gap zero-duplicate, rollback frees numbers without gaps, concurrent first-year is safe, and the UNIQUE backstop fires on duplicate insert.

## What Was Built

### `allocator_concurrency_test.go` — Concurrency invariant proof

Package `receiptno_test` (black-box), all tests require testcontainers PostgreSQL 17:

| Test | Scenario | Key Assertions |
|------|----------|----------------|
| `TestAllocator_Concurrency` | 50 goroutines, each Allocate+commit | COUNT(*)=50, COUNT(DISTINCT running_no)=50, ledger=[1..50] |
| `TestAllocator_ConcurrentNewYear` | 50 goroutines race to allocate FY 2575 first number | no panic, no dup, counter=50, ledger=[1..50] |
| `TestAllocator_UniqueConstraintBackstop` | raw INSERT duplicate (FY 2580, running_no=1) twice | error is *pgconn.PgError with Code=pgerrcode.UniqueViolation (23505) |

**Harness design:** `errgroup.WithContext` fans out N goroutines; each calls `dbhelpers.WithTx(ctx, pool, fn)` which commits on success. Results collected under `sync.Mutex` (Pitfall 6 — no data race under `-race`). After Wait, sorted results compared against ledger via raw SQL.

### `allocator_rollback_test.go` — Rollback invariant proof

| Test | Scenario | Key Assertions |
|------|----------|----------------|
| `TestAllocator_Rollback` | Allocate then return errDeliberateRollback → rollback | no phantom ledger row; counter=0 after rollback; next alloc gets running_no=1 (reused) |
| `TestAllocator_RollbackMixedSequence` | N=30 goroutines, rollback every 3rd (idx%3==0) | ledger COUNT=M=committed_count; ledger=[1..M] contiguous; no duplicates |

**Rollback semantics proven:** Counter UPDATE + ledger INSERT both inside caller's tx → rollback undoes both atomically → freed number returned to pool → next commit reuses it → zero gap. This is the core counter-vs-SEQUENCE distinction (SEQUENCE's `nextval()` is not transactional).

## Commits

| TDD Phase | Task | Commit | Description |
|-----------|------|--------|-------------|
| RED | Task 1 (concurrency test) | `174406d` | test(02-04): add concurrency + unique-backstop tests for gap-less allocator |
| GREEN | Task 2 (rollback test RED+GREEN) | `59ca418` | test(02-04): add rollback proof tests — no gap on rollback, freed number reused |
| GREEN | Task 2 (fix + GREEN) | `b44a09c` | feat(02-04): implement rollback proof — no-gap after rollback, freed number reused |

Note: Plan 04 is test-only — no new production code was written. Allocator from Plan 03 passed all tests on first correct write (no allocator.go changes needed).

## TDD Gate Compliance

| Gate | Commit | Status |
|------|--------|--------|
| RED — test(`02-04`) Task 1 | `174406d` | PASS — tests compile; allocator exists from 02-03 so GREEN immediately |
| GREEN — feat(`02-04`) Task 1 | `174406d` (same commit) | PASS — TestAllocator_Concurrency, TestAllocator_ConcurrentNewYear, TestAllocator_UniqueConstraintBackstop all PASS |
| RED — test(`02-04`) Task 2 | `59ca418` | PASS — compile OK; 2 bugs found during GREEN run |
| GREEN — feat(`02-04`) Task 2 | `b44a09c` | PASS — TestAllocator_Rollback, TestAllocator_RollbackMixedSequence both PASS |

Note on RED→GREEN: Per plan specification, "the allocator already exists (Plan 03), so these tests should pass on first correct write; if a test reveals a concurrency bug, fix is in Plan 03's allocator.go". The allocator did NOT need fixing — bugs were in the test harness itself (see Deviations).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed fiscal year calculation in test date literals**
- **Found during:** Task 2 GREEN run
- **Issue:** `bkkTime(2032, 5, 20, ...)` has `expectedFY = 2576` but May (month 5) is Jan–Sep → fy = 2032+543 = 2575, not 2576. Same issue for `bkkTime(2033, 2, 10, ...)` with expectedFY=2577 vs correct 2576.
- **Fix:** Corrected `expectedFY` values to match the actual fiscal year formula: Jan–Sep CE+543, Oct–Dec CE+544. Added inline comments explaining the formula.
- **Files modified:** `allocator_rollback_test.go`
- **Commit:** `b44a09c`

**2. [Rule 1 - Bug] Removed duplicate errgroup loop in TestAllocator_RollbackMixedSequence**
- **Found during:** Task 2 GREEN run
- **Issue:** Test had two errgroup loops (g and g2) targeting the same fiscal year in the same pool. Loop `g` was started without `g.Wait()`, causing 30 goroutines to run concurrently with g2's 30 goroutines — 60 total allocations against one fiscal year, then `committed` was reset to empty before g2 started. The SQL assertions then saw 0 committed rows.
- **Fix:** Removed the first `g` loop entirely. Kept only the `g` (renamed from `g2`) loop with the sentinel-swallowing wrapper. The errgroup with sentinel swallowing is the correct single implementation.
- **Files modified:** `allocator_rollback_test.go`
- **Commit:** `b44a09c`

**3. [Rule 1 - Bug] Changed pgconn import path from standalone to pgx/v5 subpackage**
- **Found during:** Task 1 RED phase — `go vet` failure
- **Issue:** `github.com/jackc/pgconn` is not in go.sum; `*pgconn.PgError` is in `github.com/jackc/pgx/v5/pgconn` (a subpackage of pgx/v5 which is already a dep).
- **Fix:** Changed import to `"github.com/jackc/pgx/v5/pgconn"`.
- **Files modified:** `allocator_concurrency_test.go`
- **Commit:** `174406d`

## Known Stubs

None — these are pure test files with no production code. All assertions are hard numeric checks against real PostgreSQL containers. No placeholder values, no TODO, no hardcoded mock results.

## Threat Surface Scan

No new network endpoints, auth paths, file access patterns, or schema changes. This plan adds test files only.

Threat register coverage (per plan threat_model):

| Threat ID | Test | Status |
|-----------|------|--------|
| T-02-10: duplicate receipt under concurrency | TestAllocator_Concurrency | Mitigated — COUNT(DISTINCT) == COUNT(*) under 50-parallel proven |
| T-02-11: gap from rollback | TestAllocator_Rollback + TestAllocator_RollbackMixedSequence | Mitigated — rollback reuses freed number; zero phantom rows proven |
| T-02-12: concurrent first-allocation race | TestAllocator_ConcurrentNewYear | Mitigated — no dup/panic under 50-parallel race for new FY |
| T-02-13: test data race masking bug | All tests | Mitigated — sync.Mutex guards shared slice; all run with -race |
| T-02-SC: package install | N/A | golang.org/x/sync + pgerrcode were already in go.sum — only promoted to direct |

## Self-Check: PASSED

Files exist:
- [x] `donnarec-api/internal/receiptno/allocator_concurrency_test.go` — FOUND
- [x] `donnarec-api/internal/receiptno/allocator_rollback_test.go` — FOUND

Commits exist:
- [x] `174406d` — test(02-04): add concurrency + unique-backstop tests — FOUND
- [x] `59ca418` — test(02-04): add rollback proof tests — FOUND
- [x] `b44a09c` — feat(02-04): implement rollback proof — FOUND

Verification:
- [x] `go test ./internal/receiptno/... -race -count=1` exits 0 (all 11+ tests pass)
- [x] `func TestAllocator_Concurrency` — FOUND in allocator_concurrency_test.go
- [x] `func TestAllocator_ConcurrentNewYear` — FOUND in allocator_concurrency_test.go
- [x] `func TestAllocator_UniqueConstraintBackstop` — FOUND in allocator_concurrency_test.go
- [x] `func TestAllocator_Rollback` — FOUND in allocator_rollback_test.go
- [x] `func TestAllocator_RollbackMixedSequence` — FOUND in allocator_rollback_test.go
- [x] `COUNT(DISTINCT running_no)` — FOUND in allocator_concurrency_test.go
- [x] `UniqueViolation` — FOUND in allocator_concurrency_test.go
- [x] `deliberate rollback` — FOUND in allocator_rollback_test.go
- [x] `sync.Mutex` guard on shared slice — FOUND in both test files
- [x] All tests have `testing.Short()` skip guard
