---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: executing
stopped_at: Completed 03-13-PLAN.md (create/edit/cancel/reissue donation flows migrated to BFF + TanStack; E2E create+cancel over the production router)
last_updated: "2026-07-04T00:07:17.673Z"
last_activity: 2026-07-04
progress:
  total_phases: 6
  completed_phases: 3
  total_plans: 22
  completed_plans: 22
  percent: 50
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-06-22)

**Core value:** ออกใบเสร็จบริจาคที่มีเลขที่รันต่อเนื่องไม่ซ้ำ ห้ามข้ามเลข (gap-less) ตามปีงบประมาณ หลังผ่านการอนุมัติโดยมนุษย์ และส่งถึงผู้บริจาคได้อย่างถูกต้องน่าเชื่อถือ
**Current focus:** Phase 03 — donation-lifecycle-maker-checker-issuance

## Current Position

Phase: 03 (donation-lifecycle-maker-checker-issuance) — EXECUTING
Plan: 6 of 13
Plans: 8/8 complete (criteria 1–5, unit/service-level). Integration gate (criterion 6) NOT met.
Status: Ready to execute
Last activity: 2026-07-04

Context: Phase 3 was marked Complete 2026-07-01 on 5/5 unit-level verification. On 2026-07-02, standing up the real stack (docker compose; postgres remapped to host 5433 via docker-compose.override.yml; 4 users seeded) and driving it with a real Keycloak token surfaced three runtime-integration-seam bugs that unit tests structurally could not catch. New done-criterion added (Conventions → Integration-test gate; ROADMAP Phase 3 criterion 6).

## Performance Metrics

**Velocity:**

- Total plans completed: 5
- Average duration: —
- Total execution time: —

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01 | 5 | - | - |

**Recent Trend:**

- Last 5 plans: —
- Trend: —

*Updated after each plan completion*
| Phase 02-gap-less-receipt-numbering-core P01 | 262s | 2 tasks | 8 files |
| Phase 02 P02 | 254 | 2 tasks | 4 files |
| Phase 02 P03 | 256 | 1 tasks | 2 files |
| Phase 02-gap-less-receipt-numbering-core P04 | 502 | 2 tasks | 4 files |
| Phase 03 P05 | 120 | 3 tasks | 7 files |
| Phase 03 P09 | 35min | 3 tasks | 7 files |
| Phase 03 P11 | 30min | 3 tasks | 7 files |
| Phase 03 P10 | 18min | 2 tasks | 13 files |
| Phase 03 P12 | 35min | 2 tasks | 12 files |
| Phase 03 P13 | 3min | 3 tasks | 9 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- Roadmap: Correctness-first ordering — gap-less numbering (Phase 2) is built and concurrency-proven BEFORE the approval/issuance flow (Phase 3) depends on it.
- Roadmap: Foundation (audit + RBAC + retention model) first — expensive to retrofit; everything depends on it.
- Roadmap: PDF + email decoupled behind an async outbox worker (Phase 4); kept out of the issuance transaction.
- Roadmap: Public donation web form (Flow B) built LAST (Phase 6) — reuses the entire Flow-A pipeline.
- [Phase ?]: Allocator.queries field is *db.Queries (concrete) not db.Querier — WithTx bind requires concrete type (02-PATTERNS Key Observation #1)
- [Phase ?]: allocator_test.go uses black-box package receiptno_test; Allocator/NewAllocator/AllocatedReceipt are exported
- [Phase 03]: Atomic 7-step Approve tx: lock→SoD→Allocate→Issue→Audit→Outbox in one WithTx closure — any failure rolls back all effects; receipt number exists only on issued records
- [Phase 03]: SoD enforced at code layer (approverID != createdBy) AND DB CHECK (chk_sod_approver) — defense-in-depth; both layers tested by integration tests
- [Phase 03]: Checker route group RequireRoles(Checker, Admin) at HTTP layer in addition to service-layer SoD guard — defense-in-depth over service layer
  - ⚠️ CORRECTION 2026-07-02: this decision encoded the RBAC AND-bug. `RequireRoles(...)` enforces AND (user must hold ALL listed roles), so `RequireRoles(Checker, Admin)` requires checker AND admin — a checker-only user is wrongly blocked. Intent was "checker OR admin". Same bug on donationGroup `RequireRoles(Maker,Checker,Admin)`. Fix: add `RequireAnyRole` (OR) and use it at both sites. (bug #3, OPEN)
- [Phase 03] 2026-07-02: Identity model clarified — `claims.Subject` (KC sub) ≠ `users.id` (surrogate PK). All `REFERENCES users(id)` writes must resolve sub→users.id via `auth.ResolveAppUser` middleware; `audit_log.actor_id` intentionally keeps raw sub (no FK). (bug #1 fix, committed ef7ede6)
- [Phase 03] 2026-07-02: FE↔BE auth requires a Keycloak Audience mapper putting `donnarec-backend` in token `aud`, and `donnarec-frontend` must be a confidential client (NextAuth server-side). (bug #2 fix, uncommitted)
- [Phase 03]: GET /api/donations now returns the D-R2 pagination envelope {data:{items,total,page,per_page}} with a real COUNT (CountDonations mirrors SearchDonations' filter predicate) and creator display-name/UUID per row
- [Phase 03]: donations.sql list filters use sqlc.narg(...) instead of bare @param for nullable params — fixes a hand-edited generated-code fragility (donations.sql.go pointer types were manually patched, violating DO NOT EDIT); sqlc.narg() makes the D-53 nil-skips-filter semantics regeneration-safe
- [Phase 03]: DonationDetailResponse (D-R3): GetByID + all 8 mutations now share one buildDetailResponse builder returning national_id_masked/address/email/note/created_by(name)+created_by_id(UUID)/review_history/replaces+replaced_by({id,receipt_formatted}) plus server-computed viewer_is_creator/can_approve/can_return/can_reject/can_reveal_pii (T-03-31); viewer_is_creator always resolves claims.Subject to users.id via GetUserByKeycloakSubject, never compares claims.Subject directly (T-11-03)
- [Phase 03]: review_history is sourced from audit_log (immutable, full return/reject history) via a new GetDonationReviewHistory sqlc query, not donations.review_reason which only holds the latest review action
- [Phase ?]: [Phase 03] 03-10: BFF proxy pattern (D-R1) - app/api/bff Route Handlers + lib/bff.ts bffForward obtain the Keycloak token via getServerSession and forward a Bearer server-side; access token never reaches the browser. TanStack Query calls the same-origin BFF route only.
- [Phase ?]: [Phase 03] 03-10: apiFetch unwraps the data envelope (D-R2); DonationListResponse key donations to items; DonationSummary.amount is a numeric string (parseFloat at render). Root fix for bug #5 (undefined.length on result.donations). Donation list screen migrated to TanStack Query + TanStack Table.
- [Phase 03]: 03-12: BFF Route Handlers for donation detail (composes slip_url via a second server-side /:id/slip call)/pii (donor_tax_id->national_id mapping)/approve/return/reject; client DonationDetailView (useQuery+useMutation) drives Screen 3+4 — ReviewActionPanel/MaskedIdField needed zero changes since their existing Promise<{error}|null> callback contracts already matched the new mutation wrappers. Cancel/reissue deliberately deferred to 03-13.
- [Phase 03]: Cancel/reissue mutation wiring lives in DonationDetailView (useMutation), not in CancelDialog itself; CancelDialog stays presentational — Matches the existing approve/return/reject pattern established in 03-12; fixed a broken Server Action -> client-BFF-fetcher call path (Rule 1 bug)

### Pending Todos

Phase 3 integration-gate remediation (blocks marking Phase 3 Complete):

1. [x] Bug #1 `created-by-fk-mismatch` — resolve sub→users.id in `auth.ResolveAppUser` middleware. FIXED + committed (ef7ede6, refactor a1e348e).
2. [x] Bug #2 `fe-be-audience-mismatch` — audience mapper + confidential frontend client + web env. FIXED + committed (8604caa; debug doc 369dcce).
3. [x] Bug #3 RBAC AND-bug — added `RequireAnyRole` (OR); switched donationGroup + checkerGroup guards; test added. FIXED + committed (b10fae8).
4. [x] E2E HTTP integration test (real router + real signed token) — `cmd/server/e2e_test.go`: happy path + unprovisioned-403 + RBAC + SoD + audience. 5/5 subtests PASS (-race). COMMITTED (c5b0c4f). **Automated integration gate SATISFIED.**
5. [x] Gap #4 `frontend-auth-gating-missing` — frontend had NO route protection / login gating (root was a placeholder, no middleware, custom signin 404'd). Added middleware.ts (withAuth) + app/auth/signin (auto signIn keycloak) + `/`→`/donations` redirect + SessionProvider + SignOutButton. FIXED + committed (63c7a40; debug doc 71345e5). Verified: unauth /,/donations → 307 to signin; /auth/signin → 200.
6. [~] Human UI browser walkthrough — LAST remaining gate item, now UNBLOCKED (login works). Stack up (API :8000, Keycloak :8080, web :3000); users seeded (maker1/checker1/admin/makerchecker @ DonaRec123). Live E2E already proven via curl (Maker create→submit→Checker approve→issued, receipt 2569/000001). 5 visual items from 03-VERIFICATION human_verification need a human at the browser.
7. [ ] (Optional) wider auth/RBAC/wiring seam audit before formally re-closing Phase 3.

Once item 5 passes, Phase 3 integration gate (ROADMAP criterion 6) is met → Phase 3 can be re-marked Complete.

### Blockers/Concerns

Stakeholder confirmations are gated but NON-blocking for the roadmap; resolve at the relevant phase start:

- Phase 1: PDPA retention period (~5y vs erasure); email provider / KMS / hosting choices.
- Phase 4: §6 receipt wording + 1x/2x deduction eligibility (accounting/legal sign-off).
- Phase 5: exact e-Donation field spec + 1 Jan 2026 mandate obligation.

## Deferred Items

Items acknowledged and carried forward from previous milestone close:

| Category | Item | Status | Deferred At |
|----------|------|--------|-------------|
| *(none)* | | | |

## Session Continuity

Last session: 2026-07-04T00:07:17.666Z
Stopped at: Completed 03-13-PLAN.md (create/edit/cancel/reissue donation flows migrated to BFF + TanStack; E2E create+cancel over the production router)
Resume file: None
