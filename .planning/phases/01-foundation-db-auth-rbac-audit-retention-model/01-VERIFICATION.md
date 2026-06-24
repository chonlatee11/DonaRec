---
phase: 01-foundation-db-auth-rbac-audit-retention-model
verified: 2026-06-24T19:50:00+07:00
status: human_needed
score: 5/6 must-haves verified
overrides_applied: 0
human_verification:
  - test: "ยืนยัน password hashing ด้วย argon2id ใน Keycloak realm config"
    expected: >
      เมื่อสร้าง user ใน Keycloak realm 'donnarec' แล้วตรวจสอบใน Keycloak Admin Console
      > Users > Credentials หรือ DB dump ว่า hash algorithm เป็น argon2id
      (Keycloak 26.x ใช้ argon2id เป็น default แต่ realm-donnarec.json ไม่ได้ระบุ
      passwordHashingProvider อย่างชัดเจน — ต้องยืนยัน deploy จริง)
    why_human: >
      realm-donnarec.json ไม่มี passwordHashingProvider field ชัดเจน
      ต้องรัน Keycloak 26.6.3 จริง (docker compose up) แล้วตรวจสอบ hash algorithm
      ที่ใช้กับ credential แรกที่สร้าง — ไม่สามารถยืนยันจาก static file ได้
  - test: "ยืนยัน HTTPS/TLS end-to-end ใน deployed environment"
    expected: >
      curl -vkI https://localhost ได้รับ TLS handshake สำเร็จ; Keycloak อยู่หลัง
      reverse proxy ที่ terminate TLS; .env สลับ sslmode=require สำหรับ production
    why_human: >
      .env.example ใช้ sslmode=disable สำหรับ local dev — เป็น design decision ที่ถูกต้อง
      แต่ SC#5 ของ Phase 1 ระบุ "served over HTTPS/TLS" ซึ่งต้องยืนยันใน deployment จริง
      VALIDATION.md ระบุชัดว่านี่คือ manual/deploy verification
      (อ้างอิง: 01-VALIDATION.md §Manual-Only)
---

# Phase 01: Foundation (DB, Auth/RBAC, Audit, Retention model) Verification Report

**Phase Goal:** Staff can log in under enforced Maker/Checker/Admin roles, every significant action is recorded in a tamper-proof audit trail, and the data model encodes PDPA-vs-tax retention from day one.
**Verified:** 2026-06-24T19:50:00+07:00
**Status:** human_needed
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | เจ้าหน้าที่ login ผ่าน Keycloak (OIDC) แล้วเข้าถึง protected endpoint ได้ด้วย Bearer token | VERIFIED | `internal/auth/middleware.go`: `oidc.NewProvider` + `provider.Verifier(&oidc.Config{ClientID: clientID})` — TestOIDCMiddleware_Integration PASS (valid→200, missing→401, wrong-aud→401, expired→401) |
| 2 | RBAC ปฏิเสธ role ไม่พอด้วย 403 ฝั่ง server | VERIFIED | `internal/auth/rbac.go`: `RequireRoles(...)` อ่านจาก `realm_access.roles` เท่านั้น — TestRequireRoles_Unit PASS (maker→admin guard→403, multi-role [maker,checker] pass both guards D-02) |
| 3 | ทุก mutation เขียน audit row ที่ลบ/แก้ไม่ได้ระดับ DB + hash-chain verify ได้ | VERIFIED | `migrations/000002_audit_log.up.sql`: `REVOKE UPDATE, DELETE ON audit_log FROM donnarec_app`; `internal/audit/service.go`: SHA-256 hash-chain + pg_advisory_xact_lock; TestAuditImmutability, TestHashChainVerification, TestConcurrentAuditInserts -race ทั้งหมด PASS |
| 4 | AES-256-GCM envelope encryption boundary พร้อม; PII mask + role-gated reveal; legal_hold block hard-delete app+DB | VERIFIED | `internal/crypto/`: KeyProvider interface (LOCKED), EnvKeyProvider อ่าน DONAREC_KEK จาก env, cipher.NewGCM stdlib เท่านั้น; `internal/pii/mask.go`: MaskNationalID last-4, CanRevealFull Admin+Checker; `migrations/000003_retention_triggers.up.sql`: prevent_legal_hold_delete trigger — TestEnvelopeRoundTrip, TestMaskNationalID, TestLegalHoldDeleteBlocked ทั้งหมด PASS |
| 5 | retain_until config-driven; legal_hold block ทั้ง app และ DB level | VERIFIED | `internal/retention/service.go`: ComputeRetainUntil อ่านจาก RetentionConfig ไม่ hardcode literal; GuardHardDelete + DB trigger defense-in-depth; TestRetainUntilCalculation, TestLegalHoldDeleteBlocked PASS |
| 6 | Password stored as argon2id hash + HTTPS/TLS in transit | human_needed | passwordPolicy ใน realm-donnarec.json ระบุ length(8)+upperCase+digits แต่ไม่มี passwordHashingProvider field ชัดเจน; HTTPS/TLS เป็น deployment-time configuration (sslmode=disable ใน local dev .env.example เป็น design decision ถูกต้อง) — ต้อง verify ใน deployed environment |

**Score:** 5/6 truths verified

---

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `donnarec-api/cmd/server/main.go` | entrypoint + Gin router wiring + graceful shutdown | VERIFIED | มี setupRouter, graceful shutdown ผ่าน signal.NotifyContext, AuditMiddleware wired before RequireAuth (Pattern D) |
| `donnarec-api/internal/auth/middleware.go` | OIDC JWT validation via go-oidc | VERIFIED | NewAuthMiddleware, RequireAuth, oidc.NewProvider + provider.Verifier({ClientID}) — audience enforced (Pitfall 3) |
| `donnarec-api/internal/auth/rbac.go` | RBAC guard + role constants | VERIFIED | RoleMaker/RoleChecker/RoleAdmin constants; RequireRoles() multi-role AND logic; SoD stub comment Phase 3 (D-04) |
| `donnarec-api/migrations/000001_init_schema.up.sql` | users, user_roles, retention_config + enums | VERIFIED | CREATE TABLE users, user_roles (PK user_id,role), retention_config; ENUMs user_role_enum + legal_basis_enum; seeded 2 retention rows |
| `donnarec-api/migrations/000002_audit_log.up.sql` | append-only audit_log + REVOKE UPDATE/DELETE | VERIFIED | BIGSERIAL PK, prev_hash + row_hash NOT NULL; `REVOKE UPDATE, DELETE ON audit_log FROM donnarec_app` บรรทัด 95 |
| `donnarec-api/internal/audit/service.go` | hash-chain compute + AppendAuditEntryTx | VERIFIED | computeRowHash (SHA-256 pipe-delimited), advisory lock, nextval+NOW() pre-compute, no goroutine spawn |
| `donnarec-api/internal/audit/middleware.go` | generic Gin audit interceptor | VERIFIED | AuditMiddleware, isPIIRevealEndpoint (/reveal suffix), deriveAction; does NOT call c.Abort on audit error |
| `donnarec-api/internal/crypto/keyprovider.go` | KeyProvider interface (LOCKED boundary) | VERIFIED | `type KeyProvider interface` มี WrapKey/UnwrapKey; LOCKED comment ชัดเจน |
| `donnarec-api/internal/crypto/aes_gcm.go` | AES-256-GCM + BlindIndex stdlib only | VERIFIED | `cipher.NewGCM`, `crypto/aes`, `crypto/hmac`, `crypto/sha256` — ไม่มี external crypto lib |
| `donnarec-api/internal/pii/mask.go` | MaskNationalID + CanRevealFull | VERIFIED | MaskNationalID last-4 visible format; CanRevealFull Admin||Checker = true, Maker = false (D-10) |
| `donnarec-api/migrations/000003_retention_triggers.up.sql` | prevent_legal_hold_delete trigger | VERIFIED | CREATE OR REPLACE FUNCTION prevent_legal_hold_delete() + CREATE TRIGGER on users BEFORE DELETE |
| `donnarec-api/internal/retention/service.go` | ComputeRetainUntil + GuardHardDelete | VERIFIED | config-driven days (donation/audit_log), GuardHardDelete returns AppError i18n key "retention.legal_hold_delete_blocked" |
| `donnarec-api/internal/testutil/postgres.go` | SetupTestPostgres + SetupTestPostgresAsAppRole | VERIFIED | ทั้ง 2 function มี: SetupTestPostgres ใช้ testcontainers+migration; SetupTestPostgresAsAppRole ใช้สำหรับ REVOKE test |
| `donnarec-api/docker-compose.yml` | Postgres + Keycloak + API local stack | VERIFIED | postgres:17 + keycloak:26.6.3 (`start --import-realm` ไม่ใช่ start-dev Pitfall 4) + api; healthcheck ครบทุก service |
| `keycloak/realm-donnarec.json` | Realm ที่ปลอดภัย: passwordPolicy, lockout, session | VERIFIED | passwordPolicy length(8)+upperCase+digits; bruteForceProtected=true, maxLoginFailures=5; accessTokenLifespan=300, ssoSessionIdleTimeout=1800 |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/auth/middleware.go` | Keycloak JWKS | `oidc.NewProvider(ctx, baseURL+"/realms/"+realm)` | WIRED | บรรทัด 41: `oidc.NewProvider(ctx, providerURL)` + Verifier({ClientID}) |
| `internal/users/service.go` | PostgreSQL users table | sqlc queries via pgx pool | WIRED | CreateUser → AssignRoles; GetUserByID via db.Queries; TestCreateAndGetUser PASS (testcontainers) |
| `cmd/server/main.go` | auth + rbac middleware | router.Use + route group guards | WIRED | บรรทัด 149: `router.Use(audit.AuditMiddleware(auditSvc))`, 159: `api.Use(authMW.RequireAuth())`, 166: `adminGroup.Use(auth.RequireRoles(auth.RoleAdmin))` — ลำดับถูกต้องตาม Pattern D |
| `internal/audit/service.go` | audit_log table | pg_advisory_xact_lock + nextval + INSERT | WIRED | AppendAuditEntryTx ใช้ advisory lock, reserve nextval, compute hash, INSERT ใน single transaction |
| `internal/audit/middleware.go` | AuditService | post-handler AppendAuditEntry call | WIRED | บรรทัด 95: `svc.AppendAuditEntry(c.Request.Context(), entry)` — synchronous ไม่ใช่ goroutine |
| `internal/crypto/envelope.go` | KeyProvider + aes_gcm | EncryptField: rand DEK + AES-GCM + WrapKey | WIRED | บรรทัด 45: `kp.WrapKey(ctx, dek)`; DecryptField บรรทัด 69: `kp.UnwrapKey(ctx, wrappedDEK)` |
| `internal/retention/service.go` | retention_config + legal_hold trigger | ComputeRetainUntil reads cfg.DonationRetainDays; GuardHardDelete app check + DB trigger | WIRED | TestRetainUntilCalculation + TestLegalHoldDeleteBlocked (testcontainers) ทั้งคู่ PASS |

---

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|-------------------|--------|
| `internal/users/service.go`.CreateUser | user row + roles | INSERT via sqlc-generated AssignRole; reads from pgx pool | YES — TestCreateAndGetUser write+read จาก testcontainers PostgreSQL | FLOWING |
| `internal/audit/service.go`.AppendAuditEntryTx | audit row + hash | nextval + INSERT INTO audit_log; VerifyChain reads all rows | YES — TestHashChainVerification 5 rows, tamper detected | FLOWING |
| `internal/crypto/envelope.go`.EncryptField | ciphertext + wrappedDEK | rand.Read DEK + AES-GCM.Seal + KeyProvider.WrapKey | YES — TestEnvelopeRoundTrip, round-trip + tamper-detection | FLOWING |
| `internal/pii/mask.go`.MaskNationalID | masked string | string manipulation of last-4 | YES — TestMaskNationalID table-driven | FLOWING |
| `internal/retention/service.go`.ComputeRetainUntil | retain_until time | cfg.DonationRetainDays / cfg.AuditLogRetainDays | YES — TestRetainUntilCalculation (custom 2190d config reflected) | FLOWING |

---

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| go build ผ่านทุก package | `go build ./...` | exit 0, no output | PASS |
| go vet ผ่านทุก package | `go vet ./...` | exit 0, no output | PASS |
| RBAC unit tests | `go test ./internal/auth/... -run TestRequireRoles -short` | PASS (8 sub-tests) | PASS |
| Crypto round-trip tests | `go test ./internal/crypto/... -count=1` | PASS (TestAESGCMRoundTrip, TestEnvelopeRoundTrip, TestBlindIndex, TestEnvKeyProvider) | PASS |
| PII mask tests | `go test ./internal/pii/... -count=1` | PASS (TestMaskNationalID, TestCanRevealFull) | PASS |
| OIDC middleware integration | `go test ./internal/auth/... -count=1` | PASS (TestOIDCMiddleware_Integration, TestRequireRoles_Unit) | PASS |
| Users service + handler (testcontainers) | `go test ./internal/users/... -count=1` | PASS (TestCreateAndGetUser, TestMigrationRoundTrip, TestCreateUserRBAC) | PASS |
| Audit hash-chain + concurrency -race (testcontainers) | `go test ./internal/audit/... -race -count=1` | PASS (TestAuditImmutability, TestHashChainVerification, TestConcurrentAuditInserts, TestAuditMiddlewareCoverage, TestPIIRevealAudit, TestAuditMiddlewareNoAbortOnError, TestAuditRetainColumns) | PASS |
| Retention unit tests | `go test ./internal/retention/... -run TestRetainUntil\|TestSoftDelete\|TestGuardHardDelete` | PASS (4 sub-tests) | PASS |
| Retention integration (testcontainers DB trigger) | `go test ./internal/retention/... -run TestLegalHoldDeleteBlocked` | PASS (DB trigger raises exception on legal_hold=true DELETE) | PASS |

---

### Probe Execution

ไม่มี probe scripts ที่ declared ใน PLAN frontmatter หรือ conventional `scripts/*/tests/probe-*.sh` — Step 7c: SKIPPED (no probes declared)

---

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|---------|
| NFR-01 | 01-01 | login + RBAC server-side | SATISFIED | Keycloak OIDC authN + Go RBAC authZ; TestOIDCMiddleware_Integration + TestRequireRoles_Unit PASS |
| FR-34 | 01-01 | Admin จัดการผู้ใช้และสิทธิ์ | SATISFIED | /api/admin/users endpoint RequireRoles(RoleAdmin); user_roles junction table multi-role; TestCreateUserRBAC PASS |
| NFR-02 | 01-03 | เข้ารหัสข้อมูลอ่อนไหวขณะจัดเก็บ (encryption boundary) | SATISFIED (partial) | AES-256-GCM envelope + KeyProvider LOCKED interface พร้อม; donor PII usage deferred Phase 3 (documented in 01-03-PLAN.md objective); HTTPS/TLS เป็น deployment verification |
| NFR-05 | 01-02 | audit log ลบไม่ได้ + retain ตามระยะ | SATISFIED | REVOKE UPDATE/DELETE on audit_log; hash-chain; retention_config seeded 3650 days for audit_log; TestAuditImmutability PASS |
| FR-13 | 01-02 | audit trail ทุกการกระทำ | SATISFIED | AuditMiddleware ครอบคลุม POST/PUT/PATCH/DELETE ทุก mutation + /reveal GET; TestAuditMiddlewareCoverage PASS |
| NFR-03 | 01-03 | retention/consent/legal_hold model | SATISFIED (boundary) | ComputeRetainUntil config-driven; GuardHardDelete + DB trigger; legal_basis_enum; consent capture deferred Phase 3/6 (documented) |

---

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/users/handler.go` | 73 | `// TODO(01-04): complete CRUD` | Info | Intentional stub — cross-references plan 01-04; ไม่มี unresolved debt |
| `internal/users/service.go` | 67 | `// TODO(01-02): audit-in-tx` | Info | Intentional stub — cross-references plan 01-02 ที่ complete แล้ว (plan 01-02 wires audit ผ่าน middleware ไม่ใช่ in-service call); note อาจ outdated แต่ไม่ใช่ blocker |
| `internal/audit/service.go` | 140, 174, 181 | Raw SQL strings (`SELECT pg_advisory_xact_lock`, `SELECT nextval`, `SELECT NOW()`) | Info | ไม่ใช่ SQL string concatenation (Foundational Rule 4); เป็น parameterized PostgreSQL-specific calls ที่จำเป็นสำหรับ advisory lock + sequence + timestamp — acceptable exception ที่ documented ใน service |

ไม่พบ TBD, FIXME, XXX markers ที่ไม่มี reference — debt marker gate ผ่าน

---

### Human Verification Required

#### 1. Password hashing algorithm (argon2id ใน Keycloak 26)

**Test:** รัน `docker compose up` จาก `donnarec-api/`, สร้าง user ผ่าน scripts/seed-admin.sh, เข้า Keycloak Admin Console (localhost:8080) > donnarec realm > Users > admin user > Credentials tab — ดู algorithm ที่ใช้ หรือ query ใน donnarec_keycloak DB: `SELECT algorithm FROM user_credential;`

**Expected:** Algorithm = argon2id (Keycloak 26.x ใช้ argon2id เป็น default ตาม release notes; ถ้าเป็น version เก่ากว่าอาจได้ pbkdf2-sha256)

**Why human:** `realm-donnarec.json` ไม่ได้ระบุ `components.org.keycloak.credential.CredentialProvider` สำหรับ argon2id อย่างชัดเจน — SC#1 ของ Phase 1 ระบุ "password stored only as an argon2id hash" ซึ่ง depend on Keycloak version default behavior ที่ต้องยืนยันจาก running instance

**หากผล = algorithm ไม่ใช่ argon2id:** เพิ่ม components config ใน realm-donnarec.json สำหรับ argon2id credential provider

#### 2. HTTPS/TLS ใน deployed environment (SC#5)

**Test:** Deploy stack ด้วย nginx reverse proxy ที่ terminate TLS → `curl -vkI https://localhost` ดู TLS handshake; ยืนยัน DATABASE_URL ใช้ `sslmode=require` ใน production .env

**Expected:** TLS handshake สำเร็จ; server certificate valid; Keycloak เข้าถึงผ่าน HTTPS; Database connection ใช้ TLS

**Why human:** Local dev stack ใช้ HTTP (sslmode=disable) โดย design — VALIDATION.md §Manual-Only ระบุว่า TLS/HTTPS เป็น deployment-time verification ไม่ใช่ automated test ใน code

---

### Gaps Summary

ไม่มี gap ที่เป็น blocker ต่อ phase goal หลัก ทุก must-have artifact มีอยู่จริง, substantive, wired, และ data flows เป็นที่พิสูจน์ได้จาก test suite

Truth #6 (argon2id + HTTPS/TLS) ต้องการ human verification ตาม VALIDATION.md ที่ documented ไว้ล่วงหน้า:
- HTTPS/TLS เป็น deployment concern ไม่ใช่ code concern (local dev ใช้ HTTP by design)
- argon2id ใน Keycloak 26 เป็น default behavior ที่ต้องยืนยันจาก running instance

ROADMAP SC#1 deviation (D-03): "assign exactly one role" vs multi-role ที่ implement จริง — ผู้ใช้เลือก multi-role (D-02) และ documented ใน 01-CONTEXT.md D-03 ว่าต้อง update roadmap หลังเฟสนี้ Verifier ไม่นับเป็น gap เพราะเป็นการ override ที่ documented อย่างชัดเจนและ intentional ตาม user decision

---

## Commit History

| Commit | Description |
|--------|-------------|
| da4f75a | test(01-01): scaffold Go module + auth tests RED |
| 453c904 | feat(01-01): schema migration + sqlc data layer GREEN |
| 5d30807 | feat(01-01): OIDC auth + RBAC + Gin wiring + Keycloak realm GREEN |
| 13b781c | feat(01-02): audit_log migration + sqlc queries + immutability test GREEN |
| 9aead15 | test(01-02): add failing hash-chain + concurrent audit tests (RED) |
| eb265eb | feat(01-02): hash-chain service + concurrency-safe append GREEN |
| 85888a0 | test(01-02): add failing audit middleware coverage tests (RED) |
| d36250d | feat(01-02): generic audit middleware + router wiring GREEN |
| 3e50115 | test(01-03): add failing tests RED for crypto package |
| a889f46 | feat(01-03): implement crypto package GREEN |
| 86cf27c | test(01-03): add failing tests RED for pii package |
| 62cd66c | feat(01-03): implement pii package GREEN |
| 8dd6bf5 | test(01-03): add failing tests RED for retention package |
| 9fd5b49 | feat(01-03): implement retention package GREEN |

---

_Verified: 2026-06-24T19:50:00+07:00_
_Verifier: Claude (gsd-verifier)_
