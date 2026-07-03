---
phase: 03-donation-lifecycle-maker-checker-issuance
plan: 11
subsystem: api
tags: [go, sqlc, postgresql, pgx, rbac, audit, donation-detail-contract]

# Dependency graph
requires:
  - phase: 03-donation-lifecycle-maker-checker-issuance
    provides: donation service/handler/E2E harness (plans 03-01..03-09), D-R2/D-R3 contract decisions (03-CONTEXT-remediation.md)
provides:
  - "DonationDetailResponse — the FE-aligned detail contract (national_id_masked, address, email, note, created_by/created_by_id, review_history, replaces/replaced_by as {id,receipt_formatted}) returned by GetByID AND all eight mutations"
  - "Server-computed authorization flags (viewer_is_creator, can_approve, can_return, can_reject, can_reveal_pii) — the Go API is the authority (T-03-31), never the browser"
  - "GetUserDisplayName / GetReceiptRefByID / GetDonationReviewHistory sqlc queries (creator display-name lookup, void/reissue receipt expansion, audit_log-sourced review history)"
  - "E2E regression guard for the detail contract (auth flags + masked ID) over the real HTTP router with real tokens"
affects: [03-12, 03-13 (frontend detail/review screens now have a stable backend contract to consume)]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Single shared buildDetailResponse(ctx, row db.Donation, maskedTaxID, claims) builder for GetByID + all mutations — enriching one function aligns every screen at once"
    - "viewer_is_creator/can_* always resolve the viewer's users.id via GetUserByKeycloakSubject(claims.Subject), never compare claims.Subject directly (created-by-fk-mismatch class of bug, T-11-03)"
    - "Create/Reissue re-fetch the full db.Donation row inside their transaction after CreateDonation (whose RETURNING only carries a few columns) so the shared builder always has the complete row shape"
    - "review_history is sourced from audit_log (immutable, full history), never donations.review_reason (which only holds the LATEST review action)"

key-files:
  created: []
  modified:
    - donnarec-api/internal/db/queries/donations.sql
    - donnarec-api/internal/db/generated/donations.sql.go (+ querier.go — sqlc regen only)
    - donnarec-api/internal/donation/model.go
    - donnarec-api/internal/donation/service.go
    - donnarec-api/internal/donation/service_integration_test.go
    - donnarec-api/internal/donation/service_fk_repro_test.go
    - donnarec-api/cmd/server/e2e_test.go

key-decisions:
  - "GetDonationReviewHistory's reason column uses an explicit ::TEXT cast on the JSONB ->> extraction (after_json ->> 'review_reason')::TEXT — without the cast sqlc infers the column as interface{} instead of string, which is unusable without a manual type assertion at every call site. With the cast sqlc infers non-nullable string, which is also correct in practice: donation.return/reject always write a non-empty review_reason into after_json (the service enforces ErrMissingReason before ever building the audit entry), so the column is never actually NULL."
  - "Fixed internal/donation/service_fk_repro_test.go (created-by-fk-mismatch regression guard) — not listed in the plan's explicit file list but in the same package and broken by the identical CreatedBy rename (now a display name, not a UUID). Its identity assertion moved to resp.CreatedByID. Rule 1 (bug) auto-fix: directly caused by this task's own type swap, same package, same root cause as the plan's explicitly-listed service_integration_test.go fixes."
  - "DonationService.List (the older, unused []DonationResponse-returning method — superseded by Search/DonationListItem in plan 03-09, and confirmed dead code via a repo-wide grep with zero call sites) was left untouched. It does not call the removed donationRowToResponse and is out of this plan's explicit 9-call-site scope, so touching it would be unnecessary risk with no functional benefit."

requirements-completed: [FR-29, FR-14, FR-12, FR-19]

# Metrics
duration: ~30min
completed: 2026-07-03
---

# Phase 3 Plan 11: Donation detail-screen contract (DonationDetailResponse + auth flags) Summary

**GetByID and all eight donation mutations now return `DonationDetailResponse` — the FE-aligned contract (masked national ID, display-name `created_by` + UUID `created_by_id`, nested `replaces`/`replaced_by`, full `review_history`) with server-computed `viewer_is_creator`/`can_approve`/`can_return`/`can_reject`/`can_reveal_pii` flags, built by one shared `buildDetailResponse` and proven over the real HTTP router with real tokens.**

## Performance

- **Duration:** ~30 min
- **Tasks:** 3 (of plan), executed as 3 commits (RED, GREEN, E2E extension) per the plan's TDD structure
- **Files modified:** 7 substantive (+ 2 sqlc-generated files touched by codegen only)

## Accomplishments

- Added three sqlc queries (`GetUserDisplayName`, `GetReceiptRefByID`, `GetDonationReviewHistory`) sourcing the creator's display name, the `{id,receipt_formatted}` expansion for the D-50 void/reissue self-FK pointers, and the full return/reject review history from the immutable `audit_log`.
- Introduced `DonationDetailResponse` (+ `ReceiptRef`, `ReviewHistoryEntry`) in `model.go` with exact FE-aligned JSON tags, and a single shared `DonationService.buildDetailResponse` that `GetByID`, `Create`, `UpdateDraft`, `Submit`, `Approve`, `Return`, `Reject`, `Cancel`, and `Reissue` all now return.
- `viewer_is_creator` is computed by resolving `claims.Subject` → `users.id` via `GetUserByKeycloakSubject` and comparing against `row.CreatedBy` — never comparing `claims.Subject` directly (the same identity-resolution discipline that fixed `created-by-fk-mismatch`, T-11-03).
- `can_approve`/`can_return`/`can_reject` = reviewer role (checker|admin) AND `status==pending_review` AND NOT `viewer_is_creator` — a SoD-aware UI hint; every mutation still independently re-enforces SoD/RBAC server-side (T-11-01).
- `national_id_masked` stays masked on every response; no plaintext tax ID field exists anywhere on `DonationDetailResponse` (FR-29, T-11-02).
- Extended `TestE2E_MakerCheckerIssuancePipeline`'s `HappyPath` subtest with a new Step 2b: `GET /api/donations/{id}` driven by both the maker token (own record: `viewer_is_creator=true`, `can_approve=false`) and the checker token (`viewer_is_creator=false`, `can_approve=true`), plus assertions that `national_id_masked` never contains a 13-digit plaintext run and `created_by`/`created_by_id` match the maker — all over the real router with a real signed Keycloak-shaped token.

## Task Commits

Each task was committed atomically per the plan's RED/GREEN/extension structure:

1. **Task 1 (RED — queries + type scaffolding + retyped invariant suite)** - `1263675` (test)
   - Three sqlc queries added to `donations.sql`, regenerated via `sqlc generate`
   - `DonationDetailResponse`/`ReceiptRef`/`ReviewHistoryEntry` types added to `model.go`
   - `service_integration_test.go`'s `createAndSubmit`/`createAndIssue` helpers retyped to `*donation.DonationDetailResponse`, field renames applied (`.DonorTaxIDMasked`→`.NationalIDMasked`, `.DonorAddress`→`.Address`, `dA.CreatedBy`→`dA.CreatedByID`, `replacement.Replaces`→`replacement.Replaces.ID`)
   - Confirmed RED via `go vet`: package failed to compile because `service.go` still returned `*DonationResponse`
2. **Task 2 (GREEN — buildDetailResponse + repoint 9 call sites)** - `2845a7c` (feat)
   - `DonationService.buildDetailResponse` + `expandReceiptRef` + `reviewActionLabel` helpers added; old `donationRowToResponse` removed
   - All nine service methods repointed; `Create`/`Reissue` gained an in-tx re-fetch of the full `db.Donation` row
   - Fixed `service_fk_repro_test.go` (Rule 1 — same-package fallout of the type swap, not in the plan's explicit line list)
   - Verified: `go build ./... && go vet ./...` clean; `go test ./internal/donation/...` 31/31 (Docker); `-race` concurrency test passes; `go test -short ./...` 96/96 across 15 packages
3. **Task 3 (E2E extension)** - `159d9ee` (test)
   - `dataEnvelope`/`decodeDonation` decode `DonationDetailResponse`; new Step 2b asserts the detail contract for both maker and checker viewpoints on the same pending_review record
   - Verified: `go test ./cmd/server/ -run TestE2E_MakerCheckerIssuancePipeline` — 6/6 subtests pass (Docker/testcontainers, real signed token)

**Plan metadata:** (this commit) `docs(03-11): complete donation detail-screen contract plan`

## Files Created/Modified

- `donnarec-api/internal/db/queries/donations.sql` — Added `GetUserDisplayName`, `GetReceiptRefByID`, `GetDonationReviewHistory` (audit_log-sourced, filtered to `donation.return`/`donation.reject`, ordered `created_at ASC`)
- `donnarec-api/internal/db/generated/{donations.sql.go,querier.go}` — sqlc regen only (three new generated methods)
- `donnarec-api/internal/donation/model.go` — `DonationDetailResponse`, `ReceiptRef`, `ReviewHistoryEntry` added; `DonationResponse` retained (still used by the unrelated, unused `DonationService.List` method)
- `donnarec-api/internal/donation/service.go` — `buildDetailResponse`/`expandReceiptRef`/`reviewActionLabel` added; `donationRowToResponse` removed; all nine methods (`Create`, `GetByID`, `UpdateDraft`, `Submit`, `Approve`, `Return`, `Reject`, `Cancel`, `Reissue`) repointed to the new builder and return type
- `donnarec-api/internal/donation/service_integration_test.go` — "7 Hardest Invariants" suite retyped/renamed to match `DonationDetailResponse`
- `donnarec-api/internal/donation/service_fk_repro_test.go` — identity assertion moved from `.CreatedBy` to `.CreatedByID` (Rule 1 fix, same-package fallout)
- `donnarec-api/cmd/server/e2e_test.go` — envelope decode type swapped; new detail-contract assertions added to `HappyPath`

## Decisions Made

See `key-decisions` in frontmatter: the `::TEXT` cast for `GetDonationReviewHistory.reason`, the `service_fk_repro_test.go` Rule 1 fix, and leaving `DonationService.List` untouched (confirmed dead code, out of the plan's explicit 9-call-site scope).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed `service_fk_repro_test.go`'s identity assertion for the CreatedBy rename**
- **Found during:** Task 2 GREEN verification (`go test ./internal/donation/...` with Docker)
- **Issue:** `TestCreate_ActingUserIDWritesCorrectCreatedBy` (a third test file in the same package, not enumerated in the plan's explicit `service_integration_test.go` line list) asserted `resp.CreatedBy == userRow.ID.String()` — but `CreatedBy` is now the creator's display name under the new contract, not a UUID, so the test failed (`expected: "086c5430-..." actual: "Real KC Subject Maker"`).
- **Fix:** Moved the identity assertions to `resp.CreatedByID` (the UUID field), matching the same rename pattern applied to `service_integration_test.go` in Task 1.
- **Files modified:** `donnarec-api/internal/donation/service_fk_repro_test.go`
- **Verification:** `go test ./internal/donation/...` 31/31 passing (Docker/testcontainers)
- **Committed in:** `2845a7c` (Task 2 GREEN commit)

---

**Total deviations:** 1 auto-fixed (Rule 1 — bug directly caused by this task's own type swap, same package, same root cause as the plan's explicitly-listed fixes)
**Impact on plan:** Necessary for correctness. No scope creep — same rename pattern already applied under the plan's explicit instruction, just in a sibling test file the plan's line-number list did not enumerate.

## Issues Encountered

`TestReturnToDraft` failed once during the initial full-package test run with `port "5432/tcp" not found` — a testcontainers infrastructure flake from rapid concurrent Postgres container creation, not a code defect. Re-ran the full suite immediately after the `service_fk_repro_test.go` fix and all 31 tests (including `TestReturnToDraft`) passed cleanly; no code change was needed for this one.

## User Setup Required

None — no external service configuration required. Docker/testcontainers were available and used to run the real integration and E2E tests (not just `-short`).

## Next Phase Readiness

- Plans 03-12/03-13 (frontend detail/review screens) can now consume `GET /api/donations/:id` and every mutation response directly — `national_id_masked`, `address`, `email`, `note`, `created_by`/`created_by_id`, `review_history`, `replaces`/`replaced_by` as `{id,receipt_formatted}`, and `viewer_is_creator`/`can_approve`/`can_return`/`can_reject`/`can_reveal_pii` are all live and E2E-verified.
- The integration-test gate (`.planning/CONVENTIONS.md`) is satisfied for this slice: `TestE2E_MakerCheckerIssuancePipeline` exercises the real HTTP path with a real signed Keycloak-shaped token and asserts the enriched detail contract for both maker and checker viewpoints on the same record.
- No blockers for 03-12/03-13.

---
*Phase: 03-donation-lifecycle-maker-checker-issuance*
*Completed: 2026-07-03*

## Self-Check: PASSED

- FOUND: donnarec-api/internal/donation/model.go
- FOUND: donnarec-api/internal/donation/service.go
- FOUND: donnarec-api/internal/db/queries/donations.sql
- FOUND: donnarec-api/cmd/server/e2e_test.go
- FOUND: .planning/phases/03-donation-lifecycle-maker-checker-issuance/03-11-SUMMARY.md
- Commit 1263675 (test — RED): FOUND
- Commit 2845a7c (feat — GREEN): FOUND
- Commit 159d9ee (test — E2E extension): FOUND
