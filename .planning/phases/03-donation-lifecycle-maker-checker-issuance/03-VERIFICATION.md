---
phase: 03-donation-lifecycle-maker-checker-issuance
verified: 2026-07-04T00:00:00Z
status: human_needed
score: 6/6 must-haves verified (automated half of criterion 6 now met; human UI walkthrough still outstanding)
overrides_applied: 0
re_verification:
  previous_status: gaps_found (addendum reopened 2026-07-02; criterion 6 not met)
  previous_score: "5/5 (criteria 1-5); criterion 6 unmet (2 of 3 seam bugs open)"
  gaps_closed:
    - "created-by-fk-mismatch (bug #1) — fixed pre-remediation, reconfirmed still fixed"
    - "fe-be-audience-mismatch (bug #2) — audience mapper + confidential client committed to keycloak/realm-donnarec.json; donnarec-web/.env.example added"
    - "RBAC AND-bug (bug #3) — RequireAnyRole (OR semantics) now used for donationGroup and checkerGroup in cmd/server/main.go (commit b10fae8)"
    - "D-R2 pagination envelope {data:{items,total,page,per_page}} implemented in handler.go List + service.go Search with real COUNT (03-09)"
    - "D-R1 BFF proxy pattern implemented: all 11 donation BFF routes under app/api/bff/donations/** obtain the Keycloak token server-side via getServerSession/bffForward — token never reaches the browser (03-10/03-12/03-13)"
    - "D-R3 all screens migrated: list, detail, PII reveal, approve/return/reject, cancel/reissue, create/update/submit, slip — all via TanStack Query/Table + BFF (03-10..03-13)"
    - "bug #5 (apiFetch does not unwrap `data`, causing `result.donations` crash) — root-fixed; apiFetch now unwraps the envelope; zero remaining `result.donations` property-access call sites"
    - "Automated E2E integration test covering the critical Maker/Checker flows over the REAL HTTP path with a real minted Keycloak token now exists and passes (cmd/server/e2e_test.go: TestE2E_MakerCheckerIssuancePipeline, 7 subtests)"
  gaps_remaining:
    - "Human UI walkthrough (criterion 6, part b) has not yet been performed against a live full stack with a real Keycloak session — every remediation plan's final checkpoint (03-10, 03-12, 03-13) explicitly defers this to a dedicated human pass"
  regressions: []
human_verification:
  - test: "Full-stack human UI walkthrough: bring up Go API + Postgres + Keycloak + MinIO + Next.js web app; sign in as two distinct Keycloak users (Maker A, Checker B)"
    expected: "Maker A can create -> edit (draft) -> upload slip -> submit a donation at /donations/new and /donations/[id]/edit. Checker B opens the pending_review record at /donations/[id] and sees Approve/Return/Reject buttons; approving issues a receipt number visible in the UI."
    why_human: "Requires a live Keycloak session (real login flow), real browser rendering, and real cross-user handoff — cannot be verified by grep or by the automated E2E test's httptest-only requests."
  - test: "DevTools Network tab check: confirm the Keycloak access token never appears in any response body sent to the browser"
    expected: "Inspect responses from /api/bff/donations, /api/bff/donations/[id], and all mutation routes — no access_token/Bearer string present anywhere in the JSON payloads returned to the client (D-R1 posture)."
    why_human: "Requires live browser DevTools inspection of real network traffic; cannot be grepped from source."
  - test: "SoD blocked state: log in as a Checker who is also the creator of a record (or reuse the dual-role E2E fixture pattern in the browser) and open that record's detail page"
    expected: "SoDBlockedAlert renders; Approve/Return/Reject buttons are absent from the DOM entirely (not merely disabled)."
    why_human: "Requires a live Keycloak session where `viewer_is_creator` is server-computed and reflected in real DOM output; the E2E test asserts the JSON flag but not the rendered DOM."
  - test: "PII reveal UX in the browser: open a donation as Checker/Admin, confirm the masked national ID displays, click reveal, confirm plaintext replaces it, then reload and confirm it re-masks"
    expected: "Masked ID shown by default; reveal button fetches plaintext via /api/bff/donations/[id]/pii; audit_log gains one pii.reveal row; reload re-masks (session-only reveal state)."
    why_human: "Session-only client state and a live audit-log side effect require a real browser + live backend, not just the BFF unit test's mocked assertions."
  - test: "Filter/pagination interaction on /donations: filter by name/status/date-range/receipt_no, and page through results"
    expected: "TanStack Table re-fetches via /api/bff/donations with updated query params; rows update; masked national IDs never appear in list rows (PII-free per D-53); Thai/English i18n renders correctly."
    why_human: "Visual rendering, i18n, and interactive client-state behavior cannot be verified programmatically."
  - test: "Cancel / Void & Reissue dialogs in the browser, including edonation_keyed=true guard"
    expected: "Cancel dialog requires a reason; when edonation_keyed=true, rd_confirmation_reason is required and blocks submit until filled; Void & Reissue creates a new draft, cancels the original, and the receipt number is retained on the original (matches the E2E-proven backend invariant)."
    why_human: "Dialog interaction, conditional field rendering, and multi-step UI flow require manual browser testing."
---

# Phase 3: Donation Lifecycle & Maker-Checker Issuance — Verification Report (Remediation Re-Verification)

**Phase Goal:** A Maker can create/submit a donation with encrypted donor PII; a Checker (never the Maker) can approve/return; approval issues a gap-less numbered receipt in one atomic transaction. Criterion 6 (added on reopen): an automated E2E test drives the REAL HTTP path (router → RequireAuth → RequireRoles/ResolveAppUser → handler → service → DB) with a realistic Keycloak token for the critical flows, AND the human UI walkthrough passes.

**Verified:** 2026-07-04

**Status:** HUMAN_NEEDED — the remediation slice (03-09..03-13) closes the integration gate's *automated* half (criterion 6a) and re-confirms criteria 1-5 are not regressed. Criterion 6's *human* half (6b, the UI walkthrough) has not been performed by any of the remediation plans — each one's final checkpoint explicitly defers it. This is not a code gap; it is the one remaining required gate before the phase can be marked Complete.

**Re-verification:** Yes — after remediation of the 2026-07-02 reopen (bugs #1-#3 + the D-R1/D-R2/D-R3 frontend contract migration).

---

## Goal Achievement — Remediation Focus

### Observable Truths (remediation scope)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | D-R2 pagination envelope `{data:{items,total,page,per_page}}` is actually implemented in the backend list contract, with a real `COUNT`, not `len(items)` | VERIFIED | `donnarec-api/internal/donation/handler.go:559-566` (`List`) builds `gin.H{"data": gin.H{"items":items,"total":total,"page":...,"per_page":...}}`; `service.go:Search` calls both `SearchDonations` and a dedicated `CountDonations` sqlc query sharing the identical 5-predicate WHERE clause (03-09-SUMMARY.md); independently re-ran `TestE2E_MakerCheckerIssuancePipeline` which asserts `list.Data.Total >= 1` on a real Postgres COUNT — PASS |
| 2 | D-R1 honored: the BFF obtains the Keycloak token server-side; the browser never receives it | VERIFIED | `donnarec-web/lib/bff.ts:bffForward` calls `getServerSession(authOptions)` and attaches `Authorization: Bearer` only on the outbound server-side fetch; `donnarec-web/lib/donations.ts` — every client-facing fetcher (`fetchDonations`, `fetchDonation`, `approve`, `returnForEdit`, `reject`, `revealPII`, `createDonation`, `updateDonation`, `submitDonation`, `uploadSlip`, `viewSlip`, `removeSlip`, `cancelDonation`, `reissueDonation`) calls only same-origin `/api/bff/donations/**` paths — no Go API URL or token ever appears client-side; 8/8 Vitest BFF route tests (including an explicit no-token-leak test) independently re-run and PASS |
| 3 | D-R3 scope: every Phase-3 screen + mutation is migrated and contract-aligned | VERIFIED | 11 BFF route files confirmed on disk under `app/api/bff/donations/**`: list (`route.ts` GET+POST), detail (`[id]/route.ts` GET+PUT, composes `slip_url`), pii, approve, return, reject, cancel, reissue (composes original detail + audited PII reveal before forwarding), submit, slip (POST/GET/DELETE with raw multipart passthrough). `DonationForm.tsx` owns create/update/submit/slip via `useMutation`; `DonationDetailView.tsx` owns approve/return/reject/cancel/reissue via `useMutation`; `DonationListView.tsx`/`DonationTable.tsx` own list via `useQuery`+`@tanstack/react-table`. `[id]/page.tsx` and `new/page.tsx` are thin server shells with zero Server Actions remaining for donation mutations |
| 4 | bug #5 root fix present: `apiFetch` unwraps `data`; list no longer crashes on `undefined.length` | VERIFIED | `lib/api.ts:169-183` — explicit `D-R2` comment + unwrap logic (`if "data" in body: return body.data`); `grep -rn "result.donations"` across `app/`, `components/`, `lib/` returns only two comment references (bug-history documentation), zero property-access call sites |
| 5 | Integration gate 6a: automated E2E test drives the real HTTP path with a realistic Keycloak token, covering create/submit/approve/return + cancel, and PASSES | VERIFIED (independently re-run) | `cmd/server/e2e_test.go:TestE2E_MakerCheckerIssuancePipeline` builds the router via the production `setupRouter` (not a test-only router), mints tokens via a real local OIDC/JWKS test server (`testutil.KeycloakTestServer` + `MintTokenForSubject`), and drives every step via `httptest.NewRecorder()` + `router.ServeHTTP` (genuine HTTP request/response cycle) — NOT a hand-constructed-claims unit test. 7 subtests: `HappyPath_CreateSubmitApproveList`, `UnprovisionedSubject_403`, `RBAC_MakerRejectedFromCheckerOnlyRoute`, `SoD_SelfApprove_403`, `Audience_WrongClient_401`, `Cancel_RetainsReceiptNumber_RealPath`. Independently re-ran: **7/7 PASS** (`go test -run TestE2E_MakerCheckerIssuancePipeline ./cmd/server/...`, Docker/testcontainers) |
| 6 | No regression to gap-less numbering / SoD / issuance invariants under the retyped `DonationDetailResponse` contract | VERIFIED | Independently re-ran `go test ./internal/donation/...` (Docker) — **31/31 PASS**, including `TestConcurrentApproval_ExactlyOneSucceeds`, `TestIssuanceTransaction_RollbackOnError`, `TestSoD_DBCheckConstraint`, `TestCancelRetainsReceiptNumber`, `TestVoidAndReissue`, `TestPII_RevealRequiresCheckerOrAdmin` — all retyped to `DonationDetailResponse` (03-11) and still green |

**Score:** 6/6 remediation truths verified (all automated). Criterion 6's human-UI-walkthrough half remains outstanding (see Human Verification below) — this is the reason overall status is `human_needed`, not `passed`.

---

### Seam-bug closure verification (2026-07-02 addendum bugs #1-#3)

| Bug | Status at reopen | Status now | Evidence |
|-----|------------------|------------|----------|
| #1 `created-by-fk-mismatch` | FIXED + committed (ef7ede6, a1e348e) | STILL FIXED (regression-tested) | `auth.ResolveAppUser` middleware present in `cmd/server/main.go`; E2E `HappyPath` step asserts `created.CreatedByID == makerID.String()` and `created.CreatedByID != subMaker` — PASS |
| #2 `fe-be-audience-mismatch` | FIXED, uncommitted | FIXED + COMMITTED | `keycloak/realm-donnarec.json:96-101` defines an `audience-donnarec-backend` protocol mapper with `included.client.audience: donnarec-backend`; `donnarec-web/.env.example` present on disk; E2E `Audience_WrongClient_401` subtest independently re-run — PASS (401 `invalid_token` for a token minted with the wrong audience) |
| #3 RBAC AND-bug | OPEN | FIXED + COMMITTED | `git log -S"RequireAnyRole"` shows commit `b10fae8` ("fix(03): add RequireAnyRole (OR) for multi-role route guards — RBAC AND-bug"); `cmd/server/main.go:236` uses `auth.RequireAnyRole(auth.RoleMaker, auth.RoleChecker, auth.RoleAdmin)` for `donationGroup` and `main.go:270` uses `auth.RequireAnyRole(auth.RoleChecker, auth.RoleAdmin)` for `checkerGroup`; E2E `HappyPath` explicitly regression-tests both (maker-only token accepted on create; checker-only token accepted on approve) — PASS |

All three seam bugs identified at reopen are closed, committed, and covered by the real-router E2E test (not just unit tests) — satisfying the Conventions.md Integration-test gate's intent that these bug classes be caught by tests that exercise the real seam.

---

### Required Artifacts (remediation-added/modified)

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `donnarec-api/internal/donation/handler.go` (`List`) | D-R2 envelope | VERIFIED | Builds nested `{"data":{"items","total","page","per_page"}}`; explicit anti-bug-#5 comment |
| `donnarec-api/internal/donation/service.go` (`Search`, `buildDetailResponse`) | Real COUNT + FE-aligned detail contract | VERIFIED | `Search` returns `(items, total, err)`; `buildDetailResponse` shared by `GetByID` + all 8 mutations, computing `viewer_is_creator`/`can_approve`/`can_return`/`can_reject`/`can_reveal_pii` server-side |
| `donnarec-web/lib/api.ts` | Envelope unwrap | VERIFIED | `apiFetch` unwraps `data`; used by server-side callers (edit-page seed data) |
| `donnarec-web/lib/bff.ts` | BFF proxy helpers | VERIFIED | `bffForward`, `goFetch`, `passthroughGoResponse`, `mapFeDonorFieldsToGo` — token obtained via `getServerSession` only |
| `donnarec-web/app/api/bff/donations/**` (11 route files) | All D-R3 screens/mutations | VERIFIED | list, detail(+slip_url compose), pii, approve, return, reject, cancel, reissue(+2-step compose), submit, slip(POST/GET/DELETE) all present and substantive (no stubs) |
| `donnarec-web/components/DonationListView.tsx`, `DonationTable.tsx` | List screen (TanStack Query/Table) | VERIFIED | `useQuery` + `useReactTable`; renders `items`/`total`/`page`/`perPage` from the BFF |
| `donnarec-web/components/DonationDetailView.tsx` | Detail/review/PII/cancel/reissue screen | VERIFIED | `useQuery` for the record; `useMutation` for approve/return/reject/cancel/reissue; SoD/branching logic ported verbatim |
| `donnarec-web/components/DonationForm.tsx` | Create/edit/slip screen | VERIFIED | `useMutation` for create/update/submit/uploadSlip/removeSlip |
| `donnarec-api/cmd/server/e2e_test.go` | Real HTTP-path E2E | VERIFIED | 7 subtests over `setupRouter` + real minted tokens; independently re-run, 7/7 PASS |
| `donnarec-web/app/api/bff/donations/__tests__/bff-routes.test.ts` | BFF trust-boundary test | VERIFIED | 8 hermetic tests (token forwarding, 401 gate, field mapping, slip_url composition, no-token-leak); independently re-run, 8/8 PASS |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `DonationListView.tsx` | `/api/bff/donations` | `useQuery(["donations",filter], fetchDonations)` | WIRED | Confirmed by grep + build output (`ƒ /api/bff/donations` route registered) |
| `DonationDetailView.tsx` | `/api/bff/donations/[id]`, `/approve`, `/return`, `/reject`, `/cancel`, `/reissue` | `useQuery` + 5×`useMutation` | WIRED | All 6 endpoints present in build output; mutations invalidate the detail query on success |
| `DonationForm.tsx` | `/api/bff/donations` (POST), `/api/bff/donations/[id]` (PUT), `/submit`, `/slip` | 4×`useMutation` | WIRED | Confirmed via grep of `createDonation`/`submitDonation` usage |
| `bffForward` | `getServerSession(authOptions)` | direct call inside every proxy route (except create/update/reissue/slip, which use `getBffToken`/`goFetch` for composition) | WIRED | Token never returned to caller; confirmed via source read + Vitest no-token-leak test |
| `cmd/server/main.go:donationGroup/checkerGroup` | `auth.RequireAnyRole` | route-guard middleware | WIRED | Confirmed via source read + E2E RBAC subtests |
| `e2e_test.go:newE2EHarness` | production `setupRouter` | `setupRouter(authMW, auditSvc, appUserResolver, ...)` | WIRED | Same function used in `cmd/server/main.go`'s real boot path — this is not a parallel/simplified test router |

---

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|---------------------|--------|
| `handler.go:List` | `items`, `total` | `svc.Search` → `SearchDonations` + `CountDonations` (sqlc, real SQL) | Real Postgres query, not static/empty | FLOWING |
| `DonationListView.tsx` | `items`/`total`/`page`/`perPage` | `useQuery` → `fetchDonations` → `/api/bff/donations` → Go `List` | End-to-end real data (verified via independent E2E re-run asserting `total>=1` and item fields) | FLOWING |
| `DonationDetailView.tsx` | detail record + auth flags | `useQuery` → `fetchDonation` → BFF → Go `GetByID`/`buildDetailResponse` | Server-computed flags from real `users`/`donations` join, not client-trusted | FLOWING |
| BFF reissue route | composed `goBody` | 2 sequential real Go calls (`GET /:id` detail + `GET /:id/pii`) before forwarding | Real donor data + real audited PII reveal, not hardcoded | FLOWING |

---

### Behavioral Spot-Checks (independently re-run by this verifier, not trusted from SUMMARY)

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Go backend compiles | `cd donnarec-api && go build ./...` | exit 0 | PASS |
| Go vet clean | `cd donnarec-api && go vet ./...` | no issues | PASS |
| Real-router E2E (Docker) | `go test -run TestE2E_MakerCheckerIssuancePipeline -v ./cmd/server/...` | 7 passed | PASS |
| Donation integration suite (Docker) | `go test ./internal/donation/...` | 31 passed | PASS |
| Frontend type-check | `cd donnarec-web && npx tsc --noEmit` | No errors found | PASS |
| Frontend production build | `cd donnarec-web && npm run build` | Compiled successfully; all 11 BFF routes + 4 donation pages registered | PASS |
| BFF route-handler tests (Vitest) | `npx vitest run app/api/bff/donations/__tests__/bff-routes.test.ts` | 8 passed, 0 failed | PASS |
| Grep for bug #5 residue | `grep -rn "result.donations" app components lib` | 2 comment-only matches, 0 property access | PASS |

All 8 checks were executed fresh by this verifier (not copied from SUMMARY.md), using Docker (confirmed available: `docker info` succeeded).

---

### Probe Execution

No `scripts/*/tests/probe-*.sh` files found. Step 7c: SKIPPED (no probe scripts).

---

### Requirements Coverage

| Requirement | Description | Status | Evidence |
|---|---|---|---|
| FR-07 | Maker creates donation record | SATISFIED | `POST /api/bff/donations` → Go `Create`; E2E `HappyPath` step 1 PASS |
| FR-09 | Maker edits while draft | SATISFIED | `PUT /api/bff/donations/[id]` → Go `Update`; `DonationForm` update mutation |
| FR-10 | Search/filter by name, date, status, receipt no | SATISFIED | D-R2 envelope + `Search`/`CountDonations`; E2E list assertions PASS |
| FR-11 | Lifecycle state machine enforced | SATISFIED | Unchanged from original verification; not regressed (31/31 integration tests still green) |
| FR-12 | Return/Reject with mandatory reason | SATISFIED | `return`/`reject` BFF routes pass through Go 422 `missing_reason` unchanged |
| FR-14 | Checker approval; SoD | SATISFIED | E2E `SoD_SelfApprove_403` independently re-run — PASS |
| FR-19 | Cancel retains receipt number, audited | SATISFIED | E2E `Cancel_RetainsReceiptNumber_RealPath` independently re-run — PASS (receipt number identical pre/post cancel) |
| FR-29 | PII encrypted, masked, audited reveal | SATISFIED | `pii` BFF route field-maps `donor_tax_id`→`national_id`; Go-side masking/audit unchanged and still tested (31/31) |

REQUIREMENTS.md tracking table now shows all 8 requirements as "Complete" (previously flagged as a stale "Pending" snapshot in the prior verification) — confirmed via direct read of `.planning/REQUIREMENTS.md:139-146`.

---

### Anti-Patterns Found

No blockers in remediation-modified files:
- No `TBD`/`FIXME`/`XXX`/`TODO`/`HACK`/`PLACEHOLDER` markers found across `donnarec-api/internal/donation/{model,service,handler}.go`, `donnarec-api/cmd/server/e2e_test.go`, `donnarec-api/internal/db/queries/donations.sql`, `donnarec-web/lib/{bff,api,donations}.ts`, all 11 BFF route files, and the 4 migrated components/pages.
- One incidental match for the string "placeholder" in `DonationTable.tsx:289` — this is `header.isPlaceholder`, a legitimate `@tanstack/react-table` API property, not a stub marker.
- No hardcoded empty-array/empty-object returns found in the migrated BFF routes or components; all routes forward real Go responses or compose real multi-step data.

---

### Human Verification Required

Criterion 6, part (b) — the human UI walkthrough — has explicitly NOT been run by any of the 5 remediation plans. Each plan's final checkpoint (`checkpoint:human-verify`, gate=blocking) was either auto-advanced with only automated evidence (03-10) or explicitly deferred/not-run (03-12: "NOT run in this execution pass... remains outstanding"; 03-13: "Ready for the Task 3 checkpoint... was not part of this executor's remaining work scope"). This is the single remaining gate before Phase 3 can be marked Complete. See the `human_verification` list in the frontmatter above for the precise checklist (6 items): full-stack walkthrough with two distinct Keycloak users, DevTools token-absence check, SoD DOM-removal check, PII reveal UX + audit row, filter/pagination interaction, and cancel/void-reissue dialogs including the `edonation_keyed` guard.

---

### Gaps Summary

No code gaps. All automated evidence for criteria 1-5 (re-confirmed, not regressed) and criterion 6a (integration gate, automated half) was independently re-executed by this verifier — not taken from SUMMARY.md claims — and passed:
- `go build`/`go vet` clean
- Real-router E2E: 7/7 subtests pass (Docker/testcontainers, real minted Keycloak-shaped tokens, production `setupRouter`)
- Donation integration suite: 31/31 pass (gap-less counter, SoD DB constraint, cancel-retains-number, void/reissue, PII encryption/reveal — all retyped to the new `DonationDetailResponse` contract and still green)
- Frontend: `tsc --noEmit` clean, `npm run build` clean (11 BFF routes + 4 donation pages registered), 8/8 Vitest BFF route tests pass
- All three seam bugs from the 2026-07-02 reopen (FK mismatch, audience mismatch, RBAC AND-bug) are fixed, committed, and covered by E2E regression subtests

The only remaining item is criterion 6, part (b): the live human UI walkthrough against a full running stack (Go API + Postgres + Keycloak + MinIO + Next.js) with two distinct real Keycloak-authenticated users. This was consistently and explicitly deferred by every remediation plan as "requires a live full-stack environment + human verification, out of scope for this automated completion pass" — it is not evidence of a code defect, but it is a required part of the phase's own done-criterion (Conventions.md Integration-test gate: "(a) an automated E2E integration test ... AND (b) the human UI walkthrough ... passing"). Status is therefore `human_needed`, not `passed`, until a human runs the checklist above.

---

_Verified: 2026-07-04_
_Verifier: Claude (gsd-verifier)_
_Original verification (criteria 1-5, unit/service level) and the 2026-07-02 reopen addendum are preserved in git history of this file; this revision supersedes both with the remediation slice's re-verification._
