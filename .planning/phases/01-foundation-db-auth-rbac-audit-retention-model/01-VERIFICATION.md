---
phase: 01-foundation-db-auth-rbac-audit-retention-model
verified: 2026-06-25T21:00:00+07:00
status: passed
score: 6/6 must-haves verified
overrides_applied: 0
re_verification:
  previous_status: human_needed
  previous_score: 5/6
  gaps_closed:
    - "GAP 1: cold-start migration runner — migrate init-service (01-05 Task 1, commit f7ac2c7) + live-verified 2026-06-25 (5 tables auto-created, migrate exit 0, POST /api/admin/users 201)"
    - "GAP 2: OIDC issuer hostname mismatch — InsecureIssuerURLContext configurable via OIDC_ISSUER (01-05 Task 2, commit dafb149) + live-verified 2026-06-25 (token via localhost:8080, no Host workaround, 201)"
    - "argon2id human verification — live UAT Test 1 passed (commit fd0a950); Keycloak 26.6.3 uses argon2id as default; realm config password policy enforced (length 8, upperCase, digits, bruteForce protection)"
  gaps_remaining: []
  regressions: []
---

# Phase 01: Foundation (DB, Auth/RBAC, Audit, Retention model) Verification Report

**Phase Goal:** Staff can log in under enforced Maker/Checker/Admin roles, every significant action is recorded in a tamper-proof audit trail, and the data model encodes PDPA-vs-tax retention from day one.
**Verified:** 2026-06-25T21:00:00+07:00
**Status:** passed
**Re-verification:** Yes — after gap closure (01-05 gap-closure plan + live human verification 2026-06-25)

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | เจ้าหน้าที่ login ผ่าน Keycloak (OIDC) และรหัสผ่านถูกเก็บเป็น argon2id hash; Admin สร้าง user และกำหนด role ได้ | VERIFIED | `internal/auth/middleware.go`: `InsecureIssuerURLContext` + `provider.Verifier({ClientID})` wired + `internal/users/handler.go` RequireRoles(RoleAdmin); live UAT Test 1 (argon2id) + Test 3 (admin→201, no-token→401, maker→403) ผ่าน; SC#1 "exactly one role" deviation intentionally documented D-03 (multi-role, SoD per-record Phase 3) |
| 2 | Endpoint ปฏิเสธ action ที่ role ไม่มีสิทธิ์ด้วย 403; national/tax-ID ถูก mask สำหรับ role ที่ไม่มีสิทธิ์เปิดเผย | VERIFIED | `internal/auth/rbac.go` RequireRoles() multi-role AND; `internal/pii/mask.go` MaskNationalID last-4 + minRevealLen=10; CanRevealFull Admin||Checker=true, Maker=false; TestRequireRoles_Unit + TestMaskNationalID ผ่าน; live: maker→403, checker→403 |
| 3 | ทุก login/role change/data action เขียน audit row ที่ลบ/แก้ไม่ได้; UPDATE/DELETE บน audit_log ถูก deny ระดับ DB | VERIFIED | `migrations/000002_audit_log.up.sql` REVOKE UPDATE,DELETE ON audit_log FROM donnarec_app; `internal/audit/service.go` SHA-256 hash-chain + pg_advisory_xact_lock; `internal/audit/middleware.go` AuditMiddleware ครอบ POST/PUT/PATCH/DELETE + /reveal; TestAuditImmutability + TestHashChainVerification + TestConcurrentAuditInserts -race ผ่าน |
| 4 | ทุก donor/receipt record มี `retain_until` + legal_basis field; ไม่มี code path ที่ hard-delete record ที่อยู่ภายใต้ legal hold | VERIFIED | `migrations/000001_init_schema.up.sql` legal_basis_enum; `internal/retention/service.go` ComputeRetainUntil config-driven + GuardHardDelete; `migrations/000003_retention_triggers.up.sql` prevent_legal_hold_delete trigger; TestRetainUntilCalculation + TestLegalHoldDeleteBlocked ผ่าน |
| 5 | PII ถูกเข้ารหัสที่ระดับแอปด้วย AES-256-GCM envelope (DEK/KEK) ไม่อยู่ใน plaintext; HTTPS/TLS เป็น deployment-time control (documented) | VERIFIED | `internal/crypto/keyprovider.go` KeyProvider interface (LOCKED); `internal/crypto/aes_gcm.go` stdlib AES-256-GCM; `internal/crypto/envelope.go` DEK+WrapKey; `cmd/server/main.go` InsecureDatabaseTLS() warning สำหรับ sslmode=disable กับ non-localhost; VALIDATION.md documented HTTPS/TLS as deployment verification; TestEnvelopeRoundTrip ผ่าน |
| 6 | Cold-start stack ทำงาน end-to-end โดยไม่ต้องรัน make migrate-up เอง; token จาก localhost:8080 ผ่าน API verify โดยไม่ต้องใช้ Host workaround | VERIFIED | `docker-compose.yml`: migrate service (migrate/migrate:v4.19.1) + api depends_on migrate: service_completed_successfully; `internal/auth/middleware.go` oidc.InsecureIssuerURLContext; `internal/config/config.go` KeycloakIssuer โหลดจาก OIDC_ISSUER env พร้อม safe default; live-verified 2026-06-25: cold-start 5 tables, POST /api/admin/users 201, negative cases 401/403 |

**Score:** 6/6 truths verified

---

### Required Artifacts (01-05 additions verified)

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `donnarec-api/docker-compose.yml` | migrate init-service + api depends_on service_completed_successfully + OIDC_ISSUER env | VERIFIED | บรรทัด 111-126: migrate/migrate:v4.19.1 + ./migrations:/migrations:ro; บรรทัด 155-156: depends_on migrate: service_completed_successfully; บรรทัด 143: OIDC_ISSUER env; WR-07 fixed: DB_PASSWORD:? guard บรรทัด 121 |
| `donnarec-api/internal/auth/middleware.go` | InsecureIssuerURLContext; provider.Verifier(ClientID) ยังอยู่; ไม่มี Skip* flags | VERIFIED | บรรทัด 64: `oidc.InsecureIssuerURLContext(context.Background(), expectedIssuer)`; บรรทัด 75: `provider.Verifier(&oidc.Config{ClientID: clientID})`; grep filtered SkipIssuerCheck/SkipClientIDCheck/SkipExpiryCheck = 0 |
| `donnarec-api/internal/config/config.go` | KeycloakIssuer field โหลดจาก OIDC_ISSUER พร้อม safe default | VERIFIED | บรรทัด 34: `KeycloakIssuer string`; บรรทัด 75: `getEnvStr("OIDC_ISSUER", oidcIssuerDefault)` ใช้ fallback = `<KeycloakBaseURL>/realms/<realm>` |
| `donnarec-api/cmd/server/main.go` | ส่ง cfg.KeycloakIssuer เข้า NewAuthMiddleware | VERIFIED | บรรทัด 80: `cfg.KeycloakIssuer` เป็น param ที่ 4 ใน NewAuthMiddleware call |
| `donnarec-api/.env.example` | OIDC_ISSUER documented พร้อม dev/prod example | VERIFIED | บรรทัด 43: `OIDC_ISSUER=http://localhost:8080/realms/donnarec` พร้อม comment อธิบาย dev vs prod |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `docker-compose.yml (api)` | migrate init-service | `depends_on migrate: condition: service_completed_successfully` | WIRED | บรรทัด 155-156 |
| `docker-compose.yml (migrate)` | `donnarec-api/migrations/` | bind mount `./migrations:/migrations:ro` | WIRED | บรรทัด 117 |
| `internal/auth/middleware.go` | OIDC_ISSUER config value | `oidc.InsecureIssuerURLContext(ctx, expectedIssuer)` + `provider.Verifier(ClientID)` | WIRED | บรรทัด 64 + 75 |
| `cmd/server/main.go` | `cfg.KeycloakIssuer` | `auth.NewAuthMiddleware(..., cfg.KeycloakIssuer, logger)` | WIRED | บรรทัด 80 |
| `internal/auth/middleware.go` | Keycloak JWKS | `oidc.NewProvider(ctx, providerURL)` + `provider.Verifier({ClientID})` | WIRED | Discovery ที่ internal URL, validate iss = expectedIssuer |
| `internal/users/service.go` | PostgreSQL users table | sqlc queries via pgx pool | WIRED | CreateUser → AssignRoles; TestCreateAndGetUser PASS |
| `cmd/server/main.go` | auth + rbac middleware | router.Use + route group guards | WIRED | AuditMiddleware ก่อน RequireAuth ก่อน RequireRoles (Pattern D) |
| `internal/audit/service.go` | audit_log table | pg_advisory_xact_lock + nextval + INSERT | WIRED | AppendAuditEntryTx ใน single transaction |
| `internal/crypto/envelope.go` | KeyProvider + aes_gcm | EncryptField DEK+WrapKey; DecryptField UnwrapKey | WIRED | บรรทัด 45 + 69 |
| `internal/retention/service.go` | retention_config + legal_hold trigger | ComputeRetainUntil + GuardHardDelete + DB trigger | WIRED | TestRetainUntilCalculation + TestLegalHoldDeleteBlocked PASS |

---

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|-------------------|--------|
| `internal/users/service.go`.CreateUser | user row + roles | INSERT via sqlc; reads from pgx pool | YES — TestCreateAndGetUser write+read จาก testcontainers PostgreSQL | FLOWING |
| `internal/audit/service.go`.AppendAuditEntryTx | audit row + hash | nextval + INSERT INTO audit_log; VerifyChain reads all rows | YES — TestHashChainVerification 5 rows, tamper detected | FLOWING |
| `internal/crypto/envelope.go`.EncryptField | ciphertext + wrappedDEK | rand.Read DEK + AES-GCM.Seal + KeyProvider.WrapKey | YES — TestEnvelopeRoundTrip + tamper-detection | FLOWING |
| `internal/pii/mask.go`.MaskNationalID | masked string | string manipulation, minRevealLen=10 guard | YES — TestMaskNationalID table-driven + boundary test | FLOWING |
| `internal/retention/service.go`.ComputeRetainUntil | retain_until time | cfg.DonationRetainDays / cfg.AuditLogRetainDays | YES — TestRetainUntilCalculation (custom 2190d config reflected) | FLOWING |

---

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| go build ผ่านทุก package | `go build ./...` | exit 0 (01-05 SUMMARY verified) | PASS |
| InsecureIssuerURLContext present | `grep -c "InsecureIssuerURLContext" middleware.go` | 1 | PASS |
| ไม่มี Skip* flags ใน middleware | `grep -vE '^\s*//' middleware.go \| grep -cE "Skip(Issuer\|ClientID\|Expiry)Check"` | 0 | PASS |
| OIDC_ISSUER ใน docker-compose.yml | `grep -c "OIDC_ISSUER" docker-compose.yml` | 2 (comment + value) | PASS |
| service_completed_successfully | `grep -c "service_completed_successfully" docker-compose.yml` | 1 | PASS |
| migrate/migrate:v4.19.1 | `grep -c "migrate/migrate" docker-compose.yml` | 1 (image line) | PASS |
| DB_PASSWORD:? guard (WR-07 fix) | `grep "DB_PASSWORD" migrate command` | `${DB_PASSWORD:?DB_PASSWORD is required}` | PASS |
| .gitignore covers .env (CR-01 false positive confirmed) | `git check-ignore donnarec-api/.env` | `.gitignore:3:*.env` | PASS |
| Cold-start 5 tables + 201 (live) | `docker compose down -v && up -d --build --wait` | 5 tables, migrate exit 0, POST 201 | PASS (live-verified 2026-06-25) |
| Token localhost:8080 ผ่าน (live) | POST /api/admin/users ไม่ใช้ Host workaround | 201; no-token→401, maker→403 | PASS (live-verified 2026-06-25) |

---

### Probe Execution

ไม่มี probe scripts ที่ declared ใน PLAN frontmatter หรือ conventional `scripts/*/tests/probe-*.sh` — Step 7c: SKIPPED (no probes declared)

---

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|---------|
| NFR-01 | 01-01, 01-05 | login + RBAC server-side + cold-start stack | SATISFIED | Keycloak OIDC authN + Go RBAC authZ; InsecureIssuerURLContext; TestOIDCMiddleware_Integration + TestRequireRoles_Unit PASS; live UAT Test 3: admin→201, maker→403, no-token→401 |
| FR-34 | 01-01, 01-05 | Admin จัดการผู้ใช้และสิทธิ์ | SATISFIED | /api/admin/users endpoint RequireRoles(RoleAdmin); migrate init-service ทำให้ DB tables พร้อมใช้; live UAT 201 ยืนยัน |
| NFR-02 | 01-03 | เข้ารหัสข้อมูลขณะส่ง + เข้ารหัส PII ขณะจัดเก็บ | SATISFIED (boundary) | AES-256-GCM envelope + KeyProvider LOCKED; InsecureDatabaseTLS() warning; HTTPS/TLS เป็น deployment verification (VALIDATION.md §Manual-Only); donor PII usage deferred Phase 3 |
| NFR-05 | 01-02 | audit log ลบไม่ได้ + retain ตามระยะ | SATISFIED | REVOKE UPDATE/DELETE on audit_log; SHA-256 hash-chain + advisory lock; retention_config 3650 days; TestAuditImmutability PASS |
| FR-13 | 01-02 | audit trail ทุกการกระทำ | SATISFIED | AuditMiddleware ครอบ POST/PUT/PATCH/DELETE + /reveal GET; ActorIdentity() fallback preferred_username; TestAuditMiddlewareCoverage PASS |
| NFR-03 | 01-03 | retention/consent/legal_hold model | SATISFIED (boundary) | ComputeRetainUntil config-driven; GuardHardDelete + DB trigger; legal_basis_enum; consent capture deferred Phase 3/6 (documented) |

---

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/users/handler.go` | ~73 | `// TODO(01-04): complete CRUD` | Info | Intentional stub — cross-references plan 01-04; ไม่มี unresolved debt |
| `internal/audit/service.go` | ~140,174,181 | Raw SQL strings (advisory lock, nextval, NOW()) | Info | ไม่ใช่ SQL injection — PostgreSQL-specific calls ที่จำเป็นสำหรับ hash-chain; documented exception |
| `docker-compose.yml` | 121 | `${DB_PASSWORD:?DB_PASSWORD is required}` (WR-07) | Info (fixed) | Fixed ใน commit dafb149 — guard เพิ่มแล้ว ตรงกับ services อื่น |

ไม่พบ TBD, FIXME, XXX markers ที่ไม่มี reference — debt marker gate ผ่าน

---

### Human Verification Required

ไม่มี — ทุก human verification items จาก verification ก่อนหน้าได้รับการ resolve แล้ว:

- **argon2id:** live UAT Test 1 ผ่าน (commit fd0a950 `test(01): UAT complete - 1 passed (argon2id)`); Keycloak 26.6.3 ใช้ argon2id เป็น default ยืนยันบน running instance
- **HTTPS/TLS:** VALIDATION.md documented เป็น deployment-time verification (non-blocker ต่อ phase goal); `InsecureDatabaseTLS()` warning เพิ่มแล้วใน `cmd/server/main.go`; local dev sslmode=disable เป็น design decision ถูกต้อง

---

### Gaps Summary

ไม่มี gap ที่ยังเปิดอยู่ ทุก must-have artifact มีอยู่จริง, substantive, wired, และ data flows เป็นที่พิสูจน์ได้

**Gaps ที่ปิดแล้วในรอบนี้:**

1. **GAP 1 — Cold-start migration runner:** 01-05 Task 1 เพิ่ม `migrate` init-service (migrate/migrate:v4.19.1) ใน docker-compose.yml พร้อม `api depends_on migrate: service_completed_successfully` live-verified 2026-06-25: cold-start `down -v && up -d --build --wait` → 5 tables auto-created, migrate exit 0

2. **GAP 2 — OIDC issuer hostname mismatch:** 01-05 Task 2 แก้ `internal/auth/middleware.go` ใช้ `oidc.InsecureIssuerURLContext(ctx, expectedIssuer)` + config `OIDC_ISSUER` env ใน `config.go` + `cmd/server/main.go` ส่ง `cfg.KeycloakIssuer` live-verified 2026-06-25: token จาก localhost:8080 → 201 โดยไม่มี Host workaround; negative cases (bad token 401, maker 403) ยังถูกต้อง

**ROADMAP SC#1 deviation (D-03):** "assign exactly one role" vs multi-role ที่ implement — intentional user decision D-03 ใน 01-CONTEXT.md; SoD ยังครบ (บังคับระดับ record Phase 3); roadmap update deferred to post-phase

---

## Commit History (01-05 additions)

| Commit | Description |
|--------|-------------|
| f7ac2c7 | fix(01-05): add migrate init-service for cold-start migrations (GAP 1) |
| dafb149 | fix(01-05): make OIDC expected issuer configurable via OIDC_ISSUER (GAP 2) |

---

_Verified: 2026-06-25T21:00:00+07:00_
_Verifier: Claude (gsd-verifier)_
_Re-verification: Yes — after 01-05 gap closure + live human verification 2026-06-25_
