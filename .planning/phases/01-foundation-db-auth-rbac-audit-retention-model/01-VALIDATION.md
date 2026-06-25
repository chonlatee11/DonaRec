---
phase: 1
slug: foundation-db-auth-rbac-audit-retention-model
status: validated
nyquist_compliant: true
wave_0_complete: true
created: 2026-06-23
validated: 2026-06-24
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | `go test` (standard library + testify) + testcontainers-go for PostgreSQL/Keycloak integration |
| **Config file** | none — Wave 0 installs (go.mod test deps + docker-compose for local) |
| **Quick run command** | `go test ./... -short` |
| **Full suite command** | `go test ./... -count=1` (includes integration tests against testcontainers) |
| **Estimated runtime** | ~60 seconds (quick ~10s; full with containers ~60s) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./... -short`
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

> Reconciled against the executed phase. Each Phase 1 success criterion maps to at least one green automated test below. Test names/paths verified against the implemented codebase; full suite run green on 2026-06-24 (`go test ./... -count=1`, incl. testcontainers integration).

| Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 01-01 | W1 | NFR-01 | T-1-auth | login issues valid OIDC session; Bearer token validated with audience enforced (valid→200, missing/wrong-aud/expired→401) | integration | `go test ./internal/auth/... -run TestOIDCMiddleware_Integration` | ✅ `internal/auth/middleware_integration_test.go` | ✅ green |
| 01-01 / 01-04 | W1 | FR-34 | T-1-rbac | server-side RBAC rejects unpermitted role (403); multi-role honored (D-02); admin assigns roles | unit+integration | `go test ./internal/auth/... -run TestRequireRoles_Unit` + `go test ./internal/users/... -run TestCreateUserRBAC` | ✅ `internal/auth/rbac_test.go`, `internal/users/handler_test.go` | ✅ green |
| 01-03 | W3 | NFR-02 | T-1-pii | sensitive-ID stored AES-256-GCM ciphertext, never plaintext; envelope DEK/KEK + blind index | unit | `go test ./internal/crypto/... -run 'TestEnvelopeRoundTrip\|TestAESGCMRoundTrip\|TestBlindIndex\|TestEnvKeyProvider'` | ✅ `internal/crypto/keyprovider_test.go`, `internal/crypto/aes_gcm_test.go` | ✅ green |
| 01-02 | W2 | NFR-05 / FR-13 | T-1-audit | every mutation writes immutable audit row; hash-chain verifies; UPDATE/DELETE denied at DB; middleware covers all mutations | integration | `go test ./internal/audit/... -run 'TestAuditImmutability\|TestHashChainVerification\|TestAuditMiddlewareCoverage'` | ✅ `internal/audit/immutability_test.go`, `internal/audit/service_test.go`, `internal/audit/middleware_test.go` | ✅ green |
| 01-02 | W2 | NFR-05 | T-1-audit-conc | concurrent audit inserts keep hash-chain intact (pg_advisory_xact_lock) | integration | `go test ./internal/audit/... -run TestConcurrentAuditInserts -race` | ✅ `internal/audit/concurrent_test.go` | ✅ green |
| 01-03 | W3 | NFR-03 | T-1-retention | retain_until config-driven + legal_basis + legal_hold; hard-delete denied under legal_hold (app guard + DB trigger) | unit+integration | `go test ./internal/retention/... -run 'TestLegalHoldDeleteBlocked\|TestRetainUntilCalculation\|TestGuardHardDeleteUnit\|TestSoftDeleteAllowed'` | ✅ `internal/retention/retention_test.go` | ✅ green |
| 01-03 / 01-02 | W3 | NFR-02 | T-1-pii-mask | PII masked by default (last-4); reveal role-gated (Admin/Checker only) AND writes an audit row | unit+integration | `go test ./internal/pii/...` + `go test ./internal/audit/... -run TestPIIRevealAudit` | ✅ `internal/pii/mask_test.go`, `internal/audit/middleware_test.go` | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `go.mod` test dependencies installed (testify, testcontainers-go) — `donnarec-api/go.mod`
- [x] `docker-compose.yml` with PostgreSQL 17 + Keycloak 26.6.3 for local/integration tests — `donnarec-api/docker-compose.yml`
- [x] Shared test helpers: DB migration runner + app-role variant — `internal/testutil/postgres.go` (`SetupTestPostgres`, `SetupTestPostgresAsAppRole`)
- [x] Test DB seeding for users/roles (Maker/Checker/Admin) — `internal/users/*_test.go` via testcontainers + migrations

*Greenfield project — Wave 0 established all test infrastructure. Confirmed: full suite green incl. testcontainers.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| TLS/HTTPS served end-to-end in deployed env | NFR-02 | Local dev uses plain HTTP (`sslmode=disable`) by design; cert termination is deployment-time | Deploy behind reverse proxy, `curl -vkI https://localhost` confirms TLS handshake; prod `.env` uses `sslmode=require` |
| Password hashed with argon2id in Keycloak | NFR-01 | Realm config is data + runtime behavior; `realm-donnarec.json` has no explicit `passwordHashingProvider`; Keycloak 26.x default = argon2id, must confirm on running instance | `docker compose up`, create user, check Credentials tab / `SELECT algorithm FROM user_credential;` = argon2id |
| Keycloak realm password policy / lockout values | NFR-01 | Realm config is data, not code; confirm policy applied in admin console | Import realm, verify password policy length(8)+upperCase+digits + bruteForceProtected/maxLoginFailures=5 in Keycloak admin |

*All code-level behaviors have green automated verification. The three items above are deployment/realm-runtime checks that cannot be asserted from code — pre-documented as manual, not coverage gaps. Cross-ref: `01-VERIFICATION.md` §Human Verification Required, `01-HUMAN-UAT.md`.*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 60s (quick ~10s; full suite ~34s observed)
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** validated 2026-06-24 — all 7 code-level requirements covered by green automated tests; 3 deployment/realm-runtime items remain manual-only by design.

---

## Validation Audit 2026-06-24

| Metric | Count |
|--------|-------|
| Requirements in map | 7 |
| Gaps found (MISSING/PARTIAL automated) | 0 |
| Resolved (new tests generated) | 0 |
| Escalated to manual-only | 0 |
| Already-covered (reconciled draft → green) | 7 |
| Pre-documented manual-only items | 3 (TLS, argon2id, realm policy) |

**Finding:** VALIDATION.md was a never-populated draft (TBD task IDs, `pending` statuses, wrong test paths). Audit found every requirement already has a real, green automated test — no test generation needed. Reconciled the Per-Task Map to the implemented codebase and re-ran the full suite (`go test ./... -count=1`) green to confirm. No `gsd-nyquist-auditor` spawn required (zero fillable gaps).
