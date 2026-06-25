---
phase: 02-gap-less-receipt-numbering-core
reviewed: 2026-06-25T16:42:12Z
depth: standard
files_reviewed: 11
files_reviewed_list:
  - donnarec-api/migrations/000004_receipt_number_tables.up.sql
  - donnarec-api/migrations/000004_receipt_number_tables.down.sql
  - donnarec-api/internal/db/queries/receiptno.sql
  - donnarec-api/internal/receiptno/allocator.go
  - donnarec-api/internal/receiptno/fiscal_year.go
  - donnarec-api/internal/receiptno/format.go
  - donnarec-api/internal/receiptno/allocator_test.go
  - donnarec-api/internal/receiptno/allocator_concurrency_test.go
  - donnarec-api/internal/receiptno/allocator_rollback_test.go
  - donnarec-api/internal/receiptno/fiscalyear_test.go
  - donnarec-api/internal/receiptno/format_test.go
findings:
  critical: 1
  warning: 4
  info: 2
  total: 7
status: issues_found
---

# Phase 02: Code Review Report

**Reviewed:** 2026-06-25T16:42:12Z
**Depth:** standard
**Files Reviewed:** 11
**Status:** issues_found

## Summary

ทบทวน 11 ไฟล์ที่ครอบคลุม path หลักของการออกเลขใบเสร็จแบบ gap-less ได้แก่ migration, SQL queries, allocator, fiscal year helper, format renderer และ test suite ทั้งสาม (unit, concurrency, rollback)

โดยรวม design สอดคล้องกับข้อกำหนดของโปรเจกต์อย่างแม่นยำ: ใช้ counter table + `SELECT … FOR UPDATE` ไม่มี SEQUENCE/SERIAL สำหรับ running_no, transaction boundary เป็น caller-managed, fiscal year คำนวณด้วย Asia/Bangkok, UNIQUE backstop อยู่ครบ และ REVOKE UPDATE/DELETE บน ledger ถูกต้อง

พบ **1 BLOCKER** (data race บน package-level var ที่ตรวจพบได้ด้วย `-race`), **4 WARNING** และ **2 INFO**

---

## Critical Issues

### CR-01: Data race บน `bangkokLoc` package-level variable ใน `fiscal_year.go`

**File:** `donnarec-api/internal/receiptno/fiscal_year.go:25-43`

**Issue:** `bangkokLoc` เป็น package-level `*time.Location` ที่ `loadBangkok()` อ่านและเขียนโดยไม่มีการป้องกัน concurrent access เลย pattern ที่ใช้คือ check-then-set:

```go
// line 31-41
if bangkokLoc != nil {
    return bangkokLoc
}
loc, err := time.LoadLocation("Asia/Bangkok")
...
bangkokLoc = loc   // unprotected write
return loc
```

เมื่อ 50 goroutine จาก `TestAllocator_Concurrency` เรียก `alloc.Allocate()` พร้อมกัน แต่ละ goroutine เรียก `fiscalYear()` ซึ่งเรียก `loadBangkok()` หลาย goroutine จะเห็น `bangkokLoc == nil` พร้อมกัน และแต่ละตัวจะเขียนไปที่ตัวแปรเดียวกันโดยไม่มี lock Go race detector (`-race`) ซึ่งทั้ง `TestAllocator_Concurrency` และ `TestAllocator_ConcurrentNewYear` ประกาศว่ารันภายใต้ `-race` จะ **รายงาน DATA RACE** ทำให้ test ล้มเหลว

ผลลัพธ์ที่ได้ (การอ่านซ้ำ `time.Location` ที่เหมือนกัน) ไม่มี semantic bug แต่การมี unsynchronized concurrent read/write บน pointer เป็น undefined behavior ใน Go memory model

**Fix:** ใช้ `sync.Once`:

```go
import (
    "sync"
    "time"
)

var (
    bangkokOnce sync.Once
    bangkokLoc  *time.Location
)

func loadBangkok() *time.Location {
    bangkokOnce.Do(func() {
        loc, err := time.LoadLocation("Asia/Bangkok")
        if err != nil {
            panic("Asia/Bangkok timezone not available: " + err.Error())
        }
        bangkokLoc = loc
    })
    return bangkokLoc
}
```

---

## Warnings

### WR-01: Index ซ้ำกับ UNIQUE constraint บน `(fiscal_year, running_no)`

**File:** `donnarec-api/migrations/000004_receipt_number_tables.up.sql:79,83`

**Issue:** `CONSTRAINT uq_receipt_numbers_fy_no UNIQUE (fiscal_year, running_no)` สร้าง B-tree index อัตโนมัติบน `(fiscal_year, running_no)` อยู่แล้ว แต่ migration ยังสร้าง `idx_receipt_numbers_fy_no` บน column เดิมซ้ำอีกครั้ง:

```sql
CONSTRAINT uq_receipt_numbers_fy_no UNIQUE (fiscal_year, running_no)   -- line 79: creates implicit index
...
CREATE INDEX idx_receipt_numbers_fy_no ON receipt_numbers (fiscal_year, running_no);  -- line 83: duplicate
```

index สองตัวนี้มี structure เหมือนกันทุกประการ PostgreSQL จะ maintain ทั้งสองตัวในทุก INSERT — ทำให้ write path (ซึ่งคือ hot path ของ allocation) ช้าลงโดยไม่จำเป็น และเปลืองพื้นที่ index สองเท่า

**Fix:** ลบ `CREATE INDEX idx_receipt_numbers_fy_no` ออก และใช้ `uq_receipt_numbers_fy_no` ที่สร้างโดย UNIQUE constraint ซึ่งทำหน้าที่เป็น index อยู่แล้ว ทุก query ที่ระบุ `fiscal_year` + `running_no` จะใช้ unique-constraint index ได้โดยตรง

---

### WR-02: Comment เท็จ — `INSERT INTO receipt_number_config DEFAULT VALUES` ไม่มี `ON CONFLICT`

**File:** `donnarec-api/migrations/000004_receipt_number_tables.up.sql:42-43`

**Issue:** Comment บรรทัด 42 บอกว่า:
```sql
-- ON CONFLICT ensures idempotent re-runs without error.
INSERT INTO receipt_number_config DEFAULT VALUES;
```

แต่ SQL จริงไม่มี `ON CONFLICT` clause เลย หากนำ SQL ไปรันซ้ำโดยตรง (เช่น manual apply หรือสคริปต์ที่ไม่ผ่าน golang-migrate) จะได้รับ:
```
ERROR: duplicate key value violates unique constraint "receipt_number_config_pkey"
```

comment ที่ผิดนี้จะทำให้ DBA หรือผู้ดูแลระบบสรุปผิดว่า statement นี้ re-runnable อย่างปลอดภัย

**Fix:** แก้ SQL ให้ตรงกับ comment:
```sql
-- ON CONFLICT ensures idempotent re-runs without error.
INSERT INTO receipt_number_config DEFAULT VALUES
ON CONFLICT (id) DO NOTHING;
```

---

### WR-03: Comment เท็จเกี่ยวกับ `REVOKE` — future `GRANT` สามารถ re-grant ได้เสมอ

**File:** `donnarec-api/migrations/000004_receipt_number_tables.up.sql:101-103`

**Issue:** Comment กล่าวว่า:
```sql
-- Immutable ledger enforcement (T-02-01): no UPDATE or DELETE allowed at DB level.
-- Even a future GRANT ALL will not restore these until explicitly re-granted.
REVOKE UPDATE, DELETE ON receipt_numbers FROM donnarec_app;
```

ประโยค "a future GRANT ALL will not restore these" เป็น **ข้อมูลที่ผิด** ใน PostgreSQL `REVOKE` เพียงแค่เพิกถอน privilege ที่ grant ไปแล้ว แต่ไม่ได้ตั้ง "deny rule" ถาวร superuser ที่รัน `GRANT UPDATE ON receipt_numbers TO donnarec_app` ในภายหลังจะคืน privilege นั้นได้ทันที comment นี้อาจทำให้ทีม DBA เข้าใจผิดว่า REVOKE ป้องกัน future grant ได้

**Fix:** แก้ comment ให้ถูกต้อง:
```sql
-- Immutable ledger enforcement (T-02-01): no UPDATE or DELETE allowed at DB level.
-- REVOKE strips current privileges from donnarec_app; a future explicit GRANT can re-grant them.
-- Production hardening: monitor pg_roles / audit any GRANT on receipt_numbers.
REVOKE UPDATE, DELETE ON receipt_numbers FROM donnarec_app;
```

---

### WR-04: No-op `fmt.Sprintf("%s", appConnStr)` ใน `testutil/postgres.go`

**File:** `donnarec-api/internal/testutil/postgres.go:127`

**Issue:** บรรทัด 127 เป็น dead code ที่ไม่ทำอะไรเลย:
```go
appConnStr = fmt.Sprintf("%s", appConnStr)
```

`fmt.Sprintf("%s", x)` กับ `string` argument จะ return ค่าเดิมทุกประการ บรรทัดนี้อาจเหลือมาจากการ refactor ที่ไม่สมบูรณ์ (อาจตั้งใจจะ format URL แต่ logic ถูกย้ายออก) แต่ยังคงอยู่ ทำให้โค้ดน่าสับสน และ `golangci-lint` อาจ report ว่าเป็น redundant statement

**Fix:** ลบบรรทัดนั้นออก:
```go
// Remove: appConnStr = fmt.Sprintf("%s", appConnStr)
// appConnStr is already built by strings.Replace on the line above; no further transformation needed.
```

---

## Info

### IN-01: `TestAllocator_ConcurrentNewYear` และ `TestAllocator_Rollback` ต่างอ้างสิทธิ์ FY 2575 ว่า "unused by any other test"

**File:** `donnarec-api/internal/receiptno/allocator_concurrency_test.go:134,148` และ `donnarec-api/internal/receiptno/allocator_rollback_test.go:62`

**Issue:** ทั้งสองฟังก์ชัน comment ว่า FY 2575 เป็น "unused by any other test":
- `allocator_concurrency_test.go:134`: "FY 2575 — unused by any other test"
- `allocator_rollback_test.go:62`: "FY 2575: unused fiscal year — guaranteed clean counter"

แต่ทั้งคู่ใช้ FY 2575 จริง (Nov 2031 = CE+544 = 2575; May 2032 = CE+543 = 2575) ไม่มี runtime bug เพราะแต่ละ test เรียก `SetupTestPostgres(t)` ซึ่ง spin up container แยกกัน แต่ comment ทั้งสองผิดและสร้างความสับสนให้คนอ่าน

**Fix:** แก้ comment ใน `allocator_rollback_test.go` ให้ใช้ FY อื่นที่ยังว่างจริง เช่น FY 2577 (Feb 2034 BKK) หรืออัปเดต comment ว่า "each test has its own isolated container so FY reuse is safe":

```go
// TestAllocator_Rollback: FY 2577 — isolated per test container; FY 2575 is used by TestAllocator_ConcurrentNewYear.
issueDate := bkkTime(2034, 2, 20, 10, 0, 0) // Feb 20 2034 BKK → FY 2577
const expectedFY = 2577
```

---

### IN-02: `year_format` column ใน `receipt_number_config` ไม่มี CHECK constraint สำหรับ valid values

**File:** `donnarec-api/migrations/000004_receipt_number_tables.up.sql:33`

**Issue:** `formatReceiptNo()` รู้จัก 2 ค่า: `"BE4"` และ `"CE4"` โดย `default` branch ทำหน้าที่เป็น BE4 silent fallback แต่ DB ไม่มี CHECK constraint บังคับค่าที่ถูกต้อง ผู้ดูแลระบบที่ set `year_format = 'AD4'` ผ่าน Phase 4 UI จะได้รับ BE4 output โดยไม่มีข้อความ error ใด ๆ

**Fix:** เพิ่ม CHECK constraint บน column:
```sql
year_format  TEXT  NOT NULL DEFAULT 'BE4'
                 CHECK (year_format IN ('BE4', 'CE4')),
```

---

_Reviewed: 2026-06-25T16:42:12Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
