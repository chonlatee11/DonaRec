---
phase: 03-donation-lifecycle-maker-checker-issuance
plan: 12
subsystem: frontend-data-layer
tags: [tanstack-query, bff-proxy, nextjs, vitest, contract-alignment, pii-reveal]

# Dependency graph
requires:
  - phase: 03-donation-lifecycle-maker-checker-issuance
    provides: "DonationDetailResponse contract + server-computed auth flags (03-11); BFF proxy pattern + lib/bff.ts bffForward + TanStack provider (03-10)"
provides:
  - "BFF Route Handlers for donation detail (with server-composed slip_url), PII reveal (donor_tax_id→national_id mapping), and approve/return/reject mutations — all under app/api/bff/donations/[id]/**"
  - "Client DonationDetailView driving Screen 3 + Screen 4 via useQuery + useMutation; ReviewActionPanel/MaskedIdField consume it unchanged"
  - "First automated test suite in donnarec-web (Vitest) — hermetic route-handler tests covering the BFF trust boundary (token forwarding, 401 gate, field mapping, slip_url composition, no-token-leak)"
affects: [03-13 (create/edit/cancel/reissue/slip BFF migration — reuses the [id] BFF dir and bffForward pattern established here)]

# Tech tracking
tech-stack:
  added:
    - "vitest ^4.1.9 (devDependency) — first test runner in donnarec-web"
  patterns:
    - "BFF route composing a second server-side Go call (detail + /:id/slip) before responding — bffForward is safe to call twice on the same GET request since it only reads request.text() for non-GET/HEAD methods"
    - "Client-side BFF error mapping (lib/donations.ts mapBffError) mirrors the server-side apiFetch switch (403 sod/forbidden, 409 statusConflict, 422 validation) so ReviewActionPanel/MaskedIdField's existing error-shaped callback contracts (Promise<{error}|null>) needed zero changes when their data source moved from Server Actions to TanStack mutations"
    - "vitest.config.ts mirrors tsconfig's @/* path alias so BFF route files under app/api/bff/** resolve normally in tests"

key-files:
  created:
    - donnarec-web/app/api/bff/donations/[id]/route.ts
    - donnarec-web/app/api/bff/donations/[id]/pii/route.ts
    - donnarec-web/app/api/bff/donations/[id]/approve/route.ts
    - donnarec-web/app/api/bff/donations/[id]/return/route.ts
    - donnarec-web/app/api/bff/donations/[id]/reject/route.ts
    - donnarec-web/app/api/bff/donations/__tests__/bff-routes.test.ts
    - donnarec-web/vitest.config.ts
    - donnarec-web/components/DonationDetailView.tsx
  modified:
    - donnarec-web/lib/donations.ts
    - donnarec-web/app/donations/[id]/page.tsx
    - donnarec-web/package.json
    - donnarec-web/package-lock.json

key-decisions:
  - "Cancel/reissue (issued-receipt actions) were kept as inline Server Actions in app/donations/[id]/page.tsx, passed down as props to DonationDetailView, rather than migrated to BFF/TanStack — 03-12's explicit files_modified list and interfaces section only cover detail/pii/approve/return/reject; 03-13's files_modified list explicitly owns cancel/reissue/create/update/submit/slip BFF routes and CancelDialog.tsx. Keeping the server-action pattern here means zero functional regression for the issued-receipt Checker/Admin flow while staying inside 03-12's declared scope."
  - "ReviewActionPanel.tsx and MaskedIdField.tsx needed NO code changes despite being listed in the plan's files_modified — their existing Promise<{error}|null> and Promise<{national_id}|{error}> callback contracts (originally written for Server Actions) are structurally identical to what a TanStack-mutation wrapper produces, so DonationDetailView's handleApprove/handleReturn/handleReject/handleRevealPII wrappers satisfy those contracts without touching either component."
  - "getDonation (server-side, apiFetch-based) was kept alongside the new client-side fetchDonation rather than replaced, because app/donations/[id]/edit/page.tsx (out of 03-12 scope) still depends on it to seed the edit form's initial values."
  - "detail BFF route calls bffForward twice on the same incoming GET request (once for /api/donations/:id, once for /:id/slip) instead of duplicating token/session lookup logic — verified safe because bffForward only calls request.text() for non-GET/HEAD methods, so the request body is never consumed by the first call."

requirements-completed: [FR-29, FR-14, FR-12]

# Metrics
duration: ~35min
completed: 2026-07-03
---

# Phase 3 Plan 12: Frontend Detail/Review Slice — BFF + TanStack Migration Summary

**Detail/review screen (Screen 3) and audited PII reveal (Screen 4) now run on live Go-API data through five new BFF Route Handlers and a client `DonationDetailView` (useQuery + useMutation), with a Vitest test suite proving the token-forwarding/field-mapping/slip-composition trust boundary — zero changes needed to `ReviewActionPanel` or `MaskedIdField`.**

## Performance

- **Duration:** ~35 min
- **Tasks:** 2 (plus a pre-approved package-legitimacy checkpoint)
- **Files modified:** 12 (8 created, 4 modified)

## Accomplishments

- `app/api/bff/donations/[id]/route.ts` GET: forwards to Go `/api/donations/:id`, then composes `slip_url` server-side by also calling Go `/:id/slip` (200→url, 404→null) — the browser never calls the presigned-URL endpoint directly (T-12-04).
- `app/api/bff/donations/[id]/pii/route.ts` GET: forwards to Go `/:id/pii` (checker/admin-gated, audited server-side) and renames `donor_tax_id`→`national_id` to match the FE contract (T-12-01).
- `approve`/`return`/`reject` BFF routes: thin `bffForward` proxies passing through Go 403 (sod/insufficient_role), 409 (status conflict), and 422 (missing_reason) unchanged (T-12-02).
- All five routes obtain the Keycloak token server-side via `bffForward`/`getServerSession` — the access token never reaches the browser (T-12-03), proven by an explicit no-token-leak test.
- Added Vitest (pre-approved package-legitimacy checkpoint — official `vitest-dev/vitest` package) as the project's first test runner, with `vitest.config.ts` mirroring the `@/*` path alias, and 8 hermetic route-handler tests (mocked `getServerSession` + `fetch`, no Docker/network) covering Bearer forwarding, the 401-without-Go-call gate, PII field mapping, slip_url composition (both 200 and 404 branches), and the token-leak guard.
- `components/DonationDetailView.tsx` (new client component): `useQuery(["donation", id])` drives the record; `useMutation` wraps approve/return/reject through the BFF and invalidates the detail query on success so the panel updates without a full page reload.
- `lib/donations.ts`: `fetchDonation`/`approve`/`returnForEdit`/`reject`/`revealPII` now call the client BFF routes with a local error mapper (`mapBffError`) that mirrors `apiFetch`'s 403/409/422 handling — so `ReviewActionPanel` and `MaskedIdField`'s existing `Promise<{error}|null>`-shaped callback contracts needed **zero code changes**.
- `app/donations/[id]/page.tsx` reduced to a thin server shell rendering `<DonationDetailView id .../>`; cancel/reissue stay as inline Server Actions passed down as props (unaffected — out of 03-12's scope, migrates in 03-13).
- SoD blocked state, review-panel branching, receipt/status/consent/review-history rendering, and the two-column UI-SPEC Screen 3 layout were ported verbatim into the client view.

## Task Commits

1. **Task 1: BFF routes for detail (compose slip_url), PII reveal, approve/return/reject + route-handler test** - `0de0f2f` (feat)
   - Five new BFF route files under `app/api/bff/donations/[id]/**`
   - `vitest` devDependency + `vitest.config.ts` + `npm run test` script
   - `app/api/bff/donations/__tests__/bff-routes.test.ts` — 8/8 tests passing
   - Verified: `npx tsc --noEmit` clean, `npm run test` 8/8 passing
2. **Task 2: Client DonationDetailView (useQuery + useMutation) + aligned types; preserve UI-SPEC branching** - `6ceb135` (feat)
   - `components/DonationDetailView.tsx` created
   - `lib/donations.ts` client fetchers/mutations added
   - `app/donations/[id]/page.tsx` reduced to a server shell
   - Verified: `npx tsc --noEmit && npm run build` clean; all 5 BFF routes registered in the build output

**Plan metadata:** (this commit) `docs(03-12): complete donation detail/review frontend slice plan`

## Files Created/Modified

- `donnarec-web/app/api/bff/donations/[id]/route.ts` — GET detail proxy composing `slip_url`
- `donnarec-web/app/api/bff/donations/[id]/pii/route.ts` — GET PII reveal proxy, `donor_tax_id`→`national_id`
- `donnarec-web/app/api/bff/donations/[id]/approve/route.ts` — POST approve proxy
- `donnarec-web/app/api/bff/donations/[id]/return/route.ts` — POST return proxy
- `donnarec-web/app/api/bff/donations/[id]/reject/route.ts` — POST reject proxy
- `donnarec-web/app/api/bff/donations/__tests__/bff-routes.test.ts` — 8 hermetic BFF trust-boundary tests
- `donnarec-web/vitest.config.ts` — node environment + `@/*` alias
- `donnarec-web/components/DonationDetailView.tsx` — client detail/review view (useQuery + useMutation)
- `donnarec-web/lib/donations.ts` — client BFF fetchers/mutations (`fetchDonation`, `approve`, `returnForEdit`, `reject`, `revealPII`) + `mapBffError`; `getDonation` (server-side) kept for the edit page
- `donnarec-web/app/donations/[id]/page.tsx` — thin server shell; cancel/reissue Server Actions preserved
- `donnarec-web/package.json` / `package-lock.json` — `vitest` devDependency + `test` script

## Decisions Made

See `key-decisions` in frontmatter: cancel/reissue deferred to 03-13 (explicit scope boundary), `ReviewActionPanel`/`MaskedIdField` left untouched (contracts already compatible), `getDonation` kept alongside the new `fetchDonation` (edit page dependency), and the double-`bffForward`-call pattern for slip_url composition (safe on GET requests).

## Deviations from Plan

None — plan executed as written. The plan's own `files_modified` list included `ReviewActionPanel.tsx` and `MaskedIdField.tsx`, but on inspection their existing prop contracts already matched what the new client mutation wrappers produce, so no edits were needed there (documented as a key-decision, not a deviation, since the plan's own action text said "preserve... untouched").

## Checkpoints

- **Task 0 (checkpoint:human-verify, gate=blocking-human — package legitimacy):** PRE-APPROVED per the orchestrator's `<checkpoint_handling>` context — the user had already sanctioned adding Vitest for the BFF route-handler test during plan revision. Installed the official `vitest` package (vitest-dev/vitest, VoidZero/Vitest team) directly without pausing.
- **Final (checkpoint:human-verify, gate=blocking — live UI walkthrough):** NOT run in this execution pass (this run covers only the two `type="auto"` tasks). Per `.planning/CONVENTIONS.md`'s integration-test gate, the live human walkthrough (two distinct Keycloak users — Maker A creates, Checker B reviews; SoD self-view; PII reveal + audit row; DevTools token-absence check) remains outstanding before this slice can be considered fully verified end-to-end. Automated evidence captured above (tsc + build + route registration + 8/8 Vitest passing) satisfies the automated half of the gate.

## Known Stubs

None — the detail/review/reveal screens render live Go-API data through the BFF proxy; no hardcoded/placeholder data introduced.

## Threat Flags

None — all new surface (5 BFF routes, PII reveal proxy, slip_url composition) was already modelled in the plan's threat register (T-12-01..T-12-SC) and mitigated as specified (server-side token, Go re-enforces RBAC/SoD/audit, BFF passes through Go's status codes unchanged).

## Issues Encountered

None.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- 03-13 can reuse the `[id]` BFF directory structure and `bffForward` pattern established here for `submit`/`cancel`/`reissue`/`slip` routes.
- The live human UI walkthrough (checkpoint at the end of 03-12) is still outstanding — recommended before Phase 3 is formally re-closed, run together with 03-13's walkthrough since both touch the same detail screen.
- Cancel/reissue on the detail screen currently work via the pre-existing Server Action path (unchanged by this plan) — 03-13 will migrate them to BFF + TanStack mutations.

---
*Phase: 03-donation-lifecycle-maker-checker-issuance*
*Completed: 2026-07-03*
