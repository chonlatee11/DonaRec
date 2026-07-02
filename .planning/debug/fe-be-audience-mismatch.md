---
slug: fe-be-audience-mismatch
status: awaiting_human_verify
trigger: "Back-office auth chain never wired: Keycloak tokens issued to the donnarec-frontend client carry aud=account, but the Go backend's OIDC middleware enforces aud must include donnarec-backend, so every UI->API call returns 401 invalid_token. Additionally the frontend NextAuth config requires a confidential client (KEYCLOAK_CLIENT_SECRET) but realm client donnarec-frontend is public, and no web .env.local exists. Blocks the entire Phase 3 UI walkthrough and real back-office usage."
created: 2026-07-02
updated: 2026-07-02T08:35Z
phase: 03-donation-lifecycle-maker-checker-issuance
tdd_mode: true
goal: find_and_fix
---

# Debug Session: fe-be-audience-mismatch

## Symptoms

- **Expected:** A staff user (maker/checker/admin) logs into the Next.js back-office via Keycloak, and the browser's access token is accepted by the Go API so donation CRUD works end to end.
- **Actual:** A real Keycloak access token for `maker1@donarec.test` (obtained via the `donnarec-frontend` client) is rejected by the Go API with HTTP 401 `{"error":"invalid_token"}`. The frontend also cannot complete NextAuth login (client-type mismatch), and has no env config.
- **Error messages:** `POST http://localhost:8000/api/donations` with a valid maker1 bearer token → `401 {"error":"invalid_token"}`.
- **Timeline:** Latent since Phase 3 UI was built; never exercised (Phase 3 verification marked the UI flows `human_needed`). Unrelated to and downstream of the already-fixed+committed `created-by-fk-mismatch` (that fix is in; the 401 happens at token verification, before any FK write).
- **Reproduction:** (stack is up via docker compose; postgres host port remapped to 5433 via docker-compose.override.yml; 4 users seeded incl. maker1@donarec.test / DonaRec123). Temporarily enable `directAccessGrantsEnabled` on the `donnarec-frontend` client (admin API), request a password-grant token for maker1, decode it (aud=account), then `POST /api/donations` with it → 401. Revert direct grants after.

## Evidence (live)

- timestamp: 2026-07-02T07:5xZ
  checked: Decoded a real maker1 access token from client `donnarec-frontend`.
  found: `aud: "account"`, `azp: "donnarec-frontend"`, `realm_access.roles` includes `maker` (correct), `sub` = the seeded KC sub. There is NO `donnarec-backend` in `aud`.
  implication: The realm has no audience mapper adding the backend client to frontend tokens.

- timestamp: 2026-07-02T07:5xZ
  checked: `POST http://localhost:8000/api/donations` with that token.
  found: `HTTP 401 {"error":"invalid_token"}`.
  implication: Go middleware (internal/auth/middleware.go) builds `provider.Verifier(&oidc.Config{ClientID: clientID})` with clientID = KEYCLOAK_CLIENT_ID = `donnarec-backend`, which enforces `aud` contains `donnarec-backend`. aud=account fails → 401 before any handler runs. This is the root blocker.

- timestamp: 2026-07-02T07:5xZ
  checked: keycloak/realm-donnarec.json clients + donnarec-web/lib/auth.ts.
  found: (G2) `donnarec-frontend` is `publicClient: true` (no secret), standardFlow + PKCE. But NextAuth `KeycloakProvider` in lib/auth.ts uses `clientId: KEYCLOAK_CLIENT_ID`, `clientSecret: KEYCLOAK_CLIENT_SECRET!` (non-null) + `issuer: KEYCLOAK_ISSUER` — i.e. expects a CONFIDENTIAL client. (G3) No web `.env.local` exists (only code references to NEXT_PUBLIC_API_BASE_URL default http://localhost:8000, plus KEYCLOAK_CLIENT_ID/SECRET/ISSUER, NEXTAUTH_URL, NEXTAUTH_SECRET).
  implication: Three coupled gaps must all be closed for the walkthrough: G1 audience (aud must include donnarec-backend), G2 client type (confidential + secret, or a public-client NextAuth setup), G3 web env file.

## Current Focus

reasoning_checkpoint:
  hypothesis: "G1 causes the 401: the `donnarec-frontend` client has no Audience protocol mapper, so tokens it issues carry aud=account (the default), not donnarec-backend. Go's oidc verifier is constructed with oidc.Config{ClientID: \"donnarec-backend\"}, which per go-oidc semantics rejects any token whose aud does not contain that value, before any handler/RBAC code runs. G2 (NextAuth expects confidential client, realm has public) and G3 (no .env.local) are separate, non-blocking-for-G1 gaps that independently prevent the browser login flow from ever producing a token in the first place."
  confirming_evidence:
    - "Decoded a real maker1 token from donnarec-frontend: aud=\"account\", azp=\"donnarec-frontend\", no donnarec-backend in aud (Evidence entry 1)."
    - "internal/auth/middleware.go:75 — verifier := provider.Verifier(&oidc.Config{ClientID: clientID}) with clientID=KEYCLOAK_CLIENT_ID=donnarec-backend (.env); go-oidc's Verifier enforces aud contains ClientID unless SkipClientIDCheck is set (it is not) — read directly, not inferred."
    - "POST /api/donations with that exact token -> 401 {\"error\":\"invalid_token\"} (Evidence entry 2), reproduced live, matches the code path (Verify() fails -> generic invalid_token, before claims/RBAC)."
    - "realm-donnarec.json clients[] has no protocolMappers/clientScopes granting audience=donnarec-backend to donnarec-frontend (read full file)."
  falsification_test: "After adding an Audience protocol mapper (or scope) that injects donnarec-backend into aud for donnarec-frontend tokens, decode a freshly issued token: if aud still lacks donnarec-backend, or POST /api/donations with the new token still returns 401, the hypothesis is wrong (e.g. mapper not applied to the right client/scope, or client scope not assigned as default)."
  fix_rationale: "Adding an Audience mapper on the frontend client (or via a dedicated client scope assigned as default) is the standard Keycloak pattern for multi-client aud claims — it fixes the root cause (missing aud entry) rather than a symptom (e.g. weakening the Go verifier's audience check, which would reopen the audience-bypass this code explicitly guards against per its own comments)."
  blind_spots: "Have not yet tested: whether the mapper must be 'access token' scoped (vs ID token) — go-oidc verifies the access token's aud when doing bearer verification here (need to confirm access.token.claim=true). Have not yet tested G2/G3 end-to-end browser login (out of scope per orchestrator; a build/token-endpoint check substitutes). Have not tested realm re-import idempotency of the new JSON blocks."

test: (repro) direct-grant maker1 token -> POST /api/donations -> currently 401 (RED, confirmed). After fix: expect 2xx/non-401 (GREEN).
next_action: Add Audience protocol mapper on donnarec-frontend (live via admin API + persist to realm-donnarec.json), re-test token -> should carry donnarec-backend in aud -> POST /api/donations should stop 401ing. Then make donnarec-frontend confidential (G2), create web env files (G3).

## Eliminated

(none — initial hypothesis for G1/G2/G3 was confirmed correct on first test; no false leads)

## Evidence (fix + verification)

- timestamp: 2026-07-02T08:1xZ
  checked: Added `oidc-audience-mapper` (config: included.client.audience=donnarec-backend, access.token.claim=true, id.token.claim=false) to `donnarec-frontend` live via Keycloak admin API. Minted a fresh maker1 token (temp directAccessGrants, reverted after) and decoded it.
  found: `aud: ["donnarec-backend", "account"]` — donnarec-backend now present.
  implication: G1 mapper works as intended (falsification test from reasoning_checkpoint passed).

- timestamp: 2026-07-02T08:1xZ
  checked: `POST http://localhost:8000/api/donations` with the new token (aud includes donnarec-backend).
  found: `HTTP 403 {"error":"insufficient_role"}` — NOT 401. Verified via code read (internal/auth/rbac.go) that 403/insufficient_role can only be reached after `RequireAuth()` (token verify incl. aud check) succeeds and control passes to `RequireRoles`.
  implication: G1 CONFIRMED FIXED — the audience check that previously produced 401 invalid_token now passes; the request is authenticated and reaches business-logic authorization. The 403 comes from a SEPARATE, pre-existing bug (see "Out-of-scope finding" below), not from the audience mismatch this session targeted.

- timestamp: 2026-07-02T08:1xZ
  checked: cmd/server/main.go:236 `donationGroup.Use(auth.RequireRoles(auth.RoleMaker, auth.RoleChecker, auth.RoleAdmin))` and internal/auth/rbac.go:33-62 `RequireRoles` (doc comment: "passes if and only if... ALL of the specified required roles (logical AND)").
  found: The route comment says "Maker/Checker/Admin — /api/donations (all staff)" (intent: ANY of the 3 roles), but `RequireRoles` implements AND semantics (verified by reading `HasRole` loop — every listed role must be present in `realm_access.roles`, else abort 403). No seeded user (maker1, checker1, admin, makerchecker) holds all three roles simultaneously, so with the current code NO staff user can ever pass this guard for ANY /api/donations route.
  implication: OUT OF SCOPE for this session (not G1/G2/G3, not audience-related) but this is a SEVERE, TOTAL blocker for the exact walkthrough this debug session exists to unblock — donation create/list/etc. will 403 for every real user even after G1/G2/G3 are fixed. Flagged prominently for the human / a follow-up debug session (suggested slug: `rbac-any-role-and-bug`). Root cause: `RequireRoles(...)` needs an "ANY of" (OR) mode for multi-role route guards, distinct from its existing chained-AND usage (adminGroup uses it correctly with a single role). NOT fixed here — deliberately left alone to keep this session's diff minimal and scoped.

- timestamp: 2026-07-02T08:2xZ
  checked: Made `donnarec-frontend` confidential live (publicClient=false, clientAuthenticatorType=client-secret) + regenerated secret via admin API. Requested token without secret, then with secret.
  found: Without secret -> `401 {"error":"unauthorized_client",...}`. With secret -> `200` with valid token (aud still correctly includes donnarec-backend, confirming G1 fix persists alongside G2 change).
  implication: G2 CONFIRMED FIXED — donnarec-frontend now requires the secret NextAuth is configured to send, matching lib/auth.ts's confidential-client assumption.

- timestamp: 2026-07-02T08:2xZ
  checked: Created donnarec-web/.env.local (gitignored — confirmed via `git check-ignore -v`) with the live secret, and donnarec-web/.env.example (no real secrets, to be committed). Ran `npm run build` then a short-lived `next start -p 3001` probe.
  found: Build succeeded, log line "Environments: .env.local" confirms Next.js loaded the file. `GET /api/auth/providers` on the running instance returned 200 with the keycloak provider correctly configured (signinUrl/callbackUrl using NEXTAUTH_URL). Probe server was killed immediately after (pid 2718206; port 3001 confirmed free after).
  implication: G3 CONFIRMED FIXED — env file is present, gitignored, loaded by Next.js, and NextAuth's Keycloak provider initializes without error. Full browser OAuth code+PKCE flow was NOT exercised (requires a browser; deferred to human checkpoint per orchestrator instruction).

- timestamp: 2026-07-02T08:3xZ
  checked: Reverted all temporary admin-API toggles: `directAccessGrantsEnabled` back to `false` on donnarec-frontend (verified via GET after each revert).
  found: `directAccessGrantsEnabled: False` confirmed both times it was toggled back.
  implication: No test-only affordance left enabled; live realm state matches the persisted realm-donnarec.json intent (direct grants off, confidential, audience mapper on).

## Resolution

- root_cause: |
    Three coupled configuration gaps, none of which involved application code:
    G1 (primary/blocking): keycloak/realm-donnarec.json's `donnarec-frontend` client had no
    Audience protocol mapper, so tokens it issued carried `aud: ["account"]` only. Go's
    `internal/auth/middleware.go` builds its OIDC verifier with
    `oidc.Config{ClientID: "donnarec-backend"}`, which requires `aud` to contain that value —
    so every UI-issued token was rejected with 401 invalid_token before reaching any handler.
    G2: donnarec-web/lib/auth.ts's NextAuth KeycloakProvider is configured for a confidential
    client (clientSecret required), but realm-donnarec.json defined donnarec-frontend as
    publicClient:true (no secret) — a client-type mismatch that would prevent NextAuth's
    Keycloak provider from completing token exchange.
    G3: donnarec-web had no .env.local (nor a committed .env.example), so none of
    KEYCLOAK_CLIENT_ID/SECRET/ISSUER, NEXTAUTH_URL/SECRET were set, and the Keycloak provider
    could not initialize at all in a fresh checkout.
  fix: |
    G1: Added an `oidc-audience-mapper` protocol mapper to `donnarec-frontend`
    (included.client.audience=donnarec-backend, access.token.claim=true) — applied live via
    Keycloak admin API AND persisted into keycloak/realm-donnarec.json's client protocolMappers
    so a realm re-import reproduces it.
    G2: Changed `donnarec-frontend` to `publicClient: false` +
    `clientAuthenticatorType: client-secret` in realm-donnarec.json (persisted, no secret value
    committed — Keycloak auto-generates one on import) and applied the same change live,
    regenerating a secret via the admin API. standardFlowEnabled + PKCE (S256) kept as-is.
    Architectural note (per orchestrator instruction to flag this clearly): chose confidential
    over public because donnarec-web's NextAuth runs server-side (Next.js API route), which is
    exactly the case where a confidential client + secret is the OIDC-conventional choice —
    the secret never reaches the browser. A public-client NextAuth setup was not pursued since
    lib/auth.ts already assumed confidential (non-null clientSecret!) and changing that would
    have been a larger, less conventional diff.
    G3: Created donnarec-web/.env.local (gitignored, contains the live secret + a generated
    NEXTAUTH_SECRET) and donnarec-web/.env.example (committed-ready, no real secrets) enumerating
    every env var donnarec-web's app/lib/components code actually reads
    (NEXT_PUBLIC_API_BASE_URL, KEYCLOAK_CLIENT_ID/SECRET/ISSUER, NEXTAUTH_URL/SECRET, and the
    optional NEXT_PUBLIC_CONSENT_TEXT_VERSION).
  verification: |
    RED (pre-fix, prior session): maker1 token (aud=account) -> POST /api/donations -> 401
    invalid_token.
    GREEN (post-fix, this session): maker1 token (aud includes donnarec-backend, minted from the
    now-confidential client with its secret) -> POST /api/donations -> 403 insufficient_role
    (NOT 401) — confirms token verification/audience check now passes; the 403 is a distinct,
    out-of-scope RBAC bug (see Evidence) unrelated to the audience mismatch this session targeted.
    G2 verified directly: token request without client_secret -> 401 unauthorized_client; with
    client_secret -> 200 + valid token.
    G3 verified via `npm run build` (succeeded, loaded .env.local) and a short-lived `next start`
    probe: GET /api/auth/providers -> 200 with correctly configured keycloak provider.
    All temporary admin-API toggles (directAccessGrantsEnabled) reverted to false; live realm
    state now matches persisted realm-donnarec.json.
    NOT verified (deferred to human, per orchestrator instruction — needs a browser): the full
    interactive OAuth authorization-code + PKCE login flow through the Next.js UI, and an actual
    donation created end-to-end through the browser (currently blocked by the separate RBAC
    AND-bug documented above, independent of this session's fix).
  files_changed:
    - keycloak/realm-donnarec.json (donnarec-frontend: publicClient false + clientAuthenticatorType
      client-secret + protocolMappers audience-donnarec-backend added; live Keycloak realm updated
      to match via admin API)
    - donnarec-web/.env.local (new, gitignored — not a git change)
    - donnarec-web/.env.example (new, untracked — ready to commit)
