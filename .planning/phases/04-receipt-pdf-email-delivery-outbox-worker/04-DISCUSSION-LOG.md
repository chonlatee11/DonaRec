# Phase 4: Receipt PDF + Email Delivery (Outbox Worker) - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-04
**Phase:** 4-receipt-pdf-email-delivery-outbox-worker
**Areas discussed:** ภาษาของ PDF/email, การเก็บไฟล์ PDF, Retry/resend/download, Config store + Admin UI, การ model สิทธิ 1x/2x

---

## ภาษาของ PDF/email (FR-23)

| Option | Description | Selected |
|--------|-------------|----------|
| เพิ่ม donor_language | column บน donation, Maker เลือกตอนสร้าง, freeze กับ snapshot; ต้อง migrate + เพิ่ม field ฟอร์ม Phase 3 | ✓ |
| Default ไทย + toggle ตอนส่ง | ไม่ persist, staff toggle ตอน download/resend | |
| You decide | ให้ planner เลือก | |

**User's choice:** เพิ่ม donor_language (D-55)
**Notes:** ตรง requirement FR-23 ที่สุด; ภาษาถูก freeze เป็นส่วนของ snapshot (แนว D-43)

---

## การเก็บไฟล์ PDF (FR-24, immutability)

| Option | Description | Selected |
|--------|-------------|----------|
| Freeze เก็บใน MinIO | render ครั้งเดียวตอน worker ทำงาน เก็บไฟล์; resend/download ใช้ไฟล์เดิม (immutable) | ✓ |
| Render on-demand | ไม่เก็บ, render ใหม่ทุกครั้ง — เสี่ยงเอกสารเปลี่ยนเมื่อ config เปลี่ยน | |
| You decide | ให้ planner/researcher ชั่งน้ำหนัก | |

**User's choice:** Freeze เก็บใน MinIO (D-56)
**Notes:** เอกสารภาษี ณ จุดเวลาต้อง immutable แม้ template/config เปลี่ยนภายหลัง (D-42/D-43)

---

## Retry / resend / ดาวน์โหลดเอง (FR-27/28, NFR-07)

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-retry + resend มือ | auto-retry มี backoff, เกินแล้ว 'failed' → staff เห็น + resend เอง; download PDF ได้เสมอ; resend ไม่ allocate เลขใหม่ | ✓ |
| Retry ไม่จำกัด | retry ต่อเนื่อง interval คงที่จนสำเร็จ | |
| You decide | ให้ planner กำหนดตัวเลข retry/backoff | |

**User's choice:** Auto-retry + resend มือ (D-57)
**Notes:** ต้องการ failure UX ชัดเจน ไม่วน infinite; staff ควบคุม resend ได้เอง

---

## Config store + Admin UI (FR-33/NFR-09)

| Option | Description | Selected |
|--------|-------------|----------|
| Lean: ค่าที่เปลี่ยนบ่อย | config store + seeded HTML template; Admin UI แก้เฉพาะ §6/1-2x, upload ภาพ, รูปแบบเลข; HTML layout ผ่าน dev | |
| Full: แก้ HTML ได้ด้วย | config store + Admin HTML template editor เต็มรูป | ✓ |
| You decide | ให้ planner ชั่งขอบเขต UI | |

**User's choice:** Full: แก้ HTML ได้ด้วย (D-58)
**Notes:** ยอมงานใหญ่ขึ้นเพื่อความยืดหยุ่น no-deploy. **Claude flag:** เพิ่ม security surface (template-injection/XSS/SSRF ใน headless Chromium) — CONTEXT บันทึกให้ downstream ออกแบบ sandbox (ปิด JS + network isolation, whitelist placeholder, sanitize upload, Admin-only + audit)

---

## การ model สิทธิลดหย่อน 1 เท่า / 2 เท่า (FR-24, compliance)

| Option | Description | Selected |
|--------|-------------|----------|
| Global config ระดับ รพ. | ค่าเดียวใน config store, ใบเสร็จทุกใบใช้ข้อความ §6 เดียวกัน | ✓ |
| ต่อใบเสร็จ (เลือกได้) | เพิ่ม field บน donation ให้ staff เลือก 1x/2x ต่อรายการ | |
| You decide | default global ก่อนจน stakeholder ยืนยัน | |

**User's choice:** Global config ระดับ รพ. (D-59)
**Notes:** โรงพยาบาลมีสถานะลดหย่อนเดียว; ลดความซับซ้อน schema/UI/validation. เพิ่ม field ต่อ donation ภายหลังได้ถ้าจำเป็น

---

## Claude's Discretion

- Worker trigger model (polling vs LISTEN/NOTIFY vs asynq/River) — planner เลือก; DB-backed poll เพียงพอ
- chromedp vs rod — ยืนยันด้วย spike (research flag) ก่อน lock
- จำนวน retry / backoff schedule / max attempts — planner กำหนด (D-57 กำหนดแค่รูปแบบ)
- Schema รายละเอียด (email_delivery, config table, donor_language, PDF ref, MinIO bucket) + โครงสร้าง package Go + migration 000008+
- Email provider จริง (SES vs Postmark) — stakeholder gate; build ต่อ EmailSender interface + dev capture (D-60)

## Deferred Ideas

- Email provider จริง + deliverability (SPF/DKIM/DMARC) — stakeholder gate (D-60)
- 1x/2x ต่อ donation — MVP ใช้ global config (D-59)
- e-Donation export + reports + backup/restore — Phase 5
- Flow B public form + acknowledgement email — Phase 6
- PKI digital signature — เกินขอบเขต MVP
