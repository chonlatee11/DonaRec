---
phase: 02-gap-less-receipt-numbering-core
verified: 2026-06-27T00:00:00Z
status: passed
score: 5/5 must-haves verified
overrides_applied: 0
re_verification:
  previous_status: none
requirements_covered:
  - FR-15
  - FR-16
  - FR-17
  - FR-18
  - NFR-04
gaps: []
human_verification: []
---

# Phase 02: Gap-less Receipt Numbering Core — Verification Report

**Phase Goal:** The system can allocate a unique, gap-less, per-fiscal-year receipt running number inside a single short DB transaction, and this invariant is proven under concurrency and rollback before any UI depends on it.

**Verified:** 2026-06-27
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth (Success Criterion) | Status | Evidence |
|---|---|---|---|
| 1 | Formatted number = fiscal year + zero-padded running number (`2569/000123`), separator + padding from config | ✓ VERIFIED | `format.go:40-59` renders `prefix+year+separator+running`; config read from DB in `allocator.go:121` (`GetReceiptNumberConfig`) → `receiptno.sql:48-50` reads `separator, running_no_padding, year_format, prefix`. `TestAllocator_DefaultConfigFormat` (`allocator_test.go:234-265`) asserts `"2569/000001"` from seeded config; `format_test.go:30-74` covers default/expand/prefix/CE4 |
| 2 | Pure `fiscalYear(issueDate)`, Asia/Bangkok + Buddhist-era, correct at 30 Sep 23:59 / 1 Oct 00:00 boundaries, unit-tested | ✓ VERIFIED | `fiscal_year.go:71-90` normalises via `issueDate.In(loadBangkok())`, `month >= October → ce+544 else ce+543`; no `time.Now()`. `fiscalyear_test.go:29-66` asserts Sep 30 23:59:59→2568, Oct 1 00:00→2569, UTC 17:00 Sep30 (=BKK Oct1)→2569, Oct 1 2026→2570 |
| 3 | Running number resets to 1 per fiscal year automatically (counter keyed per FY, no scheduled job) | ✓ VERIFIED | Counter keyed `PRIMARY KEY (fiscal_year)` (`000004...up.sql:54-62`); new-FY path auto-inits via `InitCounterRow` (`allocator.go:99-110`, `receiptno.sql:23-31`). `TestAllocator_NewFiscalYearStartsAtOne` (`allocator_test.go:145-177`) and `TestAllocator_MultiYearIsolation` (`:181-229`) prove FY 2570/2571 start at 1 independent of 2569. No cron/job code present |
| 4 | Concurrency + rollback test asserts zero gaps + zero dups; `UNIQUE(fiscal_year, running_no)` backstop | ✓ VERIFIED | `UNIQUE` constraint `uq_receipt_numbers_fy_no` (`000004...up.sql:82`). `TestAllocator_Concurrency` (50 parallel, contiguous 1..50, COUNT==COUNT DISTINCT, `allocator_concurrency_test.go:54-131`), `TestAllocator_ConcurrentNewYear` (:138-216), `TestAllocator_UniqueConstraintBackstop` asserts SQLSTATE 23505 (:224-253), `TestAllocator_Rollback` + `TestAllocator_RollbackMixedSequence` prove freed number reused, no gap (`allocator_rollback_test.go:52-244`). Full suite green under `-race` |
| 5 | Allocator is the only path to hand out a number; never pre-computes/reserves on a draft | ✓ VERIFIED | grep confirms NO caller of `IncrementCounter`/`InsertReceiptNumberLedger`/`InitCounterRow`/`LockCounterForUpdate` outside `internal/receiptno` + generated code. Number is "born" only at ledger INSERT (`allocator.go:140-147`); caller-managed tx, no `tx.Commit`/`Rollback`, no `time.Now()`, no `MAX(running_no)` (verified by grep). No draft reservation logic exists |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|---|---|---|---|
| `internal/receiptno/allocator.go` | Sole allocator, FOR UPDATE + increment + ledger insert in caller tx | ✓ VERIFIED | 157 lines, full 8-step flow, caller-managed tx, snapshot return |
| `internal/receiptno/fiscal_year.go` | Pure BKK/BE fiscal year helper | ✓ VERIFIED | `sync.Once` tzdata load, normalise + boundary logic |
| `internal/receiptno/format.go` | Config-driven format renderer | ✓ VERIFIED | min-width padding (`%0*d`), BE4/CE4, prefix |
| `internal/db/queries/receiptno.sql` | sqlc queries: lock/init/increment/config/ledger | ✓ VERIFIED | parameterized, `SELECT ... FOR UPDATE`, `ON CONFLICT DO NOTHING` |
| `migrations/000004_receipt_number_tables.up.sql` | counter + ledger + config, UNIQUE, REVOKE | ✓ VERIFIED | UNIQUE backstop + `REVOKE UPDATE, DELETE` on ledger; seeded default config |
| `*_test.go` (5 files) | unit + integration (concurrency/rollback) | ✓ VERIFIED | Pass under `-race` in 19.8s — integration tests actually executed |

### Key Link Verification

| From | To | Via | Status | Details |
|---|---|---|---|---|
| `allocator.go` | counter row | `LockCounterForUpdate` (SELECT FOR UPDATE) | WIRED | `allocator.go:94`, `receiptno.sql:18-21` |
| `allocator.go` | counter increment | `IncrementCounter` (UPDATE RETURNING) | WIRED | `allocator.go:114`, `receiptno.sql:37-42` |
| `allocator.go` | config | `GetReceiptNumberConfig` in same tx | WIRED | `allocator.go:121`, `receiptno.sql:44-50` |
| `allocator.go` | ledger | `InsertReceiptNumberLedger` (UNIQUE backstop) | WIRED | `allocator.go:140`, `receiptno.sql:52-59` |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|---|---|---|---|
| Full unit + integration suite (incl. concurrency/rollback) under race | `go test ./internal/receiptno/... -race -count=1` | `ok ... 19.775s` (exit 0) | ✓ PASS |

### Requirements Coverage

| Requirement | Status | Evidence |
|---|---|---|
| FR-15 (configurable receipt number format) | ✓ SATISFIED | `receipt_number_config` table + `format.go` + `GetReceiptNumberConfig` |
| FR-16 (gap-less running number) | ✓ SATISFIED | counter-table + FOR UPDATE; rollback tests prove no gap |
| FR-17 (per-fiscal-year reset) | ✓ SATISFIED | counter keyed per FY; new-year auto-init tests |
| FR-18 (Thai BE fiscal year boundary) | ✓ SATISFIED | `fiscal_year.go` + boundary unit tests |
| NFR-04 (concurrency-safe, no dup/gap) | ✓ SATISFIED | 50-way concurrency test + UNIQUE backstop under `-race` |

### Anti-Patterns Found

| File | Pattern | Severity | Impact |
|---|---|---|---|
| (none) | debt markers TBD/FIXME/XXX/HACK in phase files | — | None found |

No `SEQUENCE`/`SERIAL` for running_no, no `MAX(running_no)+1`, no `time.Now()` in allocation path, no caller-side pre-computation — all CLAUDE.md "What NOT to Use" rules respected.

### Gaps Summary

None. All five success criteria are observably true in the codebase and proven by a green `-race` test suite that exercises real PostgreSQL via testcontainers for concurrency and rollback. The hardest invariant (gap-less + zero-dup under 50-way concurrency, plus reuse-on-rollback and a UNIQUE backstop) is directly tested and passing.

---

_Verified: 2026-06-27_
_Verifier: Claude (gsd-verifier)_
