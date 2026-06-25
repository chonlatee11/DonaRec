# Phase 2: Gap-less Receipt Numbering Core (★) - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-25
**Phase:** 2-gap-less-receipt-numbering-core
**Areas discussed:** รูปแบบ & ค่า default ของเลข, ที่เก็บ config รูปแบบเลข, Seam ↔ Phase 3 (tx + return), ที่อยู่ของ backstop/ledger, Freeze เลขที่แสดง, Error/retry contract, issueDate contract + timezone, Overflow padding

---

## รูปแบบ & ค่า default ของเลข

| Option | Description | Selected |
|--------|-------------|----------|
| `2569/000123` | ปีงบ 4 หลัก + `/` + เลขรัน 6 หลัก zero-pad — ตรง ROADMAP SC#1 | ✓ |
| `2569/0123` | zero-pad 4 หลัก | |
| `DN-2569-000123` | มี prefix ตัวอักษรโรงพยาบาล คั่นด้วย `-` | |

| Option (ปีในเลข) | Description | Selected |
|--------|-------------|----------|
| พ.ศ. เต็ม 4 หลัก (2569) | ตรง SC#1 | ✓ |
| พ.ศ. 2 หลักท้าย (69) | สั้นลง แต่กำกวมเอกสารภาษี | |

**User's choice:** `2569/000123`, ปี พ.ศ. เต็ม 4 หลัก
**Notes:** ทุกองค์ประกอบยังตั้งค่าได้ — นี่คือค่า default เท่านั้น

---

## ที่เก็บ config รูปแบบเลข (ในเฟสนี้)

| Option | Description | Selected |
|--------|-------------|----------|
| DB settings table ตั้งแต่ตอนนี้ | no-deploy ตั้งแต่แรก, Phase 4 แค่ต่อ UI | ✓ |
| env/app config ก่อน → ย้าย DB Phase 4 | ต้อง deploy ถึงจะเปลี่ยน, ค่อย migrate ทีหลัง | |
| Claude discretion | ให้ researcher/planner ชั่ง | |

**User's choice:** DB settings table ตั้งแต่ Phase 2
**Notes:** schema/seam วางที่เฟสนี้, Phase 4 ต่อ UI เข้ามาแก้ค่าใน table เดิม

---

## Seam ↔ Phase 3 (transaction management)

| Option | Description | Selected |
|--------|-------------|----------|
| รับ pgx.Tx จากภายนอก (caller-managed) | Phase 3 ห่อ allocate+issue+audit+enqueue ใน commit เดียว | ✓ |
| allocator เปิด tx เอง (self-contained) | เสี่ยงเลขขาดถ้า issue rollback | |

| Option (return) | Description | Selected |
|--------|-------------|----------|
| คืนทั้ง fiscal_year, running_no, formatted | raw ไว้ query/sort + formatted พร้อมแสดง | ✓ |
| คืนเฉพาะ string ที่ format แล้ว | Phase 3 ต้อง parse เองถ้าต้องการ raw | |

**User's choice:** caller-managed pgx.Tx; คืน struct เต็ม
**Notes:** ตรง CLAUDE.md "ออกเลขในจังหวะ commit เดียวกับ issue"

---

## ที่อยู่ของ backstop UNIQUE + ledger

| Option | Description | Selected |
|--------|-------------|----------|
| Ledger แยก (standalone) — Phase 3 อ้างอิง | `receipt_numbers` + UNIQUE; concurrency test ยิงจริงตอนนี้ | ✓ |
| Counter-only — backstop ไป Phase 3 | UNIQUE ยังไม่พิสูจน์ในเฟสนี้ (อ่อนต่อ SC#4) | |
| Ledger = receipts minimal, Phase 3 ALTER | coupling Phase 2 กับ entity design Phase 3 ก่อนเวลา | |

**User's choice:** Ledger standalone, Phase 3 อ้างอิง/FK
**Notes:** allocation แยกอิสระจาก entity บริจาค

---

## Freeze เลขที่แสดง (compliance)

| Option | Description | Selected |
|--------|-------------|----------|
| เก็บ formatted string ตอน allocate (ตรึง) | ledger เก็บ snapshot; config เปลี่ยนไม่กระทบใบเก่า | ✓ |
| เก็บ raw อย่างเดียว format ตอนอ่าน | config เปลี่ยน = ใบเก่าเปลี่ยน (เสี่ยง compliance) | |

**User's choice:** เก็บ formatted snapshot ตอน allocate
**Notes:** เลขที่ออกแล้วต้อง immutable ตามหลัก audit/ภาษี

---

## Error/retry contract ของ allocate

| Option | Description | Selected |
|--------|-------------|----------|
| Bubble ขึ้น caller ไม่ retry ใน allocator | rollback ลบ ledger row, gap-less ปลอดภัย, ไม่ถือ lock นาน | ✓ |
| Retry ภายใน allocator | ซับซ้อน, ถือ tx นาน, serialize approver | |

**User's choice:** Bubble error ขึ้น caller, ไม่ retry ภายใน
**Notes:** —

---

## issueDate input contract + timezone

| Option | Description | Selected |
|--------|-------------|----------|
| caller ส่ง time.Time, fiscalYear normalize Asia/Bangkok | ทดสอบ boundary ได้ + ตรงเวลา issue | ✓ |
| allocator เรียก now() เอง | ทดสอบ boundary ยาก, แยกเวลา issue/อนุมัติไม่ได้ | |

**User's choice:** caller ส่ง time.Time; fiscalYear normalize Asia/Bangkok เสมอ
**Notes:** —

---

## Overflow เลขเกิน padding

| Option | Description | Selected |
|--------|-------------|----------|
| ขยายตามธรรมชาติ (pad = min width) | 1000000 แสดงเต็ม ไม่ error ไม่บล็อกการออกเลข | ✓ |
| Error เมื่อเกิน width | กันรูปแบบผิดแต่บล็อกการออกใบ | |

**User's choice:** padding = min width, เลขขยายตามธรรมชาติ
**Notes:** volume รพ. ต่ำมาก แทบไม่เกิด แต่ correctness มาก่อน

---

## Claude's Discretion

- กลไก lock ที่แน่นอน (`SELECT … FOR UPDATE` + UPDATE vs `INSERT … ON CONFLICT … RETURNING`) — research flag ของ ROADMAP
- schema รายละเอียด counter/ledger/settings table
- ค่า config seed เริ่มต้น, ชื่อ package ฝั่ง Go
- จำนวน N / รูปแบบ rollback scenario ใน concurrency test (assert zero gap + zero dupe + UNIQUE holds)

## Deferred Ideas

- UI แก้ config รูปแบบเลข → Phase 4 (FR-33/NFR-09)
- receipt entity เต็ม + maker-checker issuance tx → Phase 3
- FK ledger → receipts → Phase 3 (D-38)
