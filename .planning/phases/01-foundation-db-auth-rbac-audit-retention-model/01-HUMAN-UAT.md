---
status: partial
phase: 01-foundation-db-auth-rbac-audit-retention-model
source: [01-VERIFICATION.md]
started: 2026-06-24
updated: 2026-06-24
---

## Current Test

[awaiting human testing]

## Tests

### 1. Keycloak 26 ใช้ argon2id สำหรับ password hashing
expected: หลัง `docker compose up` (stack: Postgres + Keycloak + API) เปิด Keycloak Admin Console → Realm `donnarec` → Authentication → Policies → Password policy / Realm defaults แล้วยืนยันว่า password hashing algorithm = `argon2` (argon2id). หมายเหตุ: Keycloak 24+ ตั้ง argon2 เป็น default algorithm อยู่แล้ว ดังนั้น `realm-donnarec.json` ที่ไม่ได้ระบุ `passwordHashingProvider` ควรใช้ argon2id ตาม default — ข้อนี้เป็นการ "ยืนยัน" ไม่ใช่การแก้
result: [pending]

### 2. HTTPS/TLS ใน deployed environment (SC#5)
expected: ใน production/staging ที่ deploy จริง ต้องมี reverse proxy (เช่น nginx/traefik) terminate TLS และบังคับ HTTPS; การเชื่อม API ↔ Postgres ใช้ `sslmode=verify-full`. หมายเหตุ: local dev ใช้ HTTP + `sslmode=disable` ตาม design decision (ถูกต้องสำหรับ dev) และ VALIDATION.md §Manual-Only ระบุไว้ล่วงหน้าแล้วว่าเป็น manual/deploy-time verification
result: [pending]

## Summary

total: 2
passed: 0
issues: 0
pending: 2
skipped: 0
blocked: 0

## Gaps
