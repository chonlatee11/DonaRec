---
status: diagnosed
phase: 01-foundation-db-auth-rbac-audit-retention-model
source: [01-01-SUMMARY.md, 01-02-SUMMARY.md, 01-03-SUMMARY.md, 01-04-SUMMARY.md]
started: 2026-06-25T11:38:09Z
updated: 2026-06-25T12:12:00Z
---

## Current Test

[testing complete — 6/6 pass; 2 infra gaps diagnosed (live), รอ fix plan]

## Tests

### 1. Cold Start Smoke Test
expected: หยุด stack + ล้าง volume (`cd donnarec-api && docker compose down -v`) แล้ว `docker compose up -d --wait` boot จากศูนย์ — postgres/keycloak/api ขึ้น healthy ครบ, migration รันสำเร็จ, Keycloak import realm donnarec สำเร็จ, `curl http://localhost:8000/healthz` → `{"status":"ok"}`
result: pass

### 2. Automated Test Suite GREEN
expected: รัน `cd donnarec-api && go test ./... -race -count=1` (ต้องมี Docker สำหรับ testcontainers) — ทุก suite ผ่าน: auth/RBAC, users handler, DB migration roundtrip, audit (immutability + hash-chain + 50-goroutine concurrent), crypto (AES-256-GCM + envelope + blind index), pii (mask + reveal), retention (legal-hold guard) — ไม่มี FAIL/race
result: pass

### 3. RBAC — Admin-only User Creation
expected: เฉพาะ token ที่มี role `admin` เท่านั้นที่ POST /api/admin/users ได้ (201); token `maker` หรือ `checker` → 403; ไม่มี token → 401; body ไม่ครบ → 400/422. ตรวจได้จาก TestCreateUserRBAC (suite test 2) หรือยิง API จริงด้วย token จาก Keycloak
result: pass
verified: "[2026-06-25] ยิง live API จริงผ่าน Bruno collection (Bruno/DonaRec/) — bru CLI 11/11 requests, 17/17 assertions ผ่าน: admin→201, maker→403, checker→403, no-token→401, bad-json→400, missing-fields→422; audit_log บันทึก action=user.create ครบ. RBAC logic ถูกต้อง. หมายเหตุ: ต้อง workaround 2 จุดก่อน (ดู Gaps) — รัน migration เอง + ส่ง Host:keycloak:8080 เพื่อให้ iss ตรง"

### 4. Audit Log Immutability + Hash-Chain
expected: audit_log เป็น append-only — DB role `donnarec_app` ทำ UPDATE/DELETE ไม่ได้ (permission denied) แต่ SELECT/INSERT ได้; แต่ละแถวมี SHA-256 hash-chain ที่ถ้าแก้แถวกลางทาง VerifyChain จะตรวจจับได้ (คืน brokenID); insert พร้อมกัน 50 goroutine ไม่มี prev_hash ซ้ำ. ตรวจได้จาก TestAuditImmutability + TestHashChainVerification + TestConcurrentAuditInserts
result: pass

### 5. PII Masking + Role-Gated Reveal
expected: national ID ถูก mask เหลือเห็น 4 ตัวท้าย (รูปแบบ x-xxxx-xxxxx-xNNNN); role Admin/Checker เปิดเผยเต็มได้ (CanRevealFull=true), Maker เปิดไม่ได้ (false); national/tax ID เข้ารหัส AES-256-GCM แบบ envelope (DEK wrap ด้วย KEK), ciphertext ถูกแก้ → decrypt ล้ม (ตรวจ tamper). ตรวจได้จาก TestMaskNationalID + TestCanRevealFull + TestEnvelopeRoundTrip
result: pass

### 6. Retention Legal-Hold Guard
expected: record ที่ `legal_hold=true` ถูกบล็อกการ hard-delete ทั้งสองชั้น — app-level GuardHardDelete คืน error `retention.legal_hold_delete_blocked` และ DB trigger `prevent_legal_hold_delete` raise exception; record ที่ legal_hold=false ลบได้ปกติ; soft-delete (is_active=false) ไม่ถูกบล็อก. ตรวจได้จาก TestLegalHoldDeleteBlocked
result: pass

## Summary

total: 6
passed: 6
issues: 0
pending: 0
skipped: 0
blocked: 0
open_gaps: 2

## Gaps

<!-- 2 findings พบระหว่างทดสอบ Test 3 ด้วย live stack (ผ่าน Bruno) — RBAC logic เองผ่าน
     แต่ live stack ใช้งานจริงไม่ได้จนกว่าจะแก้ 2 จุดนี้ -->

- truth: "หลัง `docker compose up` (cold start) ระบบใช้งาน end-to-end ได้จริง — endpoint ที่แตะ DB (user-creation, audit) ทำงานได้"
  status: failed
  reason: "พบระหว่าง Test 3: live `donnarec_app` ไม่มีตารางเลย (ไม่มีแม้ schema_migrations). docker-compose ไม่มีขั้น migrate และ main.go ไม่รัน migrate ตอน startup → migration รันเฉพาะใน testcontainers เท่านั้น. POST /api/admin/users + audit ล้มด้วย 'relation \"audit_log\"/\"users\" does not exist' จนกว่าจะ `make migrate-up` เอง"
  severity: major
  test: 1
  root_cause: "ไม่มี migration runner ใน cold-start path: docker-compose.yml มีแค่ postgres/keycloak/api และ api depends_on เฉพาะ health ของ 2 ตัวนั้น; main.go startup เรียกแค่ pool.Ping ไม่รัน migrate.up; migration ถูกรันเฉพาะใน testcontainers (testutil.SetupTestPostgres) → live `donnarec_app` จึงว่างเปล่าจนกว่าจะ `make migrate-up` เอง (ยืนยันสด: \\dt ว่าง, ไม่มี schema_migrations; หลัง make migrate-up ได้ครบ 5 ตาราง + endpoint ทำงาน)"
  artifacts:
    - path: "donnarec-api/docker-compose.yml"
      issue: "ไม่มี service/step รัน golang-migrate (เช่น init container หรือ entrypoint) ก่อน api start"
    - path: "donnarec-api/cmd/server/main.go"
      issue: "startup ไม่เรียก migrate up (มีแค่ pool.Ping)"
  missing:
    - "เพิ่มขั้นรัน migration ใน cold-start path (init container migrate/migrate, หรือ run-on-startup ใน main.go, หรือ document `make migrate-up` ใน compose flow ให้ชัด)"
  debug_session: ""

- truth: "token ที่ออกโดย Keycloak ผ่าน URL ที่ client เข้าถึง (localhost:8080) ผ่านการ verify ของ API"
  status: failed
  reason: "พบระหว่าง Test 3: token ที่ขอผ่าน http://localhost:8080 มี iss=http://localhost:8080/realms/donnarec แต่ API คาดหวัง iss=http://keycloak:8080/... (KEYCLOAK_BASE_URL). KC_HOSTNAME_STRICT=false ทำให้ iss อิง Host ของ request → go-oidc reject 401 ทุก token. Bruno เลี่ยงชั่วคราวด้วย header Host:keycloak:8080 แต่ frontend จริง (Phase 6) ที่ login ผ่าน localhost จะเจอ 401"
  severity: major
  test: 3
  root_cause: "asymmetric hostname: API ทำ OIDC discovery ที่ KEYCLOAK_BASE_URL=http://keycloak:8080 (docker network) → go-oidc verifier บังคับ iss=http://keycloak:8080/realms/donnarec; แต่ KC_HOSTNAME_STRICT=false ทำให้ Keycloak สร้าง iss ตาม Host ของ request → token ที่ขอผ่าน localhost:8080 ได้ iss=http://localhost:8080/... → verifier.Verify reject = 401 (ยืนยันสด: token localhost → 401, token Host:keycloak:8080 → 200). 01-04 ตั้ง KC_HOSTNAME_URL=keycloak:8080 แก้ให้ discovery ฝั่ง API ตรง แต่ไม่ครอบ token ที่ client ฝั่ง browser ขอผ่าน localhost"
  artifacts:
    - path: "donnarec-api/docker-compose.yml"
      issue: "KC_HOSTNAME_URL=http://keycloak:8080 + KC_HOSTNAME_STRICT=false ทำให้ browser-facing iss (localhost) ไม่ตรงกับ backend-facing iss (keycloak) ที่ API ตรวจ"
  missing:
    - "ทำให้ iss สอดคล้องกันทั้ง browser และ backend (เช่น ใช้ canonical hostname เดียว + เพิ่ม host alias ให้ api เข้าถึง Keycloak ด้วยชื่อเดียวกับ browser, หรือปรับ go-oidc ให้ accept issuer ที่ตั้งไว้)"
  debug_session: ""
