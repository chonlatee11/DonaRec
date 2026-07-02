# Conventions

## Integration-test gate (done-criterion for runtime-integration phases)

A phase that touches the **runtime request seam** — HTTP routing, auth middleware
(Keycloak/OIDC token verification), RBAC/route guards, identity resolution, or DB writes
behind those layers — is **NOT "done"** until an **end-to-end integration test** exercises the
real path:

> HTTP request → `RequireAuth` (real token: `sub` / `aud` / `realm_access.roles`) → `RequireRoles` / `ResolveAppUser` → handler → service → DB

driven by a **realistic Keycloak-shaped token** (audience includes the backend client; roles in
`realm_access.roles`; `sub` is a UUID that must resolve to a provisioned `users.id`).

Unit/service tests that construct claims or user rows by hand and call services **directly** do
NOT satisfy this gate — they structurally cannot catch seam defects: audience mismatch,
`RequireRoles` AND-vs-OR misuse, `claims.Subject`-vs-`users.id` identity ([[user-identity-model]]),
or route-guard wiring.

**Why (evidence):** Phase 3 passed 5/5 unit-level verification yet shipped three seam bugs that
only surfaced when the real stack was driven with a real token — `created-by-fk-mismatch` (FK),
`fe-be-audience-mismatch` (aud=account → 401), and an RBAC AND-bug (`RequireRoles(a,b,c)` used
where "any of" was intended → 403 for every user). All three are invisible to isolated unit tests.

**Rule:** Phase verification MUST include this gate before a phase is marked **Complete**. The
gate is satisfied by (a) an automated E2E integration test covering the phase's critical flows via
the real HTTP path, AND (b) the human UI walkthrough (where a UI exists) passing.
