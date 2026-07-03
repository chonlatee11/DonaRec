# Phase 3 Remediation — CONTEXT (frontend integration + TanStack migration)

**Created:** 2026-07-03
**Scope:** Remediation slice under REOPENED Phase 3. NOT the original phase context (see 03-CONTEXT.md for that). Feeds `/gsd-plan-phase 3`.

## Domain

Make the Phase 3 back-office UI actually work end-to-end against the Go API by (a) migrating the frontend data layer to **TanStack Query + TanStack Table** and (b) **aligning the FE↔BE contract**, which was never validated (integration gate / walkthrough surfaced repeated mismatches — bug #5: `GET /api/donations` returns `{"data":[array]}` but the UI expects `{donations,total,page,per_page}` → `DonationTable` crashes on `undefined.length`).

## Locked Decisions (from discuss-phase, 2026-07-03)

### D-R1 — Data-fetching pattern: BFF proxy (Next Route Handlers)
TanStack Query calls **Next.js Route Handlers** (server-side, e.g. `app/api/bff/**`), which obtain the Keycloak access token via `getServerSession(authOptions)` and forward to the Go API. The **access token never reaches the browser** (preserves the current server-side security posture). TanStack Query still provides client cache / refetch / pagination / mutation state.
- Rationale: interactive pagination/filtering (TanStack Table) needs a client-callable endpoint; a BFF proxy gives that without exposing the token or requiring CORS on the Go API.
- Rejected: client-fetch-direct-to-Go (token in browser + CORS); pure server-prefetch+hydrate (can't drive client-side pagination/filter alone).

### D-R2 — API contract / pagination envelope: `{data:{items,total,page,per_page}}`
List endpoints return `{"data": {"items": [...], "total": N, "page": P, "per_page": 20}}`. Backend adds a `COUNT` for `total`. Single-object endpoints keep `{"data": {...}}`. Keep the `data` envelope consistent across all endpoints.
- Backend change: `GET /api/donations` (handler `List` + service `Search`) must return items + total/page/per_page, not a bare array.
- Frontend: `DonationListResponse`/`apiFetch` usage aligns to `data.items` + metadata.

### D-R3 — Scope: ALL Phase 3 screens
Migrate + contract-align every Phase-3 back-office screen in this slice: **list, detail, PII reveal, cancel/void-reissue, create/edit** (and their mutations: approve/return/reject/cancel/reissue/create/update/submit). Audit each screen's FE type vs the real backend response (field names + envelope), not just the list.

## Done-criterion (integration-test gate — see .planning/CONVENTIONS.md)
Not "done" until an automated test drives the real path for the migrated screens' data flows (extend `donnarec-api/cmd/server/e2e_test.go` and/or add a frontend↔BFF↔API contract test), AND the human UI walkthrough passes. The existing E2E test (`TestE2E_MakerCheckerIssuancePipeline`) is the pattern to extend.

## Canonical refs (full paths — MANDATORY for planner/researcher)
- `.planning/CONVENTIONS.md` — integration-test gate (done-criterion)
- `.planning/ROADMAP.md` — Phase 3 (reopened) + criterion 6
- `.planning/phases/03-donation-lifecycle-maker-checker-issuance/03-VERIFICATION.md` — addendum (why reopened)
- `.planning/phases/03-donation-lifecycle-maker-checker-issuance/03-UI-SPEC.md` — UI design contract (screens/layout to preserve)
- Bug trigger: `donnarec-web/app/donations/page.tsx` (reads `result.donations`), `donnarec-web/components/DonationTable.tsx:141`, `donnarec-web/lib/donations.ts` (`DonationListResponse`, `searchDonations`), `donnarec-web/lib/api.ts` (`apiFetch` returns raw `res.json()`, does not unwrap `data`)
- Backend list contract: `donnarec-api/internal/donation/handler.go` (`List` → `c.JSON(200, gin.H{"data": resp})`), `donnarec-api/internal/donation/service.go:1189` (`Search` returns `[]DonationResponse`, no total)

## Code context (reusable assets)
- Frontend auth already wired (this session): `donnarec-web/middleware.ts`, `app/auth/signin/page.tsx`, `components/AuthSessionProvider.tsx` (SessionProvider), `lib/auth.ts`, `lib/api.ts` (`getServerSession` + Bearer). BFF route handlers should reuse `getServerSession(authOptions)` + the existing `API_BASE_URL`.
- Backend response envelope helper pattern: handlers already wrap in `gin.H{"data": ...}`. Add a paginated variant.
- E2E harness: `donnarec-api/cmd/server/e2e_test.go` + `internal/testutil/keycloak.go` (`MintTokenForSubject`) + `setupRouter` (main.go:199).
- TanStack not yet a dependency in `donnarec-web/package.json` — planner adds `@tanstack/react-query` (+ `@tanstack/react-table`) and a `QueryClientProvider` (client) in the layout.

## Deferred ideas
- (none captured — scope intentionally covers all Phase-3 screens)

## Suggested plan granularity (planner decides)
Likely 2–3 plans: (1) backend contract/pagination envelope + total COUNT + tests; (2) frontend BFF route handlers + TanStack Query/Table provider + migrate list; (3) migrate detail/reveal/cancel/create screens + contract tests + extend E2E. New plan numbers under Phase 3 (03-09+).
