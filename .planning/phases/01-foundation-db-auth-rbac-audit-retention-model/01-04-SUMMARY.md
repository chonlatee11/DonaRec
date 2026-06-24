---
phase: "01"
plan: "04"
subsystem: infra/docker
tags: [docker-compose, keycloak, gap-closure, local-dev]
dependency_graph:
  requires: []
  provides: [local-stack-boot, keycloak-realm-import, postgres-multi-db]
  affects: [human-uat-test-1, argon2id-verification]
tech_stack:
  added: []
  patterns:
    - Keycloak 26 healthcheck via /dev/tcp probe on management port 9000 (no curl)
    - KC_HOSTNAME_URL for Docker-internal OIDC issuer alignment
    - debian:bookworm-slim runtime with wget for healthcheck (replaces distroless)
key_files:
  created: []
  modified:
    - donnarec-api/docker-compose.yml
    - keycloak/realm-donnarec.json
    - donnarec-api/Dockerfile
decisions:
  - "Use KC_HOSTNAME_URL=http://keycloak:8080 so OIDC issuer URL in discovery document matches what the Go API sends to go-oidc; KC_HOSTNAME_ADMIN_URL=http://localhost:8080 preserves browser admin console access"
  - "Switch API runtime image to debian:bookworm-slim + wget to support CMD-SHELL healthcheck (distroless has no /bin/sh or wget)"
metrics:
  duration: "~25 min"
  completed_date: "2026-06-24"
  tasks_completed: 3
  files_modified: 3
---

# Phase 01 Plan 04: Gap-Closure Local Stack Boot Summary

Closed 5 root-cause gaps (+ 2 auto-fixed deviations) that blocked local Docker stack from booting, unblocking human UAT Test 1 (argon2id verification via Keycloak Admin Console).

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Fix docker-compose.yml — GAP 1+2+5 | a45e5c5 | donnarec-api/docker-compose.yml |
| 2 | Fix realm-donnarec.json — GAP 3+4 | 9e9a9ec | keycloak/realm-donnarec.json |
| 3 | Boot stack + verify all healthy | — | (verification only) |
| — | Rule 1: Fix OIDC issuer mismatch + API healthcheck | 694427f | docker-compose.yml, Dockerfile |

## Gaps Closed

### GAP 1 — DONAREC_KEK YAML parse error
- **Root cause:** Unquoted YAML scalar `${...}` with `: ` (colon-space) in default error text caused YAML to interpret it as a mapping → parse fail for entire compose file
- **Fix:** Wrapped value in double quotes: `DONAREC_KEK: "${DONAREC_KEK:?...}"`

### GAP 2 — Postgres init script path
- **Root cause:** Volume path `./scripts/create-multiple-dbs.sh` resolves relative to `donnarec-api/` but the file lives at repo-root `scripts/`
- **Fix:** Changed to `../scripts/create-multiple-dbs.sh` + updated usage comment

### GAP 3 — realm JSON unsupported `_comment_*` keys
- **Root cause:** Keycloak RealmRepresentation deserializer uses strict Jackson; unknown fields starting with `_` cause import failure
- **Fix:** Removed all 6 underscore-prefixed keys (`_comment_security`, `_comment_brute_force`, `_comment_session`, `_comment_ssl`, `_comment_users`, `_note`); rationale preserved in planning docs only

### GAP 4 — realm JSON `maxLoginFailures` field
- **Root cause:** `maxLoginFailures` is not a valid RealmRepresentation field; `failureFactor` is the correct field (both were 5, so functionally identical)
- **Fix:** Removed `maxLoginFailures: 5`; retained `failureFactor: 5` (D-07 unchanged)

### GAP 5 — Keycloak 26 healthcheck
- **Root cause:** Healthcheck used `curl` (not in Keycloak 26 image — NO_CURL) and targeted port 8080 (main HTTP) instead of 9000 (management/health endpoint); `KC_HEALTH_ENABLED` was absent so /health/ready returned 404
- **Fix:** Added `KC_HEALTH_ENABLED: "true"`; replaced curl probe with `/dev/tcp/localhost/9000` shell probe that works without curl

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] OIDC issuer URL mismatch (GAP 6 — discovered during Task 3 boot)**
- **Found during:** Task 3 (stack boot verification)
- **Issue:** `go-oidc.NewProvider()` was given `http://keycloak:8080/realms/donnarec` but Keycloak's discovery document returned issuer `http://localhost:8080/realms/donnarec` (due to `KC_HOSTNAME=localhost`). go-oidc enforces issuer match → API crash-looped on startup
- **Fix:** Replaced `KC_HOSTNAME: localhost` + `KC_HOSTNAME_PORT: 8080` with `KC_HOSTNAME_URL: http://keycloak:8080` (sets Docker-internal issuer) and `KC_HOSTNAME_ADMIN_URL: http://localhost:8080` (preserves browser admin console). KC_HOSTNAME_STRICT=false retained (allows localhost access)
- **Files modified:** `donnarec-api/docker-compose.yml`
- **Commit:** 694427f (co-committed with Rule 2 fix below)

**2. [Rule 1 - Bug] API container healthcheck failed on distroless image**
- **Found during:** Task 3 (stack boot verification)
- **Issue:** `CMD-SHELL` healthcheck requires `/bin/sh`; `gcr.io/distroless/static-debian12` has no shell, no wget. All healthcheck attempts failed with "exec: /bin/sh: no such file"
- **Fix:** Switched runtime stage from `distroless/static-debian12:nonroot` to `debian:bookworm-slim`; added `wget` + `ca-certificates` via apt; preserved nonroot user (UID 65532). The existing `wget -qO- http://localhost:8000/healthz` healthcheck now works
- **Files modified:** `donnarec-api/Dockerfile`
- **Commit:** 694427f

## Verification Results

All checks passed after applying all fixes:

| Check | Result |
|-------|--------|
| `docker compose config -q` | COMPOSE_PARSE_OK |
| realm JSON: no `_` keys, no `maxLoginFailures`, failureFactor=5 | REALM_JSON_OK |
| `docker compose up -d --wait` exit 0 | PASSED |
| postgres: `donnarec_app` + `donnarec_keycloak` both present | KEYCLOAK_DB_OK |
| Keycloak logs: "Import finished successfully" | REALM_IMPORT_OK |
| Keycloak logs: no "Unrecognized field" | NO_UNRECOGNIZED_FIELD |
| All 3 services (postgres, keycloak, api) status=healthy | ALL_HEALTHY |
| `curl http://localhost:8000/healthz` → `{"status":"ok"}` | API_HEALTHY |

## UAT Unblocked

- **Human UAT Test 1** (argon2id verification) is now unblocked
- Keycloak Admin Console accessible at http://localhost:8080
- Realm `donnarec` imported with: passwordPolicy, bruteForceProtected=true, failureFactor=5, 3 roles (maker/checker/admin), 2 clients (backend + frontend)

## Threat Flags

None — no new network endpoints or trust-boundary surface introduced. Changes are infrastructure config only (docker-compose.yml, realm JSON, Dockerfile runtime stage).

## Self-Check: PASSED

Files exist:
- FOUND: donnarec-api/docker-compose.yml (modified)
- FOUND: keycloak/realm-donnarec.json (modified)
- FOUND: donnarec-api/Dockerfile (modified)

Commits exist:
- a45e5c5: fix(01-04): quote DONAREC_KEK, fix scripts path, Keycloak 26 healthcheck
- 9e9a9ec: fix(01-04): remove unsupported _comment keys and maxLoginFailures from realm
- 694427f: fix(01-04): fix OIDC issuer mismatch and API container healthcheck
