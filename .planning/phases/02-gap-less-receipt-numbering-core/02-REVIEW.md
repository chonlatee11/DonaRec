---
phase: 02-gap-less-receipt-numbering-core
reviewed: 2026-06-27T00:00:00Z
depth: standard
files_reviewed: 12
files_reviewed_list:
  - donnarec-api/migrations/000004_receipt_number_tables.up.sql
  - donnarec-api/migrations/000004_receipt_number_tables.down.sql
  - donnarec-api/internal/db/queries/receiptno.sql
  - donnarec-api/internal/db/generated/receiptno.sql.go
  - donnarec-api/internal/receiptno/fiscal_year.go
  - donnarec-api/internal/receiptno/format.go
  - donnarec-api/internal/receiptno/fiscalyear_test.go
  - donnarec-api/internal/receiptno/format_test.go
  - donnarec-api/internal/receiptno/allocator.go
  - donnarec-api/internal/receiptno/allocator_test.go
  - donnarec-api/internal/receiptno/allocator_concurrency_test.go
  - donnarec-api/internal/receiptno/allocator_rollback_test.go
findings:
  critical: 0
  warning: 3
  info: 4
  total: 7
status: issues_found
---

# Phase 02: Code Review Report (รอบที่ 2)

**Reviewed:** 2026-06-27T00:00:00Z
**Depth:** standard
**Files Reviewed:** 12
**Status:** issues_found

## Summary

นี่คือหัวใจของ gap-less receipt numbering ซึ่งเป็นข้อกำหนดความถูกต้องที่ยากที่สุดของโปรเจกต์ การรีวิวรอบนี้เป็นการตรวจซ้ำหลังจาก commit `1eab44a` ได้แก้ findings รอบก่อนครบแล้ว (ยืนยันว่าแก้จริง: `bangkokLoc` ใช้ `sync.Once` แล้ว, index ซ้ำถูกลบ, comment ที่ผิดถูกแก้, `year_format` มี CHECK constraint แล้ว)

**core ของ concurrency ถูกต้องจริง** ผมไล่ trace ทุก race ที่สร้างได้:
- การออกเลขแบบ sequential ภายใต้ READ COMMITTED serialize ถูกต้อง (ถือ lock จนถึง commit, หลัง block แล้ว re-read ค่าที่ committed)
- การ first-allocation พร้อมกันของปีงบใหม่ปลอดภัย: `InitCounterRow` (`ON CONFLICT DO NOTHING`) ทำให้ session ที่แพ้ block บน insert ที่ยังไม่ commit ของผู้ชนะ แล้ว re-lock row ที่มีอยู่จริง อ่านค่าที่ committed → ไม่มี duplicate ไม่มี gap
- การ rollback ของ `Allocate` คืนเลขถูกต้อง (counter increment + ledger insert ถูก undo พร้อมกัน) และเลขที่คืนถูกใช้ซ้ำโดย commit ถัดไป — พิสูจน์จุดต่างของ counter-table vs SEQUENCE
- SQL parameterized ครบผ่าน sqlc, ไม่มี string concatenation, grant แบบ least-privilege พร้อม `REVOKE UPDATE, DELETE` บน ledger, และ `UNIQUE(fiscal_year, running_no)` backstop ผ่าน test

logic fiscal year (Oct→+544, Jan–Sep→+543, normalise เป็น Asia/Bangkok ก่อนอ่านเดือน) ถูกต้องและมี test ครอบคลุม รวมถึงการข้าม boundary UTC→BKK; guard `sync.Once` สำหรับ tz เหมาะสม

**ไม่พบ BLOCKER** findings ด้านล่างเป็นเรื่อง robustness และ defense-in-depth ซึ่งสำคัญกว่าปกติในเฟสนี้เพราะ ledger เป็น **immutable** (`REVOKE DELETE`) — row ผิดที่เขียนลงไปจะลบไม่ได้ถาวร

---

## Warnings

### WR-01: `Allocate` ไม่ได้บังคับ/ระบุ isolation level ที่ต้องการ

**File:** `donnarec-api/internal/receiptno/allocator.go:83-110`

**Issue:** การรับประกัน gap-less + serialization แบบ non-blocking ขึ้นกับว่า tx ของ caller รันที่ **READ COMMITTED** ตอนนี้ helper ในโปรเจกต์ (`internal/db/helpers.go:26`) ใช้ `pool.Begin(ctx)` ซึ่ง default เป็น READ COMMITTED จึงปลอดภัยอยู่ แต่ `Allocate` รับ `pgx.Tx` จาก caller ใด ๆ และถูกระบุว่าเป็น single allocation path สำหรับ Phase 3+ หาก caller ในอนาคตเปิด tx ด้วย `REPEATABLE READ` หรือ `SERIALIZABLE` การ re-lock หลัง block (`LockCounterForUpdate`/`IncrementCounter`) จะ throw `40001 could not serialize access due to concurrent update` แทนที่จะ re-read — ทำให้ approval ที่ทำพร้อมกันกลายเป็น hard failure (allocator ไม่ retry ตาม D-36) ทั้ง signature และ doc comment ไม่ได้ระบุข้อกำหนด isolation นี้ไว้

**Fix:** ระบุข้อกำหนดใน doc comment ของ `Allocate` ให้ชัด และพิจารณา assert แบบเบา ๆ:
```go
// tx ที่ส่งเข้ามา MUST เป็น READ COMMITTED. ภายใต้ REPEATABLE READ / SERIALIZABLE
// การ re-lock FOR UPDATE หลัง concurrent commit จะ raise SQLSTATE 40001 และ
// Allocate ไม่ retry (D-36).
```

---

### WR-02: `formatted` ประกอบจาก `prefix`/`separator` ที่ admin ตั้งค่าได้ โดยไม่ validate อักขระ แล้วถูก render เป็น PDF ผ่าน headless Chromium HTML

**File:** `donnarec-api/internal/receiptno/format.go:40-58`, `donnarec-api/migrations/000004_receipt_number_tables.up.sql:30-35`

**Issue:** `prefix` และ `separator` เป็นค่า config `TEXT` แบบอิสระที่ไหลเข้า `formatted` โดยไม่ escape แล้วถูก freeze ลง ledger (immutable) และ (ตาม CLAUDE.md) ถูก render ผ่าน pipeline HTML→Chromium เพื่อสร้าง PDF ค่า config เช่น `prefix = "<img src=x onerror=...>"` จะกลายเป็น stored markup/script-injection payload ในทุกใบเสร็จ หาก renderer ปลายทางไม่ escape เนื่องจาก ledger เป็น append-only `formatted` ที่ปนเปื้อนแก้ด้วย UPDATE/DELETE ไม่ได้ ปัจจุบัน DB constrain เฉพาะ `year_format` (CHECK IN) ไม่ได้ constrain `prefix`/`separator`

**Fix:** defense-in-depth: จำกัดอักขระที่ขอบ config เช่น `CHECK (prefix ~ '^[A-Za-z0-9 _./-]*$')` (และทำนองเดียวกันกับ `separator`) และ/หรือรับประกันว่ามี HTML-escaping ตอน render ใน Phase 4 อย่าพึ่งพา renderer อย่างเดียวเพราะ snapshot เป็น immutable

---

### WR-03: `Allocate` ไม่ปฏิเสธ `issueDate` ที่เป็น zero / ค่าผิดปกติ และผลลัพธ์ที่ผิดเป็นถาวร

**File:** `donnarec-api/internal/receiptno/allocator.go:83-85`, `donnarec-api/internal/receiptno/fiscal_year.go:71-90`

**Issue:** `fiscalYear` รับ `issueDate` จาก caller ตรง ๆ โดยไม่มี sanity check ค่า `time.Time{}` (ปี ค.ศ. 1) จะให้ fiscal year `544`/`545`; timestamp ขยะใด ๆ จาก bug ฝั่ง Phase 3 จะ allocate เลขใต้ปีงบที่ไร้สาระเงียบ ๆ สร้าง counter row และ ledger row ที่ — เพราะ `REVOKE UPDATE, DELETE` — ลบไม่ได้ตลอดไป สัญญาบอกว่า caller ส่ง approval timestamp มาก็จริง แต่ guard ราคาถูกป้องกันความผิดพลาดถาวรที่กู้คืนไม่ได้ในตารางที่สำคัญที่สุดของโปรเจกต์

**Fix:** ปฏิเสธ zero value และวันที่เก่า/อนาคตที่ผิดปกติก่อน allocate:
```go
if issueDate.IsZero() {
    return AllocatedReceipt{}, fmt.Errorf("allocate: issueDate must not be zero")
}
```
และอาจ bound ช่วงปีที่สมเหตุสมผล (เช่น ปี >= 2020) เพื่อจับ input ที่เสียหาย

---

## Info

### IN-01: `GetReceiptNumberConfig` ใช้ `LIMIT 1` โดยไม่มี `ORDER BY`

**File:** `donnarec-api/internal/db/queries/receiptno.sql:48-50`

**Issue:** ความ deterministic พึ่งพา invariant `single_row CHECK (id = true)` ที่รับประกันว่ามีแถวเดียว ตอนนี้จริง แต่ `LIMIT 1` ไม่มี `ORDER BY` จะ non-deterministic ถ้า invariant แถวเดียวถูกผ่อนในอนาคต ความเสี่ยงต่ำ — บันทึกไว้เพื่อความทนทาน

**Fix:** คงไว้ได้ (ยอมรับได้เพราะมี CHECK) หรือเพิ่ม `WHERE id = true` ให้ intent ชัดเจนขึ้น

---

### IN-02: lock แบบ explicit ก่อน `UPDATE` ที่ lock อยู่แล้ว (ซ้ำซ้อนเล็กน้อย)

**File:** `donnarec-api/internal/receiptno/allocator.go:94-117`

**Issue:** `IncrementCounter` (`UPDATE … RETURNING`) ก็ถือ row lock อยู่แล้ว ดังนั้น `LockCounterForUpdate` ที่นำหน้าจึงซ้ำซ้อนบางส่วน — มีไว้เพื่อ detect กรณี row หาย (ปีงบใหม่) ก่อนทำ UPDATE ไม่ใช่ defect; การ lock แบบ explicit ทำให้ branch ปีงบใหม่อ่านง่าย บันทึกเป็นโอกาส simplify เท่านั้น (`IncrementCounter` → ถ้า `ErrNoRows` → `InitCounterRow` → `IncrementCounter`)

**Fix:** optional ไม่ต้องแก้ — รูปแบบปัจจุบันถูกต้องและชัดเจน

---

### IN-03: down migration ลบข้อมูล ledger ที่มีนัยทางกฎหมายแบบกู้คืนไม่ได้

**File:** `donnarec-api/migrations/000004_receipt_number_tables.down.sql:11-13`

**Issue:** down migration `DROP TABLE` receipt_numbers ledger (เอกสารภาษี/audit) comment DANGER เขียนดี แต่ไม่มีกลไกเชิงเทคนิคป้องกันการรันกับ DB ที่ไม่ใช่ dev

**Fix:** ยอมรับได้สำหรับตอนนี้เพราะมี warning ชัด ถ้าทำได้ ควร guard ที่ระดับ migration-runner ไม่ให้ production apply down step นี้

---

### IN-04: comment ลำดับการ drop ใน down migration อ้าง FK-safety ที่ยังไม่มีจริง

**File:** `donnarec-api/migrations/000004_receipt_number_tables.down.sql:8-9`

**Issue:** comment อธิบายลำดับ drop แบบ "child → parent (foreign key safe)" แต่ migration นี้ยังไม่มี FK constraint ระหว่างสามตารางเลย (FK ของ `receipts` ใน Phase 3 เป็นของอนาคต) comment จึงเป็น aspirational และอาจทำให้เข้าใจผิด

**Fix:** แก้ถ้อยคำให้ระบุว่า FK เป็นเรื่องอนาคต หรือตัดข้ออ้าง FK-safety ออกจนกว่า Phase 3 จะเพิ่ม constraint

---

_Reviewed: 2026-06-27T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
