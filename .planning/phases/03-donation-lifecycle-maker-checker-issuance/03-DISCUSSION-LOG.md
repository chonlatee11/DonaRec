# Phase 3: Donation Lifecycle & Maker-Checker Issuance - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-27
**Phase:** 3-donation-lifecycle-maker-checker-issuance
**Areas discussed:** Donor model, Approval/rejection workflow, PII reveal policy, Receipt cancellation, Slip in Flow A

---

## Donor model

| Option | Description | Selected |
|--------|-------------|----------|
| Snapshot-only | แต่ละ donation เก็บ donor fields ของตัวเอง ไม่มี donor table | ✓ |
| Donor master + dedup (blind index) | donor table + lookup/reuse ด้วยเลขผ่าน blind index | |
| คุยก่อน (อธิบายเพิ่ม) | ขอ trade-off ก่อนตัดสิน — ผลต่อ e-Donation export/reporting | (intermediate) |

**User's choice:** Snapshot-only
**Notes:** ผู้ใช้ขอให้อธิบาย trade-off ต่อ e-Donation export (Phase 5) และ reporting ก่อน. สรุป: export เป็น per-donation-record และ report เป็น aggregate ตามช่วงเวลา/ยอดรวม → ทั้งคู่ทำงานบน snapshot ได้ครบ ไม่ต้อง dedup; กฎหมายบังคับ freeze donor identity อยู่แล้ว; dedup/auto-fill/rollup เพิ่มภายหลังได้โดยไม่ migrate snapshot. หลังฟังคำอธิบายผู้ใช้เลือก snapshot-only (แนะนำ)

---

## Donor required fields

| Option | Description | Selected |
|--------|-------------|----------|
| บังคับเสมอ | ไม่มีเลขภาษี/ปชช. = ออกใบไม่ได้ | ✓ |
| ไม่บังคับ (optional) | ออกใบได้แม้ไม่มีเลข | |
| คุณตัดสินใจให้ | ตามแนวปฏิบัติ | |

**User's choice:** บังคับเสมอ
**Notes:** ต้องมีเลขถึงจะคีย์เข้า e-Donation RD ได้ → enforce ที่ขอบ API + DB NOT NULL บน ciphertext

---

## Approval / rejection workflow

| Option | Description | Selected |
|--------|-------------|----------|
| แยก 2 action: ตีกลับแก้ (loop) + ปฏิเสธถาวร | ตีกลับ→draft resubmit ได้; ปฏิเสธ→rejected terminal | ✓ |
| ตีกลับอย่างเดียว → draft (loop) | มี action เดียว, ไม่มี rejected | |
| ตีกลับ = rejected ถาวร | terminal, maker สร้างใหม่ | |

**User's choice:** แยก 2 action (ตีกลับเพื่อแก้ loop + ปฏิเสธถาวร) — ทั้งคู่บังคับเหตุผล
**Notes:** ตรงกับ lifecycle ROADMAP ที่มี rejected terminal อยู่แล้ว และรองรับการแก้รอบเล็กน้อยแบบ loop

---

## PII reveal policy

| Option | Description | Selected |
|--------|-------------|----------|
| Checker + Admin | Checker reveal เพื่อตรวจเทียบสลิป, Admin support | ✓ |
| Admin เท่านั้น | Checker เห็น mask | |
| Maker + Checker + Admin | ทุก back-office role reveal ได้ | |

**User's choice:** Checker + Admin (audited)
**Notes:** ตรงกับ Phase 1 D-10 (`pii.CanRevealFull`) พอดี → reuse ของเดิม ไม่สร้างใหม่. Mask format (last-4) และ reveal flow (gate→decrypt→audit) ถูก locked ใน `pii/mask.go` แล้ว ไม่ถามซ้ำ. Maker เห็นเลขเต็มเฉพาะตอนแก้ draft ของตัวเอง

---

## Receipt cancellation

| Option | Description | Selected |
|--------|-------------|----------|
| Checker + Admin | สอดคล้องอำนาจอนุมัติ | ✓ |
| Admin เท่านั้น | เข้มงวดสุด | |
| คุณตัดสินใจ | ตาม SoD/least-privilege | |

**User's choice:** Checker + Admin (บังคับเหตุผล + audit)
**Notes:** Maker ยกเลิกเองไม่ได้. ยกเลิก = set "ยกเลิก"/cancelled, เก็บเลขเดิม ไม่ลบ ไม่เกิด gap (FR-19)

---

## Slip in Flow A

| Option | Description | Selected |
|--------|-------------|----------|
| แนบได้ (optional) + สร้าง storage ตอนนี้ | MinIO + magic-byte ใน Phase 3, Phase 6 reuse | ✓ |
| ไม่มีสลิปใน Flow A (เลื่อน storage ไป Phase 6) | โฟกัส lifecycle core | |
| บังคับแนบสลิปทุกราย | ทุก donation ต้องมีสลิป | |

**User's choice:** แนบได้ (optional) + สร้าง object storage ใน Phase 3
**Notes:** รองรับเงินสด/ไม่มีสลิป (optional). ตรง ROADMAP SC#1 "view any attached slip"; Phase 6 (Flow B) reuse seam. เก็บไฟล์ใน object storage + reference ใน DB (ไม่เก็บ BLOB)

---

## Claude's Discretion

- schema donation/receipt entity (column, FK→ledger, index search), migration 000005+, status constraint, SoD CHECK
- Go package structure (`internal/donation/`, `internal/storage/`), object storage client (`minio-go`), config endpoint/bucket
- outbox table shape + enqueue mechanism (worker = Phase 4)
- validation donor fields อื่น (email/address format)

## Deferred Ideas

- Donor master + dedup + blind index + per-donor rollup/auto-fill → future (no migrate)
- Flow B public form + slip upload + web pending-review queue (FR-01..06, FR-08) → Phase 6
- PDF/email/outbox worker (FR-20..28, NFR-07) → Phase 4
- ข้อความลดหย่อน 1 เท่า/2 เท่า + template/config UI (FR-24, FR-33, NFR-09) → Phase 4
- e-Donation export + reports + "คีย์แล้ว" flag (FR-30/31/32) → Phase 5
