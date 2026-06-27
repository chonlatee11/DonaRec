---
phase: 2
slug: gap-less-receipt-numbering-core
status: complete
nyquist_compliant: true
wave_0_complete: true
created: 2026-06-25
validated: 2026-06-27
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test + testify + testcontainers-go (Postgres 17) |
| **Config file** | none — uses `donnarec-api/internal/testutil/postgres.go` fixture |
| **Quick run command** | `cd donnarec-api && go test ./internal/receiptno/... -count=1` |
| **Full suite command** | `cd donnarec-api && go test ./... -count=1` |
| **Race-detector command** | `cd donnarec-api && go test ./internal/receiptno/... -race -count=1` |
| **Estimated runtime** | ~30–90 seconds (testcontainers Postgres spin-up dominates) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/receiptno/... -count=1`
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 90 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| DB schema (config/counter/ledger) | 02-01 | W0 | FR-16, NFR-04 | T-02-01/02/03/04 | REVOKE UPDATE/DELETE immutable ledger; UNIQUE(fiscal_year, running_no) backstop; sqlc named params (no string concat); plain INT not SERIAL | integration (exercised via allocator tests) | `go test ./internal/receiptno/... -count=1` | ✅ | ✅ green |
| `fiscalYear()` helper | 02-02 | W0 | FR-17, FR-18, SC#2 | T-02-05 | Asia/Bangkok + BE normalisation; never calls `time.Now()` | unit (pure fn) | `go test ./internal/receiptno/... -run TestFiscalYear -count=1` | ✅ | ✅ green |
| `formatReceiptNo()` helper | 02-02 | W0 | FR-15, SC#1 | T-02-06 | config-driven format; `%0*d` min-width expands without truncation | unit (pure fn) | `go test ./internal/receiptno/... -run TestFormatReceiptNo -count=1` | ✅ | ✅ green |
| `Allocator.Allocate` (caller-managed tx) | 02-03 | W0 | FR-16, SC#3, SC#5 | T-02-07/08/09 | single code path; `pgx.Tx` not `pgxpool.Pool`; no `MAX()+1`; number born only at ledger INSERT | integration | `go test ./internal/receiptno/... -run TestAllocator_Single\|Sequential\|NewFiscalYear\|MultiYear\|DefaultConfig -count=1` | ✅ | ✅ green |
| Concurrency proof | 02-04 | W0 | NFR-04, SC#4 | T-02-10/12/13 | 50-parallel → COUNT(DISTINCT)=COUNT(*); first-year race deadlock-free; UNIQUE backstop fires (23505) | integration (`-race`) | `go test ./internal/receiptno/... -race -run TestAllocator_Concurrency\|ConcurrentNewYear\|UniqueConstraintBackstop -count=1` | ✅ | ✅ green |
| Rollback proof | 02-04 | W0 | FR-16, SC#4 | T-02-11 | rollback leaves no phantom row; freed number reused; ledger contiguous [1..M] | integration (`-race`) | `go test ./internal/receiptno/... -race -run TestAllocator_Rollback -count=1` | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

**Result:** 12 test functions / 22 cases — all PASS under `-race` against PostgreSQL 17 (validated 2026-06-27).

---

## Wave 0 Requirements

- [x] `internal/receiptno/fiscalyear_test.go` — boundary tests for `fiscalYear()` (30 Sep 23:59 / 1 Oct 00:00) — SC#2 / FR-17, FR-18
- [x] `internal/receiptno/allocator_concurrency_test.go` — N-parallel allocation harness asserting zero gaps + zero dupes — SC#4 / NFR-04
- [x] `internal/receiptno/allocator_rollback_test.go` — rollback leaves no gap; UNIQUE backstop fires — SC#4 / FR-16
- [x] reuse `internal/testutil/postgres.go` — shared testcontainers Postgres fixture (no new framework install needed)

Additional coverage delivered beyond the Wave 0 minimum:
- [x] `internal/receiptno/format_test.go` — config-driven format + min-width expansion — SC#1 / FR-15
- [x] `internal/receiptno/allocator_test.go` — sequential gap-less, per-year reset, multi-year isolation — SC#3 / FR-16

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| (none) | — | — | — |

*All phase behaviors target automated verification — this is a backend-only, test-proven phase. SC#5 (single code path / no pre-compute) is verified by automated anti-pattern grep over `allocator.go` (UAT #7): `pgxpool.Pool`, `MAX(`, `time.Now(`, `nextval`, `tx.Commit`, `tx.Rollback` all absent from production code.*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 90s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved (2026-06-27) — zero gaps; all requirements have automated, race-proven verification.

---

## Validation Audit 2026-06-27

| Metric | Count |
|--------|-------|
| Gaps found | 0 |
| Resolved | 0 |
| Escalated | 0 |

Audit method: read all PLAN/SUMMARY artifacts + UAT, cross-referenced 6 task groups against 12 existing test functions, and re-ran `go test ./internal/receiptno/... -race -count=1` → 22 cases PASS. No tests generated — coverage was already complete; the prior VALIDATION.md was a pre-execution draft stub that this audit reconciled with the shipped implementation.
