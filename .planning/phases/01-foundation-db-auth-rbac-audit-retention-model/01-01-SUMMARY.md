---
phase: 01-foundation-db-auth-rbac-audit-retention-model
plan: "01"
subsystem: auth-rbac-db-skeleton
tags: [go, gin, keycloak, oidc, postgresql, sqlc, pgx, tdd, walking-skeleton]
dependency_graph:
  requires: []
  provides:
    - "Go module github.com/donnarec/donnarec-api with pinned dependencies"
    - "internal/auth: KeycloakClaims, AuthMiddleware, RequireRoles"
    - "internal/db: WithTx helper, sqlc-generated Queries"
    - "internal/users: UserService, UserHandler"
    - "migrations/000001_init_schema: users, user_roles, retention_config"
    - "docker-compose: PostgreSQL 17 + Keycloak 26.6.3 + API"
    - "keycloak/realm-donnarec.json: realm with RBAC roles"
  affects:
    - "Plan 01-02: audit middleware wires into main.go TODO(01-02) hook"
    - "Plan 01-03: retention triggers extend migration 000001 schema"
    - "Plan 01-04: CRUD handlers build on UserHandler stub"
tech_stack:
  added:
    - "Go 1.25 + gin v1.12.0 (HTTP router)"
    - "jackc/pgx v5.10.0 (PostgreSQL driver)"
    - "coreos/go-oidc v3.19.0 (OIDC token validation)"
    - "golang-jwt/jwt v5.3.1 (JWT parsing)"
    - "sqlc v1.31.1 (type-safe SQL codegen)"
    - "golang-migrate v4.19.1 (schema migrations)"
    - "testcontainers-go v0.43.0 (integration test fixtures)"
    - "go-i18n v2.6.1 (i18n message catalogs)"
    - "zap v1.28.0 (structured logging)"
    - "validator v10.30.3 (input validation)"
    - "testify v1.11.1 (test assertions)"
    - "Keycloak 26.6.3 (self-hosted OIDC provider)"
    - "PostgreSQL 17 (primary DB)"
  patterns:
    - "Constructor injection throughout — no global state"
    - "TDD: RED (test without impl) → GREEN (impl) → commit per wave"
    - "WithTx(ctx, pool, fn) transaction helper (Pattern B)"
    - "httptest JWKS server for OIDC tests (no live Keycloak needed)"
    - "Claims from realm_access.roles only (Keycloak Pitfall 1)"
    - "Keycloak start (production mode) not start-dev (Pitfall 4)"
key_files:
  created:
    - donnarec-api/go.mod
    - donnarec-api/go.sum
    - donnarec-api/Makefile
    - donnarec-api/.env.example
    - donnarec-api/Dockerfile
    - donnarec-api/docker-compose.yml
    - donnarec-api/cmd/server/main.go
    - donnarec-api/internal/auth/claims.go
    - donnarec-api/internal/auth/middleware.go
    - donnarec-api/internal/auth/rbac.go
    - donnarec-api/internal/auth/rbac_test.go
    - donnarec-api/internal/auth/middleware_integration_test.go
    - donnarec-api/internal/config/config.go
    - donnarec-api/internal/db/sqlc.yaml
    - donnarec-api/internal/db/helpers.go
    - donnarec-api/internal/db/queries/users.sql
    - donnarec-api/internal/db/generated/db.go
    - donnarec-api/internal/db/generated/models.go
    - donnarec-api/internal/db/generated/querier.go
    - donnarec-api/internal/db/generated/users.sql.go
    - donnarec-api/internal/i18n/bundle.go
    - donnarec-api/internal/i18n/locales/th.json
    - donnarec-api/internal/i18n/locales/en.json
    - donnarec-api/internal/testutil/postgres.go
    - donnarec-api/internal/testutil/keycloak.go
    - donnarec-api/internal/users/handler.go
    - donnarec-api/internal/users/handler_test.go
    - donnarec-api/internal/users/service.go
    - donnarec-api/internal/users/service_test.go
    - donnarec-api/migrations/000001_init_schema.up.sql
    - donnarec-api/migrations/000001_init_schema.down.sql
    - keycloak/realm-donnarec.json
    - scripts/seed-admin.sh
    - scripts/create-multiple-dbs.sh
  modified: []
decisions:
  - "D-20 realized: Go backend with gin v1.12.0 HTTP router"
  - "D-21 realized: PostgreSQL 17 as primary DB with pgx/v5 driver"
  - "D-23 realized: sqlc + pgx for type-safe data access (no ORM)"
  - "D-24 realized: Keycloak OIDC authN + Go app authZ (RBAC guard)"
  - "D-26 realized: Docker-compose local-first with Keycloak self-hosted"
  - "D-27 realized: go-i18n v2 message catalog (th.json, en.json)"
  - "D-01 realized: Admin-only user creation endpoint (/api/admin/users)"
  - "D-02 realized: multi-role via user_roles junction table + HasRole check"
  - "D-05 realized: seed-admin.sh bootstrap script"
  - "D-06 realized: passwordPolicy length(8)+upperCase+digits in realm-donnarec.json"
  - "D-07 realized: bruteForceProtected=true, maxLoginFailures=5 in realm"
  - "D-08 realized: accessTokenLifespan=300, ssoSessionIdleTimeout=1800, max=28800"
  - "D-04 stub: SoD comment in rbac.go pointing to Phase 3"
metrics:
  duration_minutes: 16
  completed_date: "2026-06-24"
  tasks_completed: 3
  tasks_total: 3
  files_created: 33
  files_modified: 0
---

# Phase 01 Plan 01: Walking Skeleton (Auth + DB + RBAC) Summary

**One-liner:** JWT auth via Keycloak OIDC with Go RBAC guard, sqlc/pgx data layer with multi-role PostgreSQL schema, and httptest JWKS token minting for fast integration tests.

## Tasks Completed

| Task | Name | Commit | Result |
|------|------|--------|--------|
| 1 | Wave 0 — scaffold Go module + auth tests RED | da4f75a | RED: tests compile-error without impl |
| 2 | Schema migration + sqlc data layer GREEN | 453c904 | GREEN: TestCreateAndGetUser PASS |
| 3 | OIDC auth + RBAC + Gin wiring + Keycloak realm GREEN | 5d30807 | GREEN: all auth + handler tests PASS |

## What Was Built

### Walking Skeleton

สร้าง Go backend ที่ทำงานได้ครบ end-to-end:

- **OIDC token validation** via `coreos/go-oidc/v3` ค้นหา Keycloak JWKS อัตโนมัติ และ verify `iss` + `aud` claim
- **RBAC guard** `RequireRoles(...)` อ่าน roles จาก `realm_access.roles` เท่านั้น (ไม่ใช่ top-level `roles`)
- **Multi-role support** (D-02): `user_roles` junction table + `HasRole` method ใน claims
- **Type-safe data layer** sqlc codegen จาก `users.sql` → `internal/db/generated/` ด้วย pgx/v5
- **Transaction helper** `WithTx(ctx, pool, fn)` สำหรับ atomic operations
- **Gin router** ลำดับ middleware ถูกต้อง: Recovery → Logger → RequireAuth → RequireRoles
- **docker-compose** stack: postgres:17 + keycloak:26.6.3 (`start` mode) + API

### Test Infrastructure

- `SetupTestPostgres(t)` — testcontainers postgres:17 + golang-migrate auto-run
- `NewKeycloakTestServer(t)` + `MintToken(clientID, roles...)` — httptest RSA JWKS server
- ไม่ต้องใช้ live Keycloak สำหรับ integration tests

### Schema (Migration 000001)

```
users (id, email, display_name, keycloak_subject, is_active, legal_hold, created_at, updated_at)
user_roles (user_id PK, role PK) — multi-role junction
retention_config (entity_type UNIQUE, default_retain_days, legal_basis, updated_by)
ENUMs: user_role_enum(maker|checker|admin), legal_basis_enum(tax_obligation|consent|legitimate_interest)
```

### Keycloak Realm

- `realm-donnarec.json`: passwordPolicy `length(8)+upperCase+digits`, bruteForceProtected=true, maxLoginFailures=5
- accessTokenLifespan=300s, ssoSessionIdleTimeout=1800s, max=28800s (D-06/D-07/D-08)
- roles: maker, checker, admin; clients: donnarec-backend (bearerOnly), donnarec-frontend (public PKCE)

## Test Results (GREEN)

```
TestRequireRoles_Unit PASS (unit, -short compatible)
  ✓ maker claims → RequireRoles("maker") 200
  ✓ maker claims → RequireRoles("admin") 403
  ✓ no claims → RequireRoles 401
  ✓ [maker,checker] claims → passes both guards (D-02)
  ✓ HasRole reads realm_access.roles not top-level

TestOIDCMiddleware_Integration PASS
  ✓ valid token (correct aud) → 200
  ✓ missing Authorization → 401
  ✓ wrong audience → 401
  ✓ malformed token → 401
  ✓ admin token → admin guard 200
  ✓ maker token → admin guard 403

TestCreateAndGetUser PASS (testcontainers postgres:17)
  ✓ CreateUser with 2 roles → DB write
  ✓ GetUser round-trip → email/display_name/roles match
  ✓ no roles → error
  ✓ invalid UUID → error

TestMigrationRoundTrip PASS
  ✓ users, user_roles, retention_config tables exist
  ✓ retention_config seeded with 2 rows

TestCreateUserRBAC PASS (testcontainers postgres:17)
  ✓ admin token → POST /api/admin/users 201
  ✓ maker token → 403
  ✓ checker token → 403
  ✓ invalid body → 400
  ✓ missing fields → 422
  ✓ GET /api/me → returns sub + email
```

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] External test package imports required dot-import**
- **Found during:** Task 2 verification (go vet)
- **Issue:** rbac_test.go and middleware_integration_test.go used `package auth_test` but did not import the `auth` package — Go external test packages require explicit import
- **Fix:** Added `. "github.com/donnarec/donnarec-api/internal/auth"` dot-import to both test files so `KeycloakClaims`, `RequireRoles`, etc. are in scope
- **Files modified:** `internal/auth/rbac_test.go`, `internal/auth/middleware_integration_test.go`
- **Commit:** included in 453c904 (staging of modified test files)

**2. [Rule 3 - Blocking] Missing go.sum entries for go-oidc/oauth2/go-jose**
- **Found during:** Task 3 `go build ./...` after creating middleware.go
- **Issue:** `coreos/go-oidc/v3` transitively requires `go-jose/v4` and `oauth2` which were not yet in go.sum
- **Fix:** `go get github.com/coreos/go-oidc/v3/oidc@v3.19.0 && go mod tidy` — added missing entries
- **Files modified:** `go.mod`, `go.sum`
- **Commit:** included in Task 3 commit

**3. [Rule 2 - Critical functionality] Added handler_test.go for RBAC verification**
- **Found during:** Task 3 planning
- **Issue:** Plan specified `TestCreateUserRBAC` but no test file was listed in `<files>`
- **Fix:** Created `internal/users/handler_test.go` with 6 test cases covering admin/maker/checker role enforcement on user creation endpoint
- **Files modified:** `donnarec-api/internal/users/handler_test.go` (new)
- **Commit:** 5d30807

**4. [Rule 2 - Critical functionality] Added scripts/create-multiple-dbs.sh**
- **Found during:** Task 3 docker-compose.yml authoring
- **Issue:** docker-compose references `POSTGRES_MULTIPLE_DATABASES` env var which requires a PostgreSQL init script to create the Keycloak database; without it Keycloak cannot persist data
- **Fix:** Created `scripts/create-multiple-dbs.sh` (idiomatic pattern from docker postgres image docs)
- **Files modified:** `scripts/create-multiple-dbs.sh` (new)
- **Commit:** 5d30807

### Out-of-scope discoveries (deferred)

- Migration roundtrip down/up not tested automatically (testutil only runs `up`; down tested manually via Makefile)
- `pgtype.UUID` pgx driver type used in generated code — callers use `.String()` and `.Scan()` which works correctly
- Test query strings in `service_test.go` (`pool.QueryRow(ctx, "SELECT COUNT(*) ...")`) are test-only probe queries, not service SQL — acceptable per Foundational Rule 4 (rule targets service code, not test code)

## Known Stubs

| Stub | File | Line | Reason |
|------|------|------|--------|
| `// TODO(01-02): audit-in-tx` | `internal/users/service.go` | ~78 | Audit middleware wired in plan 01-02; stub left intentionally |
| `// TODO(01-02): audit middleware placeholder` | `cmd/server/main.go` | ~138 | Same — plan 01-02 adds the audit Gin middleware |
| `// TODO(01-04): complete CRUD` | `internal/users/handler.go` | ~78 | Update/list/deactivate endpoints in plan 01-04 |
| `// RequireNotCreator — implemented Phase 3 (D-04)` | `internal/auth/rbac.go` | comment | SoD per-record guard stub; Phase 3 implements it |

These stubs do NOT prevent the plan's goal (walking skeleton with auth + RBAC + DB read/write) from being achieved.

## Threat Surface Scan

No new threat surfaces beyond those analyzed in the plan's `<threat_model>`.

All mitigations implemented:
- T-1-auth-01: `oidc.NewProvider` + `ClientID` in config → enforces `iss` + `aud`
- T-1-auth-02: `bruteForceProtected=true`, `maxLoginFailures=5`, `passwordPolicy length(8)...` in realm JSON
- T-1-auth-03: `accessTokenLifespan=300`, `ssoSessionIdleTimeout=1800`, `max=28800`
- T-1-rbac-01: `RequireRoles(...)` server-side guard on all protected routes
- T-1-rbac-02: claims read from `realm_access.roles` only (Pitfall 1 mitigated in code + tests)
- T-1-tamper-01: sqlc parameterized queries; no SQL string concatenation in service code
- T-1-config-01: docker-compose uses `start` not `start-dev` (data persists in PostgreSQL)
- T-1-info-01: zap structured logger; token/PII never logged

## Self-Check: PASSED

Files exist:
- [x] donnarec-api/go.mod
- [x] donnarec-api/internal/auth/claims.go
- [x] donnarec-api/internal/auth/middleware.go
- [x] donnarec-api/internal/auth/rbac.go
- [x] donnarec-api/internal/db/generated/db.go (sqlc generated)
- [x] donnarec-api/migrations/000001_init_schema.up.sql
- [x] donnarec-api/internal/testutil/postgres.go
- [x] donnarec-api/docker-compose.yml
- [x] keycloak/realm-donnarec.json
- [x] scripts/seed-admin.sh

Commits exist:
- [x] da4f75a — test(01-01): scaffold Go module + auth tests RED
- [x] 453c904 — feat(01-01): schema migration + sqlc data layer GREEN
- [x] 5d30807 — feat(01-01): OIDC auth + RBAC + Gin wiring + Keycloak realm GREEN
