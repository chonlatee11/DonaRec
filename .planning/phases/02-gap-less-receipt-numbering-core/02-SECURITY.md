---
phase: 02
slug: gap-less-receipt-numbering-core
status: verified
threats_open: 0
asvs_level: 1
created: 2026-06-27
---

# Phase 02 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.
> Verified by gsd-security-auditor (claude-sonnet-4-6) on 2026-06-27 — **SECURED, 14/14 threats closed**.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| app (`donnarec_app` role) → PostgreSQL | App issues counter/ledger writes; DB constraints (`UNIQUE`, `REVOKE`) are the un-bypassable backstop | receipt numbers, counter state |
| migration (superuser) → runtime (`donnarec_app`) | DDL + GRANT/REVOKE define what the runtime role may do | schema + privileges |
| caller (Phase 3 issuance/approval tx) → `Allocate` | Caller owns the tx; `Allocate` operates inside it and bubbles errors so caller rollback undoes the number; approval timestamp crosses in (must not trust wall clock) | `pgx.Tx`, approval time |
| concurrent callers → counter row | Many txs race for the same counter; `FOR UPDATE` must serialize them | counter row lock |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-02-01 | Tampering | receipt_numbers ledger rows | mitigate | `REVOKE UPDATE, DELETE ON receipt_numbers FROM donnarec_app` — `000004_receipt_number_tables.up.sql:108` (issued numbers immutable at DB level) | closed |
| T-02-02 | Tampering | duplicate (fiscal_year, running_no) | mitigate | `CONSTRAINT uq_receipt_numbers_fy_no UNIQUE (fiscal_year, running_no)` — `000004_receipt_number_tables.up.sql:82` | closed |
| T-02-03 | Tampering | SQL injection via dynamic SQL | mitigate | All 5 queries use named params (`@fiscal_year`/`@running_no`/`@formatted`) — `receiptno.sql:19-58`; no string concatenation; `allocator.go` calls only sqlc-generated methods | closed |
| T-02-04 | Elevation of Privilege | running_no sourced from a SEQUENCE (gap on rollback) | mitigate | `running_no INT NOT NULL CHECK (running_no >= 1)` (plain INT) — `000004_receipt_number_tables.up.sql:76`; grep confirms `SERIAL`/`nextval`/`MAX` absent from migration, queries, and `allocator.go` | closed |
| T-02-05 | Tampering | wrong fiscal year from clock skew / client timezone | mitigate | `t := issueDate.In(loc)` normalizes to Asia/Bangkok — `fiscal_year.go:76`; no `time.Now()` anywhere in `fiscal_year.go`; UTC-normalization proven by `fiscalyear_test.go:44-48` | closed |
| T-02-06 | Denial of Service | number wider than padding rejected/errored | mitigate | `fmt.Sprintf("%0*d", padding, runningNo)` minimum-width expansion — `format.go:55`; overflow case (`1000000` > 6-digit) asserted in `format_test.go:42-51` | closed |
| T-02-07 | Tampering | gap created by allocating in a separate tx | mitigate | `Allocate(ctx, tx pgx.Tx, issueDate)` takes a `pgx.Tx` (never a Pool) — `allocator.go:83`; `pgxpool.Pool`/`tx.Commit`/`tx.Rollback` absent from production code | closed |
| T-02-08 | Tampering | duplicate number via read-max+1 | mitigate | `LockCounterForUpdate` uses `FOR UPDATE` (`receiptno.sql:21`); `IncrementCounter` uses `SET last_running_no = last_running_no + 1` (`receiptno.sql:39`); `MAX(` absent | closed |
| T-02-09 | Repudiation | number reserved on a draft then abandoned | mitigate | Number born only at `qtx.InsertReceiptNumberLedger(...)` (`allocator.go:140`) inside caller tx; `Allocator` exposes only `Allocate` — no Reserve/Preview/precompute path | closed |
| T-02-10 | Tampering | duplicate receipt number under concurrency | mitigate | `FOR UPDATE` serializes allocations (`receiptno.sql:21`); `TestAllocator_Concurrency` asserts `COUNT(*) == COUNT(DISTINCT running_no)` for 50 parallel commits (`allocator_concurrency_test.go:107-114`); UNIQUE backstop fires in `TestAllocator_UniqueConstraintBackstop` (`:251`) | closed |
| T-02-11 | Tampering | gap created by rollback (SEQUENCE-style) | mitigate | Counter+ledger in same tx — rollback undoes both; `TestAllocator_Rollback` asserts no phantom row, counter at 0, reuse of running_no=1 (`allocator_rollback_test.go:84-134`); contiguous 1..M proven in `TestAllocator_RollbackMixedSequence` (`:218-243`) | closed |
| T-02-12 | Tampering | concurrent first-allocation race on new fiscal year | mitigate | `ON CONFLICT (fiscal_year) DO NOTHING` (`receiptno.sql:31`) + re-lock after `InitCounterRow` (`allocator.go:107-109`); `TestAllocator_ConcurrentNewYear` proves no dup/panic with 50 goroutines (`allocator_concurrency_test.go:138-215`) | closed |
| T-02-13 | Repudiation | test data race masking a real bug | mitigate | `sync.Mutex` guards committed slice in both test files (`allocator_concurrency_test.go:68-81`, `allocator_rollback_test.go:163-186`); 02-04-SUMMARY confirms all tests pass under `-race` | closed |
| T-02-SC | Tampering | go module installs (supply chain) | accept | No net-new external packages; promoted indirect→direct deps were already in `go.sum` before Phase 02 — see Accepted Risks Log | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-02-01 | T-02-SC | No net-new external packages. `golang.org/x/sync/errgroup` and `github.com/jackc/pgerrcode` were promoted indirect→direct for concurrency tests but were already present in `go.sum` before Phase 02; `pgx/v5/pgconn` is a subpackage of the existing direct dep `pgx/v5`. All from the project's existing pgx/jackc + Go x/ ecosystem; residual risk minimal. | Engineering | 2026-06-27 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-06-27 | 14 | 14 | 0 | gsd-security-auditor (claude-sonnet-4-6 / a8683fa50b6831788) |

---

## Notes

- All implementation files were treated read-only; this audit produced no code changes.
- `sync.Mutex` guards are present in all concurrent test harnesses. Actual `-race` execution is attested by the 02-04-SUMMARY self-check; it cannot be proven from source alone.
- The `REVOKE` on `receipt_numbers` covers `donnarec_app` only. A future `GRANT ALL` by a superuser would restore UPDATE/DELETE — documented as WR-03 in the migration file. Acceptable for Phase 02 scope.
- `fiscalYear()` and `formatReceiptNo()` are unexported — no external caller can bypass Asia/Bangkok normalization or minimum-width padding.
- Plans 01, 03, and 04 `## Threat Flags` sections report no new attack surface; no unregistered flags found.

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-06-27
