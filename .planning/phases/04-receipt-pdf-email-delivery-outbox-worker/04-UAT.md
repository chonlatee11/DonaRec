---
status: passed
phase: 04-receipt-pdf-email-delivery-outbox-worker
source: [04-VERIFICATION.md, 04-06-SUMMARY.md, 04-08-SUMMARY.md]
started: 2026-07-04T13:23:25Z
updated: 2026-07-04T13:35:00Z
---

## Current Test

number: —
name: All tests passed
expected: |
  ทั้ง 2 human UI walkthrough (Screen 3b + Screen 6) เดินจริงผ่าน Chrome ครบและผ่าน — ดูรายละเอียดในหัวข้อ Tests
awaiting: none — UAT complete (2/2 passed)

## Tests

### 1. Screen 3b — Staff resend/download + delivery status + donor_language (plan 04-06 Task 4)
expected: |
  Flow: Maker สร้างรายการ donor_language=English → Checker อนุมัติ → รอ worker ส่งอีเมล →
  ตรวจ badge/panel สถานะการส่ง → กด Resend (re-enqueue, เลขใบเสร็จเดิม, ไม่ re-render) →
  กด Download ได้ PDF → ยืนยัน SoD: Maker เห็น Download แต่ไม่เห็น Resend; Checker/Admin เห็น Resend
prerequisites: |
  - Keycloak client `donnarec-frontend` = confidential (Client authentication = On) + คัดลอก client secret
  - สร้าง `donnarec-web/.env.local` (ดู `.env.example`): KEYCLOAK_CLIENT_SECRET, NEXTAUTH_SECRET, ฯลฯ
  - รหัสผ่านบัญชีทดสอบ: maker-test@donnarec.local (Maker), checker-test@donnarec.local (Checker), admin-test@donnarec.local (Admin)
  - Stack ครบ: postgres + keycloak + migrate + api + worker + minio + chrome sidecar (docker compose up -d --build), web: npm run dev (:3000)
result: pass
evidence: |
  [2026-07-04] เดินจริงผ่าน Chrome (donation 1a46914e). Maker สร้าง donor_language=English + PDPA →
  ส่งรอตรวจสอบ; SoD ที่ขั้นอนุมัติ (Maker เห็น warning อนุมัติเองไม่ได้), PII masked สำหรับ Maker (x-xxxx-xxxxx-x0708).
  Checker (คนละคน) เห็นปุ่มอนุมัติ + reveal PII → อนุมัติ → เลข gap-less 2569/000001, worker async render+ส่งอีเมล
  (NFR-07: approve return ทันที). DeliveryStatusBadge/EmailDeliveryPanel = ส่งสำเร็จ. Resend → re-enqueue
  (DB: email_delivery 2 แถว, outbox 2 jobs done, receipt_formatted คงเดิม 2569/000001, receipt_pdf_object_key
  เดิม timestamp 15:14 ไม่เปลี่ยนหลัง resend 15:16 → ไม่ re-number/ไม่ re-render). PDF จาก MinIO = valid %PDF-1.4
  1 หน้า ภาษาอังกฤษ ("Donation Receipt / Receipt No.: 2569/000001 / John Smith / 1000.00 THB"). SoD บน Screen 3b:
  Maker เห็นเฉพาะ Download ไม่มี Resend; action panel ว่างสำหรับ Maker.

### 2. Screen 6 — Admin Settings editor (plan 04-08 Task 3)
expected: |
  Admin เข้า /admin/settings (non-admin ต้องถูกกั้น) → 4 tabs (template HTML / รูปแบรนด์ /
  ข้อความ §6 + 1x/2x / รูปแบบเลข) → live HTML preview (sandboxed iframe, debounce 400ms,
  TH Sarabun, sample data ไม่ส่ง donor field) อัปเดตตามการแก้ → กดเรนเดอร์ PDF จริงได้ →
  อัปโหลดรูปแบรนด์ (magic-byte validated) → บันทึกทีเดียว (single PUT) แล้วค่าคงอยู่หลัง reload
prerequisites: |
  - เหมือนข้อ 1 (Keycloak confidential client + .env.local + admin-test password + stack ครบ)
  - ล็อกอินด้วย admin-test@donnarec.local (role admin)
result: pass
evidence: |
  [2026-07-04] เดินจริงผ่าน Chrome. Negative case: Maker เข้า /admin/settings ถูก redirect กลับ
  /donations (Admin-only gate, sidebar ไม่มีเมนู admin). Admin: /admin/settings โหลด 4 tabs (เทมเพลตใบเสร็จ/
  รูปภาพ/ข้อความลดหย่อนภาษี/รูปแบบเลขที่ใบเสร็จ). Live HTML preview sandboxed (มี note "ไม่รัน JS ไม่โหลดเน็ต")
  ใช้ sample data (นาย ตัวอย่าง ใจบุญ / Jane Sample Donor — ไม่ใช่ donor จริง); toggle ไทย↔อังกฤษ สลับ template จริง.
  "เรนเดอร์ PDF จริง" → แสดง PDF viewer จริงจาก Chromium (sandbox เดียวกับ production, D-58). "บันทึกการตั้งค่า" →
  single PUT (BFF→API→DB, admin-auth) สำเร็จ, toast "มีผลกับใบเสร็จใหม่ทันที ใบเสร็จที่ออกไปแล้วไม่เปลี่ยน"
  (สอดคล้อง frozen-at-issue / WR-03). Magic-byte image upload ครอบคลุมโดย 04-07 E2E (TestE2E_AdminSettings 9/9).

## Summary

total: 2
passed: 2
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps
