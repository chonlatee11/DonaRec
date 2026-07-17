---
phase: 07-close-gap-flow-b-composite-e2e-public-submit-through-approve
plan: 01
subsystem: testing
tags: [testify, testcontainers-go, gin, e2e, keycloak, flow-b]

# Dependency graph
requires:
  - phase: 06-public-donation-web-form-flow-b
    provides: "Flow B public submission seam (doPublicSubmission, validPublicFields, source=flow_b, PublicWebUserID) + its own E2E test (TestPublicDonationE2E)"
  - phase: 03-maker-checker-approval-issuance (or equivalent core issuance phase)
    provides: "Maker/Checker approve → issuance seam + its own E2E test (TestE2E_MakerCheckerIssuancePipeline), gap-less receiptno.Allocator"
provides:
  - "TestE2E_FlowBCompositePublicSubmitToIssued — automated E2E lock on the composite handoff between the public-submit seam and the approve/issuance seam"
  - "Closes v1.0 milestone audit WARNING-1"
affects: [testing, v1.0-milestone-audit]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Composite E2E test pattern: reuse two independently-built harness halves (public-submit helper + authenticated-approve helper) in one test function to lock a cross-flow seam, without adding new harness/fixtures."

key-files:
  created:
    - donnarec-api/cmd/server/e2e_flowb_composite_test.go
  modified: []

key-decisions:
  - "Reused newE2EHarness/doPublicSubmission/validPublicFields/settingsPNGBytes/provisionUser/MintTokenForSubject/do/decodeDonation/backendClientID/donation.PublicWebUserID verbatim — no new harness, fixture, or helper needed beyond a local DB-lookup query, per plan constraint."
  - "Used a dedicated Checker keycloak subject (77777777-...-7777) distinct from donation.PublicWebUserID to make the SoD precondition (approver_id != created_by) explicit and assert it directly before approving."

patterns-established:
  - "Composite/cross-seam E2E tests should be added as new sibling files in package main reusing existing harness helpers, never by modifying the seam's own dedicated E2E file."

requirements-completed: [FR-01, FR-03, FR-04, FR-08, FR-14, FR-15, FR-16]

coverage:
  - id: D1
    description: "A flow_b record submitted via multipart POST /api/public/donations, when approved by a real Keycloak-shaped Checker token, reaches status=issued with a non-empty gap-less receipt_formatted."
    requirement: "FR-16"
    verification:
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_flowb_composite_test.go#TestE2E_FlowBCompositePublicSubmitToIssued"
        status: pass
    human_judgment: false
  - id: D2
    description: "Approval enqueues exactly one issue_receipt outbox job carrying the donation id."
    requirement: "FR-14"
    verification:
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_flowb_composite_test.go#TestE2E_FlowBCompositePublicSubmitToIssued"
        status: pass
    human_judgment: false
  - id: D3
    description: "SoD is NOT violated across the composite — flow_b created_by is the public-web UUID, distinct from the approving Checker, so approve succeeds (not 403)."
    requirement: "FR-08"
    verification:
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_flowb_composite_test.go#TestE2E_FlowBCompositePublicSubmitToIssued"
        status: pass
    human_judgment: false
  - id: D4
    description: "An in-tx approval audit row (action=donation.approve) is written under the Checker's Keycloak subject."
    requirement: "FR-15"
    verification:
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_flowb_composite_test.go#TestE2E_FlowBCompositePublicSubmitToIssued"
        status: pass
    human_judgment: false

duration: 25min
completed: 2026-07-17
status: complete
---

# Phase 07 Plan 01: Close Gap — Flow B Composite E2E Summary

**Added TestE2E_FlowBCompositePublicSubmitToIssued, the first automated test locking the full public-submit → Checker-approve → issued handoff, closing v1.0 milestone audit WARNING-1.**

## Performance

- **Duration:** ~25 min
- **Completed:** 2026-07-17T22:06:00Z
- **Tasks:** 1 (single-task plan)
- **Files modified:** 1 (new file)

## Accomplishments
- New E2E test `TestE2E_FlowBCompositePublicSubmitToIssued` drives the REAL router for BOTH halves of Flow B: unauthenticated multipart `POST /api/public/donations` → DB lookup of the created `flow_b` `pending_review` row → real Keycloak-shaped Checker token `POST /api/donations/{id}/approve`.
- Asserts the full composite invariant set: `status=issued`, non-empty gap-less `receipt_formatted`, exactly one `issue_receipt` outbox job, exactly one `donation.approve` audit row under the Checker's subject, and the SoD precondition (`created_by` = `donation.PublicWebUserID`, distinct from the approving Checker).
- Test passed on first run under `-race` against the real Postgres testcontainer + Chrome sidecar stack — no seam defect found; the composite handoff was already correctly wired, this test now guards it against regression.
- Full package suite (`go test ./cmd/server/ -race -count=1`) — 56 tests passed, zero regressions. `go vet ./cmd/server/` clean. `-short` compile-only run also green.
- The two existing E2E test files (`e2e_public_test.go`, `e2e_test.go`) are byte-unchanged — confirmed via `git diff --stat`.

## Task Commits

Each task was committed atomically:

1. **Task 1: Add the Flow B composite E2E test** - `94c6745` (test)

**Plan metadata:** (this SUMMARY's commit, following)

## Files Created/Modified
- `donnarec-api/cmd/server/e2e_flowb_composite_test.go` - New sibling E2E test file (package `main`) with `TestE2E_FlowBCompositePublicSubmitToIssued`, reusing the existing harness verbatim.

## Decisions Made
- No production-code changes needed — the composite handoff was already correctly wired (no seam defect surfaced). This is a coverage-only phase; the test's PASS on first run is the expected/desired outcome per the plan's characterization-test framing.
- Chose a distinct Checker keycloak subject (`77777777-...`) rather than reusing one from another test file, keeping the test self-contained and making the SoD precondition assertion explicit and easy to read.

## Deviations from Plan

None - plan executed exactly as written. The only addition beyond the plan's literal text was importing `db "github.com/donnarec/donnarec-api/internal/db/generated"` for `db.UserRoleEnumChecker`, which the plan's read_first section referenced implicitly (via `e2e_test.go`'s own import) but did not spell out in the action steps — a mechanical Go-compile requirement, not a scope change.

## Issues Encountered
None. The test compiled cleanly on first write and passed on first execution against the real Docker/testcontainers stack (Postgres 17 + Chrome sidecar).

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- v1.0 milestone audit WARNING-1 is closed. No other open items block the v1.0 milestone per `.planning/v1.0-MILESTONE-AUDIT.md`.
- No further phases queued for this milestone; this closes the audit gap phase.

---
*Phase: 07-close-gap-flow-b-composite-e2e-public-submit-through-approve*
*Completed: 2026-07-17*

## Self-Check: PASSED
- FOUND: donnarec-api/cmd/server/e2e_flowb_composite_test.go
- FOUND: commit 94c6745
- FOUND: this SUMMARY.md
