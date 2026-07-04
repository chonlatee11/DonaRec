---
slug: frontend-auth-gating-missing
status: awaiting_human_verify
trigger: "Visiting http://localhost:3000 loads the DonaRec app directly with no Keycloak login redirect — the back-office frontend has no route protection / forced authentication. Root page is a placeholder, there is no middleware.ts, no SessionProvider, and the configured custom sign-in page (pages.signIn=/auth/signin) does not exist (404), so there is currently NO working way to log in. Blocks the Phase 3 human UI walkthrough."
created: 2026-07-02
updated: 2026-07-02T15:20:00+07:00
phase: 03-donation-lifecycle-maker-checker-issuance
tdd_mode: true
goal: find_and_fix
---

# Debug Session: frontend-auth-gating-missing

## Symptoms

- **Expected:** Visiting the back-office (http://localhost:3000) as an unauthenticated user redirects to Keycloak login; after login, staff pages (donation list/detail, etc.) load and call the API with the bearer token.
- **Actual:** http://localhost:3000 renders the app immediately with no login. There is no forced authentication anywhere, and no working sign-in entry point at all.
- **Error messages:** `GET /api/auth/signin → 302` (redirects to the configured custom page) → `GET /auth/signin → 404` (page does not exist). `GET /api/auth/csrf → 200` (NextAuth core works). `/donations` server component calls `getServerSession` and attaches the token via lib/api.ts, but nothing forces a session, so with no session the API call would 401.
- **Timeline:** Latent since the Phase 3 UI was built page-by-page. NextAuth provider config (lib/auth.ts) and the API bearer wiring (lib/api.ts) exist, but the route-protection / login-gating layer was never built. Backend auth is fully working (E2E test + live curl prove Maker→Checker→issuance through the real API with real tokens).
- **Reproduction:** Stack up (web :3000, Keycloak :8080 with confidential client donnarec-frontend + secret in donnarec-web/.env.local, redirectUris localhost:3000/*, users seeded maker1/checker1/admin/makerchecker @ DonaRec123). Open http://localhost:3000 → lands on the placeholder page, no redirect. Try to log in → no working path (/api/auth/signin → /auth/signin 404).

## Evidence (static)

- donnarec-web/app/page.tsx — root `/` is an explicit placeholder ("Later phases (03-07 / 03-08) will replace this with the donation list"); no auth check. This is the page the user lands on.
- donnarec-web/app/layout.tsx — wraps children in AppShell + NextIntlClientProvider; NO SessionProvider, no auth guard.
- No donnarec-web/middleware.ts (nor src/middleware.ts) — routes are not guarded at the edge.
- donnarec-web/lib/auth.ts — `pages: { signIn: "/auth/signin" }`, but there is no app/auth/signin route → 404. KeycloakProvider is otherwise configured (confidential client; secret in .env.local, matches live Keycloak).
- donnarec-web/app/donations/page.tsx:64 — `getServerSession(authOptions)`; donnarec-web/lib/api.ts:47 — token from `session?.accessToken`, attached as Bearer only if present. No session → no token → API 401.
- donnarec-web/components/AppShell.tsx — nav Links to /donations and /queue; no sign-in / sign-out control.
- Live: `curl GET /api/auth/signin → 302`, `GET /auth/signin → 404`, `GET /api/auth/csrf → 200`.

## Current Focus

- hypothesis: CONFIRMED. See Resolution below.
- test: TDD red confirmed via curl against running dev server (see Resolution.verification). Fix applied. TDD green confirmed via curl against running dev server.
- expecting: n/a — resolved.
- next_action: awaiting human browser-based end-to-end verification (interactive Keycloak login cannot be scripted headlessly per verification_plan constraints).

```yaml
reasoning_checkpoint:
  hypothesis: "The frontend has no route-protection layer (no middleware.ts) and no working sign-in entry (pages.signIn=/auth/signin 404s), so unauthenticated users reach every page and can never establish a NextAuth session."
  confirming_evidence:
    - "curl http://localhost:3000/donations -> HTTP 200, no redirect (unauthenticated, should have been gated)"
    - "curl http://localhost:3000/ -> HTTP 200, no redirect"
    - "curl http://localhost:3000/auth/signin -> HTTP 404 (the exact page lib/auth.ts configures as pages.signIn)"
    - "grep confirmed: no middleware.ts / src/middleware.ts anywhere in donnarec-web; app/page.tsx has no auth check; app/layout.tsx has no SessionProvider or guard"
  falsification_test: "If middleware.ts already existed and requests were still 200, the root cause would instead be a middleware matcher/config bug rather than 'missing gating layer entirely'. It did not exist -> hypothesis holds."
  fix_rationale: "Root cause is structural absence of the gating layer, not a misconfiguration within an existing one. Fix adds the missing layer directly: next-auth/middleware withAuth (edge-verified session redirect) + a working single-provider sign-in page (fixes the 404 pages.signIn was pointing at) + root '/' redirecting into the real app + SessionProvider for the sign-out control. This addresses the mechanism (no gating exists) rather than a symptom (e.g. just fixing the 404 alone would still leave every route open)."
  blind_spots: "Full interactive OAuth round-trip (Keycloak login form -> PKCE callback -> session cookie -> /donations render with real data) is not verified by curl alone (curl doesn't execute JS/handle cookies across the auth-code redirect chain interactively) — this is the documented human checkpoint. Also unverified: /queue route doesn't exist yet as a page (0 files), so its middleware matcher entry is currently inert/future-proofing only, not actively tested."
```

## Eliminated

## Resolution

- root_cause: The frontend never had a route-protection/auth-gating layer built at all — not a misconfiguration of an existing one. Three compounding gaps: (1) no `middleware.ts` anywhere in `donnarec-web`, so Next.js never checks session state before rendering any page — every route (including `/` and `/donations`) served 200 unconditionally; (2) `lib/auth.ts` set `pages: { signIn: "/auth/signin" }` but no `app/auth/signin` route existed, so the one configured entry point 404'd, meaning even a user who *wanted* to log in had no working path; (3) root `/` was an explicit placeholder page with no redirect into the real app. Net effect: no code path in the frontend could ever establish a NextAuth session, and nothing forced one.
- fix: |
    Added donnarec-web/middleware.ts using next-auth/middleware's `withAuth` (edge-compatible, verifies the session JWT via NEXTAUTH_SECRET, no per-request network call to Keycloak) with `pages.signIn: "/auth/signin"` and `matcher: ["/", "/donations/:path*", "/queue/:path*"]` — unauthenticated requests to any matched path now get a 307 to `/auth/signin?callbackUrl=<original path>`.
    Kept `lib/auth.ts`'s `pages.signIn: "/auth/signin"` (did not remove it) and instead created `app/auth/signin/page.tsx` — a minimal client component that reads `callbackUrl` from `window.location.search` in a `useEffect` and immediately calls `signIn("keycloak", { callbackUrl })`, giving a true one-step redirect into Keycloak's hosted login (single-provider app, so skipping NextAuth's generic provider-picker page is the cleaner UX and was explicitly the requested option).
    Changed `app/page.tsx` from the old placeholder to `redirect("/donations")` — since `/` is now itself gated by middleware, only authenticated users ever reach this redirect, making `/donations` the real (protected) landing page.
    Added `components/AuthSessionProvider.tsx` (thin `"use client"` wrapper around NextAuth's `SessionProvider`) and wired it into `app/layout.tsx` around `AppShell`, plus `components/SignOutButton.tsx` (uses `useSession`/`signOut`, renders nothing until `status === "authenticated"`) added to `AppShell`'s header next to `LocaleSwitcher`.
    Did not touch `lib/api.ts`'s `getServerSession`/bearer-token attachment (already correct) and did not touch Keycloak realm config or the Go API.
- verification: |
    TDD RED (pre-fix, live curl against running dev server): `GET /donations` -> 200 no redirect; `GET /` -> 200 no redirect; `GET /auth/signin` -> 404.
    TDD GREEN (post-fix, same running dev server after hot-reload): `GET /donations` -> 307, `location: /auth/signin?callbackUrl=%2Fdonations`; `GET /` -> 307, `location: /auth/signin?callbackUrl=%2F`; `GET /auth/signin` -> 200.
    Additionally verified the sign-in mechanism end-to-end up to the OAuth handoff: fetched a CSRF token from `/api/auth/csrf`, then POSTed to `/api/auth/signin/keycloak` (the exact endpoint `signIn("keycloak")` calls client-side) with `json=true`, and received `{"url":"http://localhost:8080/realms/donnarec/protocol/openid-connect/auth?client_id=donnarec-frontend&scope=openid%20email%20profile&response_type=code&redirect_uri=http%3A%2F%2Flocalhost%3A3000%2Fapi%2Fauth%2Fcallback%2Fkeycloak&state=...&code_challenge=...&code_challenge_method=S256"}` — confirms correct client_id, correct redirect_uri back to the app, and PKCE (code_challenge) all wired correctly.
    `npm run build` succeeds cleanly: compiled successfully, types/lint pass, all 8 routes generated including `/`, `/auth/signin`, `/donations` and children; `Middleware  61.4 kB` compiled as an edge function.
    HUMAN CHECKPOINT REMAINING (not scriptable): full interactive browser login — open http://localhost:3000, confirm redirect to Keycloak's hosted login page, log in as maker1/DonaRec123 (or checker1/admin/makerchecker), confirm the PKCE callback lands back on /donations with real donation data rendering (proves lib/api.ts's bearer-token attachment actually round-trips with the Go API using a real session), and confirm the new sign-out button in the header logs the user out and redirects to /auth/signin.
- files_changed:
  - donnarec-web/middleware.ts (new)
  - donnarec-web/app/auth/signin/page.tsx (new)
  - donnarec-web/app/page.tsx (placeholder -> redirect to /donations)
  - donnarec-web/app/layout.tsx (wrap AppShell in AuthSessionProvider)
  - donnarec-web/components/AuthSessionProvider.tsx (new)
  - donnarec-web/components/SignOutButton.tsx (new)
  - donnarec-web/components/AppShell.tsx (add SignOutButton to header)
