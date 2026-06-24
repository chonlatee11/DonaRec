---
status: partial
phase: 01-foundation-db-auth-rbac-audit-retention-model
source: [01-VERIFICATION.md]
started: 2026-06-24
updated: 2026-06-24
---

## Current Test

[testing complete — Test 1 ผ่าน (human-verified argon2id + stack boots healthy); Test 2 ยัง blocked แบบ deploy-time]

## Tests

### 1. Keycloak ใช้ argon2id สำหรับ password hashing
expected: หลัง `docker compose up` (stack: Postgres + Keycloak + API) เปิด Keycloak Admin Console → Realm `donnarec` → Authentication → Policies → Password policy / Realm defaults แล้วยืนยันว่า password hashing algorithm = `argon2` (argon2id). หมายเหตุ: Keycloak 24+ ตั้ง argon2 เป็น default algorithm อยู่แล้ว
result: pass
verified: "[2026-06-24] human-verified หลัง gap-closure 01-04: `docker compose down -v && up -d --wait` → postgres/keycloak/api healthy ครบ; Keycloak Admin Console เข้าได้; Realm donnarec password hashing = argon2(id)"
reported: "เข้า Keycloak Admin Console ไม่ได้เลย — docker log พัง: pgsql 'password authentication failed for user donnarec', keycloak 'Failed to obtain JDBC connection', api ไม่ start (ก่อนหน้านี้เจอ yaml: line 102 mapping values not allowed)"
severity: blocker
note: "local stack boot ไม่ขึ้นเลยจาก defect 5 จุดใน docker-compose.yml + realm-donnarec.json (ดู Gaps). แต่ละจุดถูก reproduce + ยืนยัน root cause ด้วยการรัน stack จริงระหว่าง verify (fix ถูก revert ออกแล้วเพื่อให้ execute-phase ลงมือพร้อม atomic commit). เมื่อ stack boot ได้ ค่อยทำ argon2 verification ตัวจริง"
followup: "[2026-06-24] gaps ทั้ง 5 ปิดแล้วผ่าน plan 01-04 (+ 2 deviation auto-fix: OIDC issuer mismatch → KC_HOSTNAME_URL, API distroless healthcheck → debian:bookworm-slim). blocker หมดแล้ว. ขั้นถัดไป (human): `cd donnarec-api && docker compose down -v && docker compose up -d --wait` → เปิด Keycloak Admin Console → Realm donnarec → Authentication → Policies → ยืนยัน password hashing = argon2(id)"

### 2. HTTPS/TLS ใน deployed environment (SC#5)
expected: ใน production/staging ที่ deploy จริง ต้องมี reverse proxy (เช่น nginx/traefik) terminate TLS และบังคับ HTTPS; การเชื่อม API ↔ Postgres ใช้ `sslmode=verify-full`. หมายเหตุ: local dev ใช้ HTTP + `sslmode=disable` ตาม design decision และ VALIDATION.md §Manual-Only ระบุว่าเป็น deploy-time verification
result: blocked
blocked_by: deploy-time
reason: "เป็น deploy-time verification ที่ต้องทำตอน deploy production จริง (ไม่เกี่ยวกับ local stack)"

## Summary

total: 2
passed: 1
issues: 0
pending: 0
skipped: 0
blocked: 1

## Gaps

<!-- 5 gaps — ทุกตัวบล็อกการ boot ของ local stack ซึ่งบล็อก Test 1 (argon2). root cause + fix ยืนยันด้วยการรัน stack จริงระหว่าง verify-work แล้ว -->

- truth: "docker compose parse `donnarec-api/docker-compose.yml` ได้สำเร็จ"
  status: fixed
  reason: "User reported: yaml: line 102: mapping values are not allowed in this context"
  severity: blocker
  test: 1
  root_cause: "บรรทัด 102 — ค่า env DONAREC_KEK เป็น unquoted scalar ที่มี ': ' (โคลอน+เว้นวรรค) ในข้อความ default error '...generate with: openssl rand -hex 32' → YAML parser ตีความเป็น mapping ทำให้ทั้งไฟล์ parse ไม่ผ่าน"
  artifacts:
    - path: "donnarec-api/docker-compose.yml"
      issue: "บรรทัด 102: DONAREC_KEK value ไม่ได้ครอบ quote"
  missing:
    - "ครอบค่า DONAREC_KEK ทั้งบรรทัดด้วย double quotes: DONAREC_KEK: \"${DONAREC_KEK:?...}\""
  verified_fix: "ครอบด้วย double quotes → `docker compose config -q` ผ่าน"

- truth: "init script `create-multiple-dbs.sh` ถูก mount และรันตอน Postgres init → สร้าง DB donnarec_keycloak"
  status: fixed
  reason: "Keycloak/Postgres: init script ไม่เคยรัน, Docker auto-create empty directory แทนไฟล์"
  severity: blocker
  test: 1
  root_cause: "docker-compose.yml บรรทัด 35 อ้าง './scripts/create-multiple-dbs.sh' (= donnarec-api/scripts/...) แต่ไฟล์จริงถูก track ที่ repo-root 'scripts/' (commit 5d30807). path ไม่ตรง → Docker bind-mount auto-create empty dir ที่ donnarec-api/scripts/create-multiple-dbs.sh → init ไม่รัน. หมายเหตุ: keycloak volume บรรทัด 75 ใช้ '../keycloak' ถูกแล้ว แต่ scripts ลืมแก้เป็น '../'"
  artifacts:
    - path: "donnarec-api/docker-compose.yml"
      issue: "บรรทัด 35: ./scripts/create-multiple-dbs.sh ควรเป็น ../scripts/create-multiple-dbs.sh"
    - path: "donnarec-api/docker-compose.yml"
      issue: "บรรทัด 12 (comment): bash scripts/seed-admin.sh ควรเป็น bash ../scripts/seed-admin.sh"
  missing:
    - "แก้ volume path เป็น ../scripts/create-multiple-dbs.sh"
    - "แก้ comment ให้สอดคล้อง"
    - "ลบ empty dir donnarec-api/scripts/ ที่ Docker auto-create (ถ้ายังมี)"
    - "ต้อง docker compose down -v ทิ้ง volume เก่าก่อน up ใหม่ เพื่อให้ init รันสร้าง donnarec_keycloak + ตั้ง password ตรง"
  verified_fix: "เปลี่ยนเป็น ../scripts → init log แสดง 'Creating database: donnarec_keycloak' + \\l เห็น DB ครบ"

- truth: "Keycloak import realm `realm-donnarec.json` สำเร็จ"
  status: fixed
  reason: "Keycloak: ERROR Failed to run import — Unrecognized field '_comment_security' not marked as ignorable"
  severity: blocker
  test: 1
  root_cause: "realm-donnarec.json มี pseudo-comment keys ที่ JSON ไม่รองรับ (_comment_security, _comment_brute_force, _comment_session, _comment_ssl, _comment_users, และ _note) — Keycloak RealmRepresentation deserializer แบบ strict ปฏิเสธ unknown field → import ล้ม"
  artifacts:
    - path: "keycloak/realm-donnarec.json"
      issue: "keys ขึ้นต้นด้วย _ (5x _comment_* + 1x _note) ไม่ใช่ field ของ RealmRepresentation"
  missing:
    - "ลบ key ทุกตัวที่ขึ้นต้นด้วย '_' ออกจาก realm JSON (เก็บ rationale ไว้ใน comment ของ planning/SUMMARY แทน ไม่ใช่ใน JSON)"
  verified_fix: "ลบ _* keys → import ผ่านชั้นนี้ (ไปเจอ gap ถัดไป)"

- truth: "Keycloak import realm สำเร็จ (field schema ถูกต้อง)"
  status: fixed
  reason: "Keycloak: ERROR Unrecognized field 'maxLoginFailures' not marked as ignorable (line 18)"
  severity: blocker
  test: 1
  root_cause: "realm-donnarec.json มี field 'maxLoginFailures': 5 ซึ่งไม่ใช่ field ของ Keycloak RealmRepresentation — field ที่ถูกต้องสำหรับ brute-force lockout คือ 'failureFactor' ซึ่งมีอยู่แล้ว (value 5 เท่ากัน) → maxLoginFailures เป็นตัวซ้ำที่ผิด schema"
  artifacts:
    - path: "keycloak/realm-donnarec.json"
      issue: "field maxLoginFailures ไม่มีใน RealmRepresentation (ซ้ำกับ failureFactor ที่ถูกต้องอยู่แล้ว)"
  missing:
    - "ลบ field maxLoginFailures (คง failureFactor: 5 ไว้)"
  verified_fix: "ลบ maxLoginFailures → 'Import finished successfully', Keycloak 26.6.3 started, Listening on :8080"

- truth: "Keycloak container healthcheck รายงาน healthy → api (depends_on healthy) start ได้"
  status: fixed
  reason: "Keycloak Up แต่ (unhealthy) → api ไม่ start: 'dependency failed to start: keycloak is unhealthy'"
  severity: blocker
  test: 1
  root_cause: "healthcheck ของ keycloak ผิด 2 จุดสำหรับ Keycloak 26: (1) ใช้ 'curl' ซึ่ง image ไม่มี (NO_CURL) (2) ยิง http://localhost:8080/health/ready แต่ Keycloak 26 เสิร์ฟ /health บน management port 9000 ไม่ใช่ 8080 (8080 → 404) และต้องตั้ง KC_HEALTH_ENABLED=true ถึงจะเปิด management listener (9000 refused เพราะยังไม่เปิด)"
  artifacts:
    - path: "donnarec-api/docker-compose.yml"
      issue: "บรรทัด 81-82: healthcheck ใช้ curl (ไม่มีใน image) + ยิงพอร์ต/path ผิด"
    - path: "donnarec-api/docker-compose.yml"
      issue: "keycloak environment ขาด KC_HEALTH_ENABLED=true"
  missing:
    - "เพิ่ม KC_HEALTH_ENABLED: \"true\" ใน keycloak environment"
    - "เปลี่ยน healthcheck เป็นแบบไม่ใช้ curl ยิงพอร์ต 9000 เช่น /dev/tcp: exec 3<>/dev/tcp/localhost/9000 && printf 'GET /health/ready HTTP/1.1\\r\\nHost: localhost\\r\\nConnection: close\\r\\n\\r\\n' >&3 && grep -q '200' <&3 (sh ใน image รองรับ /dev/tcp — ทดสอบแล้ว)"
  verified_fix: "apply แล้วใน plan 01-04 (commit a45e5c5): เพิ่ม KC_HEALTH_ENABLED + healthcheck /dev/tcp/localhost/9000 → `docker compose config -q` ผ่าน; stack boot ครบ healthy ระหว่าง verify-work"
