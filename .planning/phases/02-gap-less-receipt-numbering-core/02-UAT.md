---
status: complete
phase: 02-gap-less-receipt-numbering-core
source: [02-01-SUMMARY.md, 02-02-SUMMARY.md, 02-03-SUMMARY.md, 02-04-SUMMARY.md]
started: 2026-06-27T12:15:00Z
updated: 2026-06-27T12:18:00Z
verified_by: automated test suite (go test -race against PostgreSQL 17) + code-path grep
---

## Current Test

[testing complete — all 7 success criteria verified via automated evidence]

## Tests

### 1. Fiscal year boundary correctness (SC#2)
expected: fiscalYear(issueDate) pinned Asia/Bangkok + BE; ถูกต้องที่ขอบ 30 ก.ย. 23:59 / 1 ต.ค. 00:00 (Oct–Dec → ปี BE ถัดไป); ไม่เรียก time.Now()
result: pass
evidence: TestFiscalYear PASS (6 boundary cases — 30 Sep 23:59 BKK→2568, 1 Oct 00:00 BKK→2569, 30 Sep 17:00 UTC = 1 Oct BKK→2569). time.Now() absent in fiscal_year.go production code.

### 2. Receipt number format from config (SC#1)
expected: เลขใบเสร็จ = year + separator + zero-padded running (เช่น "2569/000123"); separator/padding/year-format อ่านจาก config; padding ขยายเกิน 6 หลักได้โดยไม่ truncate
result: pass
evidence: TestFormatReceiptNo PASS (default "2569/000123", expansion 1000000→"2569/1000000", "HOSP2569-0005", CE4 branch) + TestAllocator_DefaultConfigFormat PASS (config-driven BE4 + "/" + 6-pad, frozen snapshot matches ledger).

### 3. Sequential gap-less + per-year reset (SC#3)
expected: จัดสรรต่อเนื่องได้ 1,2,3 ไม่มี gap; ขึ้นปีงบใหม่ running เริ่มที่ 1 อัตโนมัติ (counter keyed ต่อปี ไม่มี job reset); หลายปีแยกกันอิสระ
result: pass
evidence: TestAllocator_SequentialGapless PASS (1,2,3 no gaps) + TestAllocator_NewFiscalYearStartsAtOne PASS (new FY starts at 1) + TestAllocator_MultiYearIsolation PASS (FY 2570 independent of 2569).

### 4. Concurrency: zero gaps, zero duplicates (SC#4)
expected: 50 goroutines จัดสรรพร้อมกัน → COUNT=50, DISTINCT running_no=50, ledger=[1..50]; การแย่งจัดสรรเลขแรกของปีใหม่ปลอดภัย ไม่ panic ไม่ซ้ำ (รันภายใต้ -race)
result: pass
evidence: TestAllocator_Concurrency PASS (2.42s, 50 parallel, COUNT=DISTINCT=50) + TestAllocator_ConcurrentNewYear PASS (1.85s, first-year race deadlock-free). No DATA RACE under -race.

### 5. Rollback leaves no gap; freed number reused (SC#4)
expected: rollback หลัง Allocate → ไม่มี phantom row, counter ไม่เดิน; เลขที่ถูก rollback ถูก reuse โดย allocation ถัดไป; mixed sequence → ledger contiguous [1..M] ไม่มี gap/ซ้ำ
result: pass
evidence: TestAllocator_Rollback PASS (1.77s, freed number reused, no phantom row) + TestAllocator_RollbackMixedSequence PASS (1.98s, rollback every 3rd → ledger contiguous, no dup).

### 6. UNIQUE(fiscal_year, running_no) backstop fires (SC#4)
expected: INSERT เลขซ้ำ (fiscal_year, running_no) โดน DB ปฏิเสธด้วย *pgconn.PgError code 23505 (UniqueViolation)
result: pass
evidence: TestAllocator_UniqueConstraintBackstop PASS (1.73s, duplicate INSERT → *pgconn.PgError Code=23505 UniqueViolation).

### 7. Single code path, no pre-compute/reserve (SC#5)
expected: Allocate เป็นทางเดียวที่ออกเลข; ไม่ใช้ pgxpool.Pool ใน signature, ไม่มี MAX()+1, ไม่เรียก time.Now(), เลขเกิดเฉพาะตอน ledger INSERT ใน issuance tx
result: pass
evidence: grep allocator.go production code — pgxpool.Pool / MAX( / time.Now( / nextval / tx.Commit / tx.Rollback all ABSENT. Allocate(ctx, pgx.Tx, issueDate) caller-managed; number born only at InsertReceiptNumberLedger after LockCounterForUpdate+IncrementCounter; AllocatedAt from DB now().

## Summary

total: 7
passed: 7
issues: 0
pending: 0
skipped: 0

## Gaps

[none — all 7 success criteria verified, 12/12 Go tests pass under -race against PostgreSQL 17]
