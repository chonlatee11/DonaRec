---
phase: 01-foundation-db-auth-rbac-audit-retention-model
reviewed: 2026-06-25T20:00:00Z
depth: standard
files_reviewed: 34
files_reviewed_list:
  - donnarec-api/cmd/server/main.go
  - donnarec-api/internal/audit/middleware.go
  - donnarec-api/internal/audit/service.go
  - donnarec-api/internal/auth/claims.go
  - donnarec-api/internal/auth/middleware.go
  - donnarec-api/internal/auth/rbac.go
  - donnarec-api/internal/config/config.go
  - donnarec-api/internal/crypto/aes_gcm.go
  - donnarec-api/internal/crypto/envelope.go
  - donnarec-api/internal/crypto/envprovider.go
  - donnarec-api/internal/crypto/keyprovider.go
  - donnarec-api/internal/db/helpers.go
  - donnarec-api/internal/pii/mask.go
  - donnarec-api/internal/retention/service.go
  - donnarec-api/internal/users/handler.go
  - donnarec-api/internal/users/service.go
  - donnarec-api/migrations/000001_init_schema.up.sql
  - donnarec-api/migrations/000002_audit_log.up.sql
  - donnarec-api/migrations/000003_retention_triggers.up.sql
  - donnarec-api/migrations/000001_init_schema.down.sql
  - donnarec-api/migrations/000002_audit_log.down.sql
  - keycloak/realm-donnarec.json
  - scripts/seed-admin.sh
  - scripts/create-multiple-dbs.sh
  - donnarec-api/docker-compose.yml
  - donnarec-api/.env.example
  - donnarec-api/internal/db/queries/audit.sql
  - donnarec-api/internal/db/queries/users.sql
  - donnarec-api/internal/auth/middleware_integration_test.go
findings:
  critical: 3
  warning: 9
  info: 7
  total: 19
status: issues_found
---

# Phase 1: Code Review Report

**Reviewed:** 2026-06-25 (01-05 gap-closure pass added; earlier findings retained)
**Depth:** standard
**Files Reviewed:** 34 (28 original + 6 from 01-05 gap-closure)
**Status:** issues_found

## Summary

Phase 1 establishes the DonaRec Go foundation: config, envelope crypto, OIDC auth, RBAC,
hash-chained audit log, PDPA retention guards, and the users domain. The crypto package
(AES-256-GCM, envelope DEK/KEK, blind index, KEK-from-env) is well-built and matches the
CLAUDE.md spec. The audit hash-chain concurrency design (advisory xact lock instead of
FOR-UPDATE-on-empty-table) is correct and well-reasoned, and the immutability/concurrency
tests are meaningful (not coverage theater).

The 01-05 gap-closure changes correctly add the migrate init-service to docker-compose.yml
and make the OIDC expected issuer configurable via `OIDC_ISSUER` env using
`oidc.InsecureIssuerURLContext`. Security-critical checks confirm: no `SkipIssuerCheck`,
`SkipClientIDCheck`, or `SkipExpiryCheck` flags are present; signature (JWKS), audience, and
expiry enforcement are intact. No new critical/blocker security issues were introduced by
01-05.

New issues surfaced in 01-05 scope: two warnings and two info items. The most operationally
significant is that the migrate service command leaves `DB_PASSWORD` without a required-guard
(`${DB_PASSWORD}` instead of `${DB_PASSWORD:?}`) inconsistent with all other services.

Previously found blockers (CR-01 through CR-03) from the 2026-06-24 review are retained unchanged.

## Critical Issues

### CR-01 (BLOCKER): No `.gitignore` anywhere — `.env` with credentials will be committed

**File:** `(repository root — missing .gitignore)`, working-tree `donnarec-api/.env`
**Issue:** There is no `.gitignore` at the repo root nor in `donnarec-api/`. A working-tree
`donnarec-api/.env` exists and contains `DB_PASSWORD=changeme`, `KC_ADMIN_PASSWORD=changeme`,
`KC_DB_PASSWORD=changeme`, and `DONAREC_KEK=...`. The file is currently untracked only by luck;
the first `git add .` / `git add donnarec-api` will stage and commit live credentials and the
KEK placeholder. For a PDPA-critical system whose entire encryption model rests on the KEK never
reaching the repo (envprovider.go anti-pattern note: "KEK in env (not DB) so DB backups cannot
reveal the KEK"), this is a direct violation of the threat model.
**Fix:** Add a `.gitignore` (root) before any further commits:
```gitignore
# secrets / local env
.env
*.env
!*.env.example
# build artifacts
donnarec-api/bin/
*.test
```
Also confirm `.env` has never been committed (`git log --all -- donnarec-api/.env`) and rotate
`changeme` credentials before any shared environment.

### CR-02 (BLOCKER): SQL injection / shell injection in `seed-admin.sh` psql heredoc

**File:** `scripts/seed-admin.sh:123-145`
**Issue:** The `psql ... <<-SQL` heredoc interpolates shell variables directly into SQL text:
`'$ADMIN_EMAIL'`, `'$ADMIN_DISPLAY_NAME'`, `'$KC_USER_ID'`. Any single quote, backslash, or SQL
metacharacter in those env-supplied values breaks out of the string literal and injects SQL
(e.g. `ADMIN_DISPLAY_NAME="O'Brien'); DROP TABLE users;--"`). The same values are also injected
unescaped into the Keycloak JSON body (lines 71-83) where a `"` in a value corrupts/injects JSON.
Even if "admin-controlled," this is unsafe input handling on a bootstrap path that runs as a
privileged DB user.
**Fix:** Pass values as bound parameters via psql variables instead of string interpolation:
```bash
psql "$DATABASE_URL" \
  -v email="$ADMIN_EMAIL" \
  -v name="$ADMIN_DISPLAY_NAME" \
  -v kcid="$KC_USER_ID" <<'SQL'
INSERT INTO users (email, display_name, keycloak_subject, is_active, legal_hold)
VALUES (:'email', :'name', :'kcid', true, false)
ON CONFLICT (keycloak_subject) DO UPDATE
    SET email = EXCLUDED.email, display_name = EXCLUDED.display_name, updated_at = now();
SQL
```
For the Keycloak JSON, build the body with `jq -n --arg email "$ADMIN_EMAIL" ...` rather than
string interpolation.

### CR-03 (BLOCKER): Auth middleware uses the ID-token verifier for a `bearerOnly` access-token backend

**File:** `donnarec-api/internal/auth/middleware.go:49,75`; `keycloak/realm-donnarec.json:63`
**Issue:** `provider.Verifier(&oidc.Config{ClientID: clientID})` returns an `*oidc.IDTokenVerifier`,
and `m.verifier.Verify(...)` is intended for **ID tokens**. The `donnarec-backend` client is
`bearerOnly: true`, so the API receives Keycloak **access tokens** from the frontend, not ID
tokens. Keycloak access tokens place the audience differently: by default `aud` is set to the
service the token was issued *to* (often the frontend client or `account`), while the backend's
identity appears in `azp`/`resource_access`, not necessarily in `aud`. With
`oidc.Config{ClientID: "donnarec-backend"}`, real frontend-issued access tokens may fail the
audience check (locking everyone out), or — if an audience mapper is later added loosely — the
check may pass for tokens never intended for this API. Either way the audience guard
(Pitfall 3, the stated reason for setting ClientID) is not validated against the actual token type.
Additionally, `email` is frequently absent from Keycloak access tokens unless an explicit mapper
adds it, which would silently break `actor_email` in the audit trail (FR-13).
**Fix:** Confirm the token type the API will actually receive and align the verifier:
- If access tokens: configure an audience mapper in Keycloak so access tokens carry
  `aud: ["donnarec-backend"]`, and add an integration test that verifies a real Keycloak-issued
  access token (not a hand-rolled ID token) passes `Verify`. Set `SkipClientIDCheck` only with a
  manual `aud`/`azp` check if you cannot add the mapper.
- Ensure an `email` protocol mapper is enabled for the access token, or read identity from a
  claim guaranteed to be present (`preferred_username`).
- Add a negative test: a token with `aud` not containing `donnarec-backend` must be rejected.

## Warnings

### WR-01 (WARNING): Audit own-tx path violates the stated "same transaction as mutation" invariant

**File:** `donnarec-api/internal/audit/middleware.go:95`, `service.go:225-233`,
`donnarec-api/internal/users/service.go:64-127`
**Issue:** Both `service.go` and CLAUDE.md (Foundational Rule 2) state audit entries must be
written *in the same transaction as the data mutation* ("ห้ามเขียน audit entry ใน goroutine แยก —
ต้องเขียนใน transaction เดียวกับ data mutation"). The actual wiring uses
`AuditMiddleware → AppendAuditEntry` (own separate transaction) *after* `c.Next()` returns, i.e.
after `CreateUser` has already committed its own `WithTx`. Result: a user can be created and
committed while the audit write fails (middleware logs and explicitly does NOT abort). That is a
silent audit gap on a legally-significant action, contradicting the invariant the code claims to
uphold. The `AppendAuditEntryTx` (in-tx) method exists but is never called by any handler in this
phase.
**Fix:** For mutating handlers, perform the audit insert inside the same `WithTx` as the
mutation via `AppendAuditEntryTx(ctx, tx, entry)` (have the service own the audit write), or
document explicitly that Phase 1 accepts best-effort post-commit auditing and downgrade the
Rule-2 language. Do not leave the code asserting an invariant it does not enforce.

### WR-02 (WARNING): `AssignRole` ON CONFLICT DO NOTHING + `:one` RETURNING errors on conflict

**File:** `donnarec-api/internal/db/queries/users.sql:46-50`,
`donnarec-api/internal/users/service.go:103-112`
**Issue:** `AssignRole` is `:one` with `ON CONFLICT (user_id, role) DO NOTHING RETURNING ...`.
On a conflict, `DO NOTHING` produces zero rows, so sqlc's `:one` `row.Scan` returns
`pgx.ErrNoRows`. In `CreateUser` that error is wrapped and returned, aborting the whole
transaction. The query comment and service comment both claim this is "idempotent," but the
idempotency is broken: re-assigning an existing role fails instead of being a no-op. New-user
creation does not hit this today (no pre-existing rows), but any future re-assign/update path will.
**Fix:** Either make it `:exec` (drop RETURNING) and stop relying on a returned row, or treat
`pgx.ErrNoRows` from `AssignRole` as success:
```go
if _, err := qtx.AssignRole(ctx, ...); err != nil && !errors.Is(err, pgx.ErrNoRows) {
    return fmt.Errorf("assign role %q: %w", role, err)
}
```

### WR-03 (WARNING): `extractBearerToken` is scheme-case-sensitive and allows empty tokens

**File:** `donnarec-api/internal/auth/middleware.go:107-113`
**Issue:** `strings.HasPrefix(h, "Bearer ")` rejects `bearer <token>` / `BEARER <token>`, which
RFC 6750/7235 treat as case-insensitive. More importantly, `TrimPrefix("Bearer ", ...)` of a
header that is literally `"Bearer "` (trailing space, no token) yields `""` which is then handled
as missing — acceptable — but a header `"Bearer    "` (multiple spaces) yields a whitespace token
passed to `Verify`. Minor robustness issue, not a bypass.
**Fix:** Case-insensitively match the scheme and trim/validate the remainder:
```go
const p = "bearer "
if len(h) < len(p) || !strings.EqualFold(h[:len(p)], p) { return "" }
return strings.TrimSpace(h[len(p):])
```
Note: The 01-05 revision of `middleware.go` already implements this fix (lines 139-144). This
warning applies only if an older version of the file is used.

### WR-04 (WARNING): `MaskNationalID` non-13-length branch can leak more than intended; format comment is self-contradictory

**File:** `donnarec-api/internal/pii/mask.go:59-97`
**Issue:** Two concerns. (1) The doc block (lines 80-90) contains a long stream-of-consciousness
note about a test assertion that "FAILS when we insert a dash" — leftover design narration that
should not ship in a security-sensitive PII function; it makes the intended format ambiguous.
(2) For non-standard lengths the function reveals exactly the last 4 chars regardless of total
length, but for inputs of length 5–12 (e.g. a partially-entered or malformed ID) this still
exposes 4 of very few digits. For a 5-char value it masks only 1 char. Given this is the PDPA
masking primitive, the "reveal last 4" rule should be bounded so short/odd values do not leak a
disproportionate fraction.
**Fix:** Replace the narration with a one-line format spec. Consider masking everything when
`len(full) < 10` (not a plausible full national ID) rather than revealing last-4 of a short value.

### WR-05 (WARNING): Dead/unused `InsertAuditLog` sqlc query diverges from the real insert path

**File:** `donnarec-api/internal/db/queries/audit.sql:5-30`, `service.go:191-212`
**Issue:** The hand-written `InsertAuditLog` sqlc query is never used — `AppendAuditEntryTx`
issues a raw `tx.Exec` INSERT that *also sets `id` and `created_at` explicitly* (the sqlc query
omits both, relying on defaults). Having two divergent insert definitions for the immutable audit
table is a maintenance trap: a future dev may "fix" the unused query or switch to it and silently
change the hash-chain inputs (the raw path's explicit `id`/`created_at` are load-bearing for the
hash). Dead code on the most security-sensitive table.
**Fix:** Delete `InsertAuditLog` from `audit.sql` (and regenerate), or convert the service to use
it — but only if it is amended to set the reserved `id` and captured `created_at`, since both feed
`computeRowHash`. Keep exactly one insert definition.

### WR-06 (WARNING): `down.sql` migrations restore UPDATE/DELETE and DROP the audit role/table

**File:** `donnarec-api/migrations/000002_audit_log.down.sql:6-29`
**Issue:** The down migration grants `UPDATE, DELETE ON audit_log` back to `donnarec_app` and
then drops the audit_log table and the role. While "down" is expected to reverse, on a shared
cluster `DROP ROLE donnarec_app` can hit dependency errors (it owns/has grants on `users`,
`user_roles`, sequences from 000002), and momentarily re-granting UPDATE/DELETE on the audit table
opens a tamper window if `down` is ever run against a populated prod DB. The header even warns
"irreversibly destroys all audit trail data."
**Fix:** Guard destructive down-migrations against non-dev environments (or split into a separate
explicitly-opt-in teardown), and revoke the audit grants before re-granting nothing — do not
re-enable UPDATE/DELETE just to drop. At minimum, document that 000002 down must never run in prod.

### WR-07 (WARNING): migrate service `DB_PASSWORD` has no required-guard — silent empty password allowed

**File:** `donnarec-api/docker-compose.yml:121`
**Issue:** The migrate service command embeds the database connection string directly in the
`command` array:
```yaml
"-database", "postgres://${DB_USER:-donnarec}:${DB_PASSWORD}@postgres:5432/donnarec_app?sslmode=disable",
```
`${DB_PASSWORD}` has no `:?` error-guard, so if `DB_PASSWORD` is unset or empty, Docker Compose
silently substitutes an empty string. The migration will attempt a connection with an empty
password. By contrast, all other services that use `DB_PASSWORD` include the required guard:
- `postgres` line 24: `${DB_PASSWORD:?DB_PASSWORD is required ...}`
- `keycloak` line 64: `${DB_PASSWORD:?DB_PASSWORD is required}`
- `api` line 136: `${DB_PASSWORD:?}`

This inconsistency means a misconfigured `.env` that is missing `DB_PASSWORD` will silently fail
in the migrate service rather than producing the clear compose-parse error that the other services
generate.
**Fix:** Add the required guard consistently:
```yaml
"-database", "postgres://${DB_USER:-donnarec}:${DB_PASSWORD:?DB_PASSWORD is required}@postgres:5432/donnarec_app?sslmode=disable",
```

### WR-08 (WARNING): `DB_PASSWORD` in migrate command not URL-encoded — connection fails with special characters

**File:** `donnarec-api/docker-compose.yml:121`
**Issue:** The migrate `command` array builds a Postgres connection DSN by direct string
interpolation of `${DB_PASSWORD}`. If `DB_PASSWORD` contains URL-special characters (`@`, `/`,
`?`, `#`, `:`), the resulting DSN is malformed and the connection will fail with a parse error
rather than an auth error. The `api` service avoids this because `pgxpool.New` accepts a DSN
string and `pgx` is lenient, but `golang-migrate`'s DSN parser is strict about URL structure.
The `postgres` and `keycloak` services are not affected because they pass the password via
separate env vars, not embedded in a URL.
**Fix:** Use a URL-encoded password via `$()` shell substitution, or switch the migrate service to
use the `PGPASSWORD` env var form with `postgresql://` DSN that only URL-encodes by convention.
A simpler workaround is to set a separate `MIGRATE_DATABASE_URL` variable in `.env` with the
password already URL-percent-encoded and reference it in compose:
```yaml
"-database", "${MIGRATE_DATABASE_URL:?MIGRATE_DATABASE_URL is required}",
```
Document in `.env.example` that special characters in `DB_PASSWORD` require URL-encoding in
`MIGRATE_DATABASE_URL`.

### WR-09 (WARNING): OIDC_ISSUER / KC_HOSTNAME_URL dependency on Keycloak 26 non-strict hostname behavior is undocumented

**File:** `donnarec-api/docker-compose.yml:72-74, 143`
**Issue:** The working OIDC flow depends on a Keycloak 26-specific behavior:
`KC_HOSTNAME_STRICT=false` causes Keycloak to reflect the request's `Host` header as the `iss`
claim in issued tokens, instead of using `KC_HOSTNAME_URL` (`http://keycloak:8080`). This is why
browser-requested tokens carry `iss=http://localhost:8080/realms/donnarec` and match the
`OIDC_ISSUER` default. The compose comment on lines 67-71 does not explain this mechanism.

The risk is:
1. **Version drift**: upgrading to a Keycloak version that changes `KC_HOSTNAME_STRICT=false`
   semantics silently breaks all token verification (all requests return 401).
2. **Config drift**: setting `KC_HOSTNAME_STRICT=true` (a reasonable hardening step) silently
   locks out all users because tokens would then carry `iss=http://keycloak:8080/realms/donnarec`
   which no longer matches `OIDC_ISSUER=http://localhost:8080/...`.
3. **Production use**: in production `KC_HOSTNAME_STRICT=false` should be `true`; at that point
   `OIDC_ISSUER` must equal `KC_HOSTNAME_URL` (the canonical domain), not the browser-access URL.
   The current `.env.example` value `http://localhost:8080/realms/donnarec` would be incorrect.

**Fix:** Add a comment in docker-compose.yml explaining the dependency:
```yaml
# KC_HOSTNAME_STRICT=false: in Keycloak 26, when strict mode is off, the issuer
# in issued tokens follows the request's Host header. This means browser requests
# via localhost:8080 produce tokens with iss=http://localhost:8080/realms/donnarec,
# matching OIDC_ISSUER. For production, set KC_HOSTNAME_STRICT=true and align
# KC_HOSTNAME_URL and OIDC_ISSUER to the same canonical public domain.
```
Also add a production note in `.env.example` under `OIDC_ISSUER` warning that when
`KC_HOSTNAME_STRICT=true` (production), `OIDC_ISSUER` must equal the `KC_HOSTNAME_URL` value.

## Info

### IN-01 (INFO): `created_at` written twice on user insert (query `now()` + column default)

**File:** `donnarec-api/internal/db/queries/users.sql:5-22`, `migrations/000001_init_schema.up.sql:32-33`
**Issue:** `CreateUser` explicitly sets `created_at = now(), updated_at = now()` while the columns
already `DEFAULT now()`. Harmless and slightly redundant; risk is only that the two sources of
truth could drift if the column default changes.
**Fix:** Drop `created_at`/`updated_at` from the INSERT column list and rely on defaults, or keep
explicit and remove the defaults — pick one convention.

### IN-02 (INFO): `retention_config` seed uses zero-UUID `updated_by` with a NOT NULL FK-less column

**File:** `donnarec-api/migrations/000001_init_schema.up.sql:47-72`
**Issue:** `updated_by UUID NOT NULL` is seeded with `'00000000-...-000000000000'` as a sentinel,
but there is no FK to `users(id)`, so the sentinel is never validated and could persist forever if
`seed-admin.sh` is not run. Not a bug, but the integrity intent ("must be replaced by admin seed")
is unenforced.
**Fix:** Add a FK `REFERENCES users(id)` (and seed after admin exists) or document that the
sentinel is intentional and acceptable.

### IN-03 (INFO): `singularize` mishandles words ending in "s" that are not plural

**File:** `donnarec-api/internal/audit/middleware.go:180-191`
**Issue:** `singularize("status")` → "statu", `singularize("address")` → "addres". The comment
even acknowledges these exceptions but the code does not handle them. Affects only the derived
audit `action` string label (e.g. `statu.read`), not correctness of the chain, but produces
confusing audit labels if such a route noun ever appears.
**Fix:** Add an exception set, or derive the action noun from an explicit route→action map rather
than heuristic singularization (more robust for an audit label that auditors will read).

### IN-04 (INFO): `.env.example` ships `sslmode=disable` for a PDPA/TLS-mandated system

**File:** `donnarec-api/.env.example:14`, `docker-compose.yml:136`
**Issue:** CLAUDE.md mandates TLS `verify-full` to Postgres as a baseline. The example and compose
default to `sslmode=disable`. Acceptable for local Docker, but the example is the template most
people copy; shipping `disable` invites it into staging/prod.
**Fix:** Comment the example to require `sslmode=verify-full` (with CA path) outside local dev, and
add a startup warning if `sslmode=disable` is detected with a non-localhost host.

### IN-05 (INFO): Concurrency test asserts "no duplicate prev_hash" which is implied, not the strongest invariant

**File:** `donnarec-api/internal/audit/concurrent_test.go:67-85`
**Issue:** The test checks unique `prev_hash` + row count + `VerifyChain`, which is good. It does
not, however, assert that the chain is a single unbroken linked list (each row's `prev_hash`
equals the previous row's `row_hash` in id order) independent of `VerifyChain`'s own recomputation
— so a bug shared between insert and verify could pass both. Minor: a second independent linkage
assertion would harden the most important invariant in the system.
**Fix:** Add a SQL self-join asserting `cur.prev_hash = prev.row_hash` for consecutive ids, as an
oracle independent of the Go hash function.

### IN-06 (INFO): Dead code in middleware_integration_test.go — unused context variable

**File:** `donnarec-api/internal/auth/middleware_integration_test.go:146-147`
**Issue:** `TestNewAuthMiddleware_InvalidProvider` contains:
```go
ctx := context.Background()
_ = ctx
```
The variable is created and immediately discarded. It appears to be a remnant of the signature
change in 01-05 where a context parameter was removed from `NewAuthMiddleware`. The `context`
import on line 4 exists solely for this dead code.
**Fix:** Remove lines 146-147 and the `"context"` import. Run `go vet` / `golangci-lint` to
catch this class of issue before review.

### IN-07 (INFO): Test "expired token returns 401" does not test expiry — tests malformed token

**File:** `donnarec-api/internal/auth/middleware_integration_test.go:74-83`
**Issue:** The test is named `"expired token returns 401"` but its implementation sends
`"Bearer not.a.real.jwt.token"` — a structurally invalid JWT. The test comment acknowledges:
"We cannot easily create a truly expired token via the test server without time mocking." The
token expiry code path (go-oidc's expiry check) is therefore untested. In a PDPA system where
session lifetime is a compliance control, an untested expiry check is a gap.
**Fix:** Either rename the test to `"malformed token returns 401"` (accurate description) and
add a separate expiry test using time-injection (e.g., mint a token with `exp = time.Now().Unix() - 1`
and ensure the verifier rejects it), or use a test clock via the `oidc.Config.Now` field if
available.

---

_Original review: 2026-06-24_
_01-05 gap-closure pass: 2026-06-25_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
