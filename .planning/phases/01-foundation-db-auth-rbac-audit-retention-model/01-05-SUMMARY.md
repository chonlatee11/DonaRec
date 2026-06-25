---
phase: 01-foundation-db-auth-rbac-audit-retention-model
plan: "05"
subsystem: infra
tags: [docker-compose, golang-migrate, go-oidc, keycloak, cold-start, issuer-config]

# Dependency graph
requires:
  - phase: 01-04
    provides: KEK quoting fix, ../scripts/ bind-mount path fix, KC_HEALTH_ENABLED, Keycloak /dev/tcp healthcheck — all preserved

provides:
  - migrate init-service (image migrate/migrate:v4.19.1) ใน docker-compose.yml รัน golang-migrate up ก่อน api start โดยอัตโนมัติ
  - OIDC expected-issuer configurable ผ่าน env OIDC_ISSUER ใน config.go + middleware.go
  - cold-start stack ที่ใช้งานได้ end-to-end โดยไม่ต้องรัน make migrate-up เอง
  - token จาก localhost:8080 verify ผ่าน API โดยไม่มี Host: keycloak:8080 workaround

affects: [phase-02-gap-less-receipt-numbering, phase-03-donation-lifecycle, all-phases-using-docker-compose]

# Tech tracking
tech-stack:
  added:
    - migrate/migrate:v4.19.1 (docker init-service สำหรับ golang-migrate)
    - oidc.InsecureIssuerURLContext (go-oidc pattern สำหรับ split discovery/issuer URL)
  patterns:
    - init-service pattern: compose service ที่ exit 0 หลังทำงานเสร็จ + depends_on condition: service_completed_successfully
    - configurable OIDC issuer: discovery ที่ internal URL แต่ validate iss = public hostname ที่ตั้งไว้ใน env

key-files:
  created: []
  modified:
    - donnarec-api/docker-compose.yml
    - donnarec-api/internal/auth/middleware.go
    - donnarec-api/internal/config/config.go
    - donnarec-api/cmd/server/main.go
    - donnarec-api/.env.example
    - donnarec-api/internal/auth/middleware_integration_test.go

key-decisions:
  - "LOCKED (user): migration runner เป็น compose init-service แยก ห้ามผูกกับ main.go startup (GAP 1)"
  - "LOCKED (user): แก้ OIDC issuer mismatch ฝั่ง go-oidc ด้วย InsecureIssuerURLContext ไม่ใช่เปลี่ยน KC hostname strategy (GAP 2)"
  - "Safe default: OIDC_ISSUER ไม่ตั้ง → fallback เป็น <KeycloakBaseURL>/realms/<realm> คงพฤติกรรมเดิม ไม่ทำให้ issuer check ถูก skip"
  - "migrate/migrate:v4.19.1 pin tag ตรง Makefile ไม่ใช้ :latest — reduce supply-chain risk (T-01-SC)"
  - "Task 3 verify recipe แก้ไขให้ตรงความจริง: client=donnarec-test-cli, user=admin-test@donnarec.local, --build flag, body format ที่ถูกต้อง"

patterns-established:
  - "init-service compose pattern: image ที่ทำงานครั้งเดียวแล้ว exit 0 ใช้ condition: service_completed_successfully"
  - "InsecureIssuerURLContext pattern: go-oidc discovery ที่ internal docker URL แต่ validate iss = public hostname จาก env"

requirements-completed: [NFR-01, FR-34]

# Metrics
duration: ~35min (Tasks 1+2 automated; Task 3 live human-verified)
completed: 2026-06-25
---

# Phase 01 Plan 05: Gap-Closure (migrate init-service + configurable OIDC issuer) Summary

**migrate init-service อัตโนมัติ (GAP 1) + OIDC expected-issuer configurable ผ่าน env (GAP 2) ปลดบล็อก cold-start stack end-to-end โดยไม่มี manual workaround**

## Performance

- **Duration:** ~35 min (Tasks 1+2 automated; Task 3 live human-verified)
- **Started:** 2026-06-25T12:34:00Z (estimated)
- **Completed:** 2026-06-25T19:37:22Z (dafb149 commit timestamp)
- **Tasks:** 3 (2 automated + 1 human-verify checkpoint)
- **Files modified:** 6

## Accomplishments

- **GAP 1 ปิด:** เพิ่ม `migrate` init-service (image migrate/migrate:v4.19.1) ใน docker-compose.yml รัน golang-migrate up อัตโนมัติก่อน api; api depends_on migrate ด้วย condition: service_completed_successfully — cold-start stack มีครบ 5 ตาราง (users, user_roles, retention_config, audit_log, schema_migrations) โดยไม่ต้องรัน `make migrate-up` เอง
- **GAP 2 ปิด:** OIDC verifier ใช้ `oidc.InsecureIssuerURLContext(ctx, expectedIssuer)` เพื่อ discovery ที่ internal keycloak:8080 แต่ validate iss = OIDC_ISSUER env (default: localhost:8080) — token จาก browser/localhost ผ่านโดยไม่ต้อง Host: keycloak:8080 workaround; signature/aud/expiry checks ยังบังคับครบ
- **Live verified (human):** cold-start `docker compose down -v && up -d --build --wait` + POST /api/admin/users ด้วย admin token จาก localhost:8080 → 201; negative cases (no token 401, bad token 401, maker → 403) ผ่าน

## Task Commits

1. **Task 1: เพิ่ม migrate init-service (GAP 1)** — `f7ac2c7` (fix)
2. **Task 2: OIDC expected-issuer configurable (GAP 2)** — `dafb149` (fix)
3. **Task 3: Live verification (human-approved)** — ไม่มี source commit (checkpoint ยืนยันบน live stack)

**Plan metadata (SUMMARY + plan-doc + tracking):** commit ในขั้นตอนนี้

## Files Created/Modified

- `donnarec-api/docker-compose.yml` — เพิ่ม migrate init-service + OIDC_ISSUER env บน api; api depends_on migrate: service_completed_successfully
- `donnarec-api/internal/auth/middleware.go` — NewAuthMiddleware รับ expectedIssuer param; ใช้ oidc.InsecureIssuerURLContext; signature/aud/expiry checks ไม่เปลี่ยน
- `donnarec-api/internal/config/config.go` — เพิ่ม KeycloakIssuer field โหลดจาก OIDC_ISSUER; safe default fallback
- `donnarec-api/cmd/server/main.go` — ส่ง cfg.KeycloakIssuer เข้า NewAuthMiddleware
- `donnarec-api/.env.example` — เอกสาร OIDC_ISSUER ใต้ Keycloak block พร้อม dev/prod example
- `donnarec-api/internal/auth/middleware_integration_test.go` — อัปเดต call sites ของ NewAuthMiddleware ให้ตรง signature ใหม่ (Rule 1 auto-fix)
- `.planning/phases/01-foundation-db-auth-rbac-audit-retention-model/01-05-PLAN.md` — แก้ Task 3 verify recipe ให้ตรงความจริง (deviation doc)

## Decisions Made

- migrate init-service แยกจาก main.go เป็น LOCKED user decision (GAP 1) — migration runner ใน binary ของ Go application เป็น anti-pattern ที่ผูก lifecycle ของ DDL กับ application restart
- InsecureIssuerURLContext เป็น go-oidc official pattern สำหรับกรณี split internal/external URL — ชื่อ "Insecure" เป็น misnomer; JWKS fetch ยังเกิดจาก discovery URL ภายใน; issuer validate ต่อ expected value
- OIDC_ISSUER default fallback เป็น KeycloakBaseURL+realm — ไม่ breaking change สำหรับ environment ที่ discovery URL = issuer URL (เช่น production ที่ใช้ domain เดียวกัน)
- migrate/migrate:v4.19.1 pin tag ตรง Makefile — ลด supply-chain risk (T-01-SC ใน threat model)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] middleware_integration_test.go call-site อัปเดตตาม signature change**
- **Found during:** Task 2 (OIDC issuer configurable)
- **Issue:** NewAuthMiddleware signature เปลี่ยนจาก 4 param เป็น 5 param (เพิ่ม expectedIssuer); integration test ยังเรียก 4 param → build fail
- **Fix:** อัปเดต call sites ใน middleware_integration_test.go ให้ส่ง IssuerURL (expectedIssuer) เป็น param ที่ 4 (ก่อน logger)
- **Files modified:** `donnarec-api/internal/auth/middleware_integration_test.go`
- **Verification:** `go build ./...` + `go test ./internal/auth/...` ผ่านหลังแก้
- **Committed in:** dafb149 (Task 2 commit)

### Plan Documentation Correction (Task 3 verify recipe)

**2. [Documentation Accuracy] Task 3 `<action>` และ `<how-to-verify>` มี recipe ผิดสำหรับ live token**
- **Found during:** Task 3 (live human verification)
- **ปัญหา (สิ่งที่ plan เขียนไว้ผิด):**
  - `client_id`: plan ระบุ `donnarec-backend` — แต่ `donnarec-backend` คือ bearer-only client ออก token ไม่ได้ (Keycloak reject ด้วย "client not allowed"); ต้องใช้ `donnarec-test-cli` (สร้างโดย `Bruno/setup-test-users.sh` มี directAccessGrantsEnabled=true + audience mapper ฉีด aud=donnarec-backend)
  - `user credentials`: plan ระบุ `admin/changeme` — นั่นคือ master-realm Keycloak admin ไม่ใช่ donnarec realm user; ต้องใช้ `admin-test@donnarec.local / TestPass123` (seeded โดย setup-test-users.sh)
  - `--build flag`: plan ระบุแค่ `docker compose up -d --wait` — ไม่มี `--build` ทำให้ใช้ api image เก่าก่อนแก้ Go code; ต้องใช้ `docker compose up -d --build --wait`
  - `prerequisite step`: plan ไม่ระบุว่าต้องรัน `bash Bruno/setup-test-users.sh` ก่อน — script นี้สร้าง test client + users ที่ขาดไม่ได้สำหรับ verify
  - `POST /api/admin/users body`: plan ระบุ `{first_name,last_name,role}` — body จริงคือ `{"email","display_name","keycloak_subject","roles":[...]}`
- **แก้ไข:** อัปเดต `<action>` และ `<how-to-verify>` ใน 01-05-PLAN.md ให้ตรงกับ recipe ที่ใช้งานจริงและ human-verified แล้ว
- **Files modified:** `.planning/phases/01-foundation-db-auth-rbac-audit-retention-model/01-05-PLAN.md`
- **ผลลัพธ์:** Human verifier รัน recipe ที่ถูกต้อง (donnarec-test-cli + admin-test@donnarec.local + --build + setup-test-users.sh) ผ่านครบทุก acceptance criteria

---

**Total deviations:** 2 (1 Rule 1 auto-fix inline; 1 plan documentation correction)
**Impact on plan:** Rule 1 fix จำเป็นสำหรับ build correctness; doc correction จำเป็นสำหรับ reproducible live verification — ทั้งคู่ไม่กระทบ scope หรือ architecture

## Verification

### Automated (Tasks 1+2)

| Check | Result |
|-------|--------|
| `docker compose config -q` | PASS — ไม่มี YAML/schema error |
| `grep migrate/migrate docker-compose.yml` | PASS — image pinned v4.19.1 |
| `grep service_completed_successfully docker-compose.yml` | PASS — api depends_on migrate |
| `grep InsecureIssuerURLContext middleware.go` | PASS — issuer context สร้างถูกต้อง |
| ไม่มี SkipIssuerCheck/SkipClientIDCheck/SkipExpiryCheck (นอก comment) | PASS — grep filtered = 0 |
| `go build ./...` | PASS — exit 0 |
| `go test ./internal/auth/... ./internal/config/...` | PASS — ไม่มี FAIL |
| migrate ไม่ใช้ restart: unless-stopped | PASS — init-service exit 0 แล้วหยุด |
| OIDC_ISSUER ปรากฏใน .env.example + docker-compose.yml | PASS |
| 01-04 fixes ยังอยู่ครบ (../scripts/, KC_HEALTH_ENABLED, /dev/tcp) | PASS |

### Live Human-Verified (Task 3)

**GAP 1 — Cold-start migration:**
- `docker compose down -v && docker compose up -d --build --wait` → exit 0, ทุก service healthy, migrate = completed (exit 0)
- `psql -d donnarec_app -c '\dt'` → ครบ 5 ตาราง: users, user_roles, retention_config, audit_log, schema_migrations
- ไม่ได้รัน `make migrate-up` เลย → **PASS**

**GAP 2 — OIDC issuer ผ่าน localhost:**
- `bash Bruno/setup-test-users.sh` รัน prerequisite step สำเร็จ
- Token ขอผ่าน `localhost:8080` (client_id=donnarec-test-cli, admin-test@donnarec.local) ผ่าน API โดยไม่มี Host: keycloak:8080 workaround
- POST /api/admin/users → 201 → **PASS**
- Negative cases: no token → 401, bad token → 401, maker token on admin endpoint → 403 → **PASS**

## Issues Encountered

การแก้ Go code ใน Task 2 ทำให้ integration test call site break เนื่องจาก signature change (handled เป็น Rule 1 auto-fix ด้านบน). ไม่มีปัญหา blocking อื่น

## User Setup Required

การรัน live stack บน environment ใหม่ต้องการขั้นตอนต่อไปนี้:
1. Copy `.env.example` เป็น `.env` และตั้งค่า: `DB_PASSWORD`, `KC_ADMIN_PASSWORD`, `DONAREC_KEK` (`openssl rand -hex 32`), `OIDC_ISSUER=http://localhost:8080/realms/donnarec`
2. `cd donnarec-api && docker compose up -d --build --wait`
3. `bash Bruno/setup-test-users.sh` (จาก repo root) — สร้าง test client + users ใน Keycloak

## Next Phase Readiness

Phase 01 Foundation สมบูรณ์ทุก 5 plans:
- cold-start stack ใช้งานได้ end-to-end โดยไม่มี manual workaround
- NFR-01 (login + RBAC end-to-end) และ FR-34 (admin user mgmt) verified บน live stack
- Phase 02 (Gap-less Receipt Numbering Core) พร้อมเริ่ม — foundation DB schema + RBAC + audit trail พร้อมรองรับ receipt counter table

---

## Self-Check

**Checking files exist:**
- [x] `donnarec-api/docker-compose.yml` — modified in f7ac2c7 + dafb149
- [x] `donnarec-api/internal/auth/middleware.go` — modified in dafb149
- [x] `donnarec-api/internal/config/config.go` — modified in dafb149
- [x] `donnarec-api/cmd/server/main.go` — modified in dafb149
- [x] `donnarec-api/.env.example` — modified in dafb149
- [x] `donnarec-api/internal/auth/middleware_integration_test.go` — modified in dafb149
- [x] `.planning/phases/01-foundation-db-auth-rbac-audit-retention-model/01-05-PLAN.md` — Task 3 recipe corrected

**Checking commits exist:**
- [x] `f7ac2c7` — fix(01-05): add migrate init-service for cold-start migrations (GAP 1)
- [x] `dafb149` — fix(01-05): make OIDC expected issuer configurable via OIDC_ISSUER (GAP 2)

## Self-Check: PASSED

*Phase: 01-foundation-db-auth-rbac-audit-retention-model*
*Completed: 2026-06-25*
