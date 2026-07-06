# Phase 5: e-Donation Export, Reports & Admin Settings - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-06
**Phase:** 05-e-donation-export-reports-admin-settings
**Areas discussed:** e-Donation Export + PII, สถานะ "คีย์แล้ว" + Aging, รายงานสรุปบริจาค, Backup/Restore, ชะตากรรมไฟล์ export, field mapping, aging rule, สิทธิ์ดูรายงาน

---

## e-Donation Export + การเปิดเผยเลขบัตร ปชช.

| Option | Description | Selected |
|--------|-------------|----------|
| .xlsx (แนะนำ) | Excel จริงผ่าน excelize | |
| ทั้ง .xlsx และ CSV | รองรับทั้งสองรูปแบบ | ✓ |
| CSV อย่างเดียว | stdlib encoding/csv | |

**User's choice:** ทั้ง .xlsx และ CSV (D-62)

| Option | Description | Selected |
|--------|-------------|----------|
| Admin เท่านั้น (แนะนำ) | least-privilege สุด | |
| Checker + Admin | role ที่อนุมัติใบเสร็จอยู่แล้ว | ✓ |
| Role ใหม่เฉพาะ export | เพิ่ม Exporter/Accounting | |

**User's choice:** Checker + Admin (D-63)

| Option | Description | Selected |
|--------|-------------|----------|
| เต็ม 13 หลัก + audit ทุกครั้ง (แนะนำ) | plaintext + download-logged | ✓ |
| มีโหมด mask/เต็ม | default mask, checkbox รวมเต็ม | |

**User's choice:** เต็ม 13 หลัก + audit ทุกครั้ง (D-64)

| Option | Description | Selected |
|--------|-------------|----------|
| ค่าคงที่ 'เงินสด/โอน' (แนะนำ) | in-kind out of scope | ✓ |
| field ต่อรายการ | Maker เลือก | |
| คุณตัดสิน | ตามสเปก e-Donation | |

**User's choice:** ค่าคงที่ (D-65)

| Option | Description | Selected |
|--------|-------------|----------|
| issued ตามช่วงวันที่ + กรองสถานะคีย์ (แนะนำ) | filter, cancelled ไม่รวม | ✓ |
| issued ทั้งหมด ไม่มี filter | ดึงทั้งหมดคัดใน Excel | |

**User's choice:** issued ตามช่วงวันที่ + กรองสถานะคีย์ (D-66)

**Notes:** export ต้อง decrypt เลขบัตรลงไฟล์ → PDPA surface; ผูกกับ audited-reveal ของ Phase 3

---

## สถานะ "คีย์เข้า e-Donation แล้ว" + Aging

| Option | Description | Selected |
|--------|-------------|----------|
| เลือกหลายแถว → mark bulk + ต่อแถว (แนะนำ) | ติ๊กหลายแถวในหน้า aging | ✓ |
| ต่อ record เท่านั้น | toggle ต่อแถว | |
| export mark ให้อัตโนมัติ | เสี่ยง mark ทั้งที่ยังไม่คีย์ | |

**User's choice:** เลือกหลายแถว → mark bulk + ต่อแถว (D-67)

| Option | Description | Selected |
|--------|-------------|----------|
| 3 กลุ่ม: ยังไม่ถึงกำหนด/ใกล้ครบ/เกินกำหนด (แนะนำ) | มองเห็นความเสี่ยงทันที | ✓ |
| แค่ overdue vs ไม่ overdue | 2 กลุ่มง่าย | |
| นับถอยหลังเป็นตาย | จำนวนวันต่อแถว | |

**User's choice:** 3 กลุ่ม (D-68)

| Option | Description | Selected |
|--------|-------------|----------|
| เท่ากับสิทธิ์ export (Checker + Admin) (แนะนำ) | workflow เดียวกัน | ✓ |
| ทุก staff รวม Maker | ยืดหยุ่นกว่า | |

**User's choice:** Checker + Admin (D-69)

---

## รายงานสรุปการบริจาค (FR-32)

| Option | Description | Selected |
|--------|-------------|----------|
| ช่วงเวลา + แยกรายเดือน/วัน (แนะนำ) | ครอบ FR-32 ไม่ over-build | ✓ |
| แค่ยอดรวมช่วงเวลา | ง่ายสุด | |
| หลายมิติ (เวลา+สถานะ+Maker) | ยืดหยุ่นแต่งานเยอะ | |

**User's choice:** ช่วงเวลา + แยกรายเดือน/วัน (D-70)

| Option | Description | Selected |
|--------|-------------|----------|
| ตาราง + card สรุปยอด (แนะนำ) | TanStack Table + stat card | ✓ |
| ตาราง + กราฟ | เพิ่ม chart lib | |

**User's choice:** ตาราง + card สรุปยอด (D-70)

| Option | Description | Selected |
|--------|-------------|----------|
| export Excel/CSV ได้ (แนะนำ) | ยอดสรุปไม่มี PII | ✓ |
| ดูบนหน้าอย่างเดียว | | |

**User's choice:** export Excel/CSV ได้ (D-70)

| Option | Description | Selected |
|--------|-------------|----------|
| ทุก staff รวม Maker (แนะนำ) | ยอดสรุปไม่มี PII | ✓ |
| เฉพาะ Checker + Admin | | |

**User's choice:** ทุก staff รวม Maker (D-71)

---

## Backup / Restore (NFR-08)

| Option | Description | Selected |
|--------|-------------|----------|
| pg_dump ตามตารางใน compose (แนะนำ) | portable, ไม่ผูก cloud | ✓ |
| Script + host cron | คุมง่ายแต่ผูก host | |
| คุณตัดสิน | ตาม hosting | |

**User's choice:** pg_dump ตามตารางใน compose (D-72)

| Option | Description | Selected |
|--------|-------------|----------|
| DB + MinIO (slip + PDF freeze) (แนะนำ) | กู้ครบ | ✓ |
| DB อย่างเดียว | PDF/สลิปหาย | |

**User's choice:** DB + MinIO (D-72)

| Option | Description | Selected |
|--------|-------------|----------|
| Runbook + ทดสอบ restore จริง บันทึกหลักฐาน (แนะนำ) | ตรง SC 'ทำจริงสำเร็จ' | ✓ |
| เทสต์ restore อัตโนมัติใน CI | แข็งแต่งานเยอะ | |

**User's choice:** Runbook + ทดสอบ restore จริง บันทึกหลักฐาน (D-73)

---

## ชะตากรรมไฟล์ export ที่มี PII

| Option | Description | Selected |
|--------|-------------|----------|
| Stream download อย่างเดียว ไม่เก็บไฟล์ (แนะนำ) | ลด attack surface สุด | ✓ |
| เก็บ MinIO มี retention + re-download | สะดวกแต่ surface เพิ่ม | |

**User's choice:** Stream download อย่างเดียว (D-74)

---

## e-Donation field mapping เก็บที่ไหน

| Option | Description | Selected |
|--------|-------------|----------|
| Config-driven (ต่อยอด D-58 config store) (แนะนำ) | แก้ไม่ deploy, รับ stakeholder gate | ✓ |
| Hardcode ตาม best-guess | ต้อง deploy เมื่อสเปกเปลี่ยน | |

**User's choice:** Config-driven (D-75)

---

## Aging threshold + base date

| Option | Description | Selected |
|--------|-------------|----------|
| ≤ 3 วัน, อิงเดือนที่ issue (แนะนำ) | threshold config ได้ | ✓ |
| ≤ 5 วัน, อิงเดือนที่ issue | ช่วงเตือนกว้างขึ้น | |
| คุณตัดสิน threshold | planner เลือก default | |

**User's choice:** ≤ 3 วัน, อิงเดือนที่ issue (D-68)

---

## เมนู/สิทธิ์ดูรายงาน

| Option | Description | Selected |
|--------|-------------|----------|
| ทุก staff รวม Maker (แนะนำ) | ยอดสรุปไม่มี PII | ✓ |
| เฉพาะ Checker + Admin | | |

**User's choice:** ทุก staff รวม Maker (D-71)

---

## Claude's Discretion

- schema รายละเอียด keyed flag / export-audit / config keys
- migration number (000013+) และโครงสร้าง package Go (internal/export, internal/report, internal/backup)
- default ตัวเลข: aging threshold (≤3 วัน config ได้), backup retention, schedule cron
- query grouping รายงาน, CSV BOM/encoding
- กลไก MinIO backup (mc mirror vs API), รูปแบบ restore runbook

## Deferred Ideas

- เชื่อม API e-Donation ตรง (milestone ถัดไป — stakeholder gate 1 ม.ค. 2026)
- chart/กราฟในรายงาน
- cash type / 1x-2x เป็น field ต่อรายการ
- Flow B public form (Phase 6)
- role 'Exporter'/'Accounting' แยกเฉพาะ export
