---
status: testing
phase: 04-receipt-pdf-email-delivery-outbox-worker
source: [04-VERIFICATION.md, 04-06-SUMMARY.md, 04-08-SUMMARY.md]
started: 2026-07-04T13:23:25Z
updated: 2026-07-04T13:23:25Z
---

## Current Test

number: 1
name: Screen 3b — Staff resend/download + delivery status + donor_language (plan 04-06 Task 4)
expected: |
  Maker สร้างรายการโดยเลือก donor_language=English → Checker อนุมัติ → outbox worker
  render+ส่งอีเมล → หน้า Donation Detail แสดง DeliveryStatusBadge + EmailDeliveryPanel,
  Checker/Admin เห็นปุ่ม Resend และกดแล้ว re-enqueue โดย "เลขที่ใบเสร็จไม่เปลี่ยน + ไม่ re-render",
  staff กด Download แล้วได้ PDF ตรงกับที่ freeze, และ Maker (ผู้สร้าง) เห็นปุ่ม Download แต่ "ไม่เห็นปุ่ม Resend" (SoD)
awaiting: user response

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
result: [pending]

### 2. Screen 6 — Admin Settings editor (plan 04-08 Task 3)
expected: |
  Admin เข้า /admin/settings (non-admin ต้องถูกกั้น) → 4 tabs (template HTML / รูปแบรนด์ /
  ข้อความ §6 + 1x/2x / รูปแบบเลข) → live HTML preview (sandboxed iframe, debounce 400ms,
  TH Sarabun, sample data ไม่ส่ง donor field) อัปเดตตามการแก้ → กดเรนเดอร์ PDF จริงได้ →
  อัปโหลดรูปแบรนด์ (magic-byte validated) → บันทึกทีเดียว (single PUT) แล้วค่าคงอยู่หลัง reload
prerequisites: |
  - เหมือนข้อ 1 (Keycloak confidential client + .env.local + admin-test password + stack ครบ)
  - ล็อกอินด้วย admin-test@donnarec.local (role admin)
result: [pending]

## Summary

total: 2
passed: 0
issues: 0
pending: 2
skipped: 0
blocked: 0

## Gaps
