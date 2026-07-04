---
phase: "03"
plan: "06"
subsystem: "donation"
tags: [cancel, void-reissue, pii-reveal, search, maker-checker, audit, pdpa]
dependency_graph:
  requires: ["03-05"]
  provides: ["cancel", "reissue", "pii-reveal", "search-filter"]
  affects: ["donations", "audit_log", "receipt_numbers"]
tech_stack:
  added: []
  patterns:
    - "WithTx for multi-step cancel+reissue mutation (Pattern B)"
    - "AppendAuditEntryTx inside every state-change tx (D-13)"
    - "pii.CanRevealFull(claims) gate — audit written BEFORE returning plaintext (D-13)"
    - "Pointer-typed SearchDonationsParams fields (nil = NULL = skip filter)"
    - "checker2 in TestVoidAndReissue for SoD Approve on replacement draft"
key_files:
  created: []
  modified:
    - "internal/donation/model.go"
    - "internal/donation/service.go"
    - "internal/donation/handler.go"
    - "internal/db/queries/donations.sql"
    - "internal/db/generated/donations.sql.go"
    - "cmd/server/main.go"
    - "internal/donation/service_test.go"
    - "internal/donation/service_integration_test.go"
decisions:
  - "D-47: Cancel restricted to Checker+Admin with mandatory reason (min 1 char)"
  - "D-50: Void & Reissue cancels original + creates draft + links replaces/replaced_by; no number bypass"
  - "D-51: e-Donation keyed guard — cancel requires explicit RDConfirmationReason when edonation_keyed=true"
  - "D-46: PII reveal Checker+Admin only; every reveal audited; default masked everywhere"
  - "D-53: SearchDonations uses nullable *string/*DonationStatus so nil skips filter (IS NULL guard)"
metrics:
  duration: "~2h (across sessions)"
  completed_date: "2026-07-01"
  tasks_completed: 3
  files_modified: 8
---

# Phase 03 Plan 06: Cancel, Void-Reissue, PII Reveal, Search Summary

**One-liner:** Cancel/void-and-reissue with SoD-safe receipt retention, role-gated audited PII reveal, and nullable-filter search — all gap-less invariants preserved.

## Tasks Completed

| # | Name | Commit | Files |
|---|------|--------|-------|
| 1 (RED) | Cancel/Reissue/PII-Reveal model + failing tests | `331116d` | model.go, service_test.go, service_integration_test.go |
| 2 (GREEN) | Implement Cancel, Reissue, RevealPII, Search + handlers | `6d7463c` | service.go, handler.go, donations.sql, donations.sql.go, main.go |
| 3 (fix) | Fix TestVoidAndReissue SoD — add checker2 for Approve | `4266e85` | service_integration_test.go |

## What Was Built

### Cancel (FR-19, D-47)
- `svc.Cancel(ctx, id, CancelDonationRequest, claims)`: role gate (Checker/Admin) → mandatory reason → `SELECT FOR UPDATE` → state machine `canTransition("cancel")` → e-Donation keyed guard (D-51) → `CancelDonation` → `AppendAuditEntryTx("donation.cancel")` → decrypt+mask response.
- Receipt `receipt_number_id` and `receipt_formatted` are **retained** after cancel — no gap ever created.
- Handler: `POST /api/donations/:id/cancel` in checkerGroup; maps `ErrEDonationKeyedCancel→409`, `ErrForbidden→403`, `ErrMissingReason→422`.

### Void & Reissue (D-50)
- `svc.Reissue(ctx, originalID, ReissueDonationRequest, claims)`: role gate → reason check → encrypt tax ID → `WithTx[LockForUpdate(original) → CancelDonation → CreateDonation(draft) → SetReplacedBy(original) → SetReplaces(replacement) → AppendAuditEntryTx("donation.reissue")]`.
- New draft earns its receipt number only via normal Submit → Approve (no bypass).
- `SetReplaces` query added to `donations.sql` + generated.
- Handler: `POST /api/donations/:id/reissue` in checkerGroup; returns 201.

### e-Donation Keyed Guard (D-51)
- When `edonation_keyed=true` on the donation, cancel (and by extension reissue) requires non-empty `rd_confirmation_reason` in the request.
- Guard returns `ErrEDonationKeyedCancel` (mapped to 409) when the field is missing.
- Integration test `TestEDonationKeyedGuard_Integration` sets flag via raw SQL, verifies both the rejection and the success+audit path.

### PII Reveal (D-46)
- `svc.RevealPII(ctx, id, claims)`: `pii.CanRevealFull(claims)` gate → `WithTx[GetDonationByID → DecryptField(national/tax ID) → AppendAuditEntryTx("pii.reveal")]` → return `PIIRevealResponse`.
- Audit written **inside** the tx, **before** plaintext returned to caller — reveal cannot succeed without being audited.
- Handler: `GET /api/donations/:id/pii` in donationGroup (all staff); service returns ErrForbidden for Makers → 403.
- No PII in logs at any point.

### Search / Filter (D-53)
- `svc.Search(ctx, ListFilter, claims)` maps to `db.SearchDonationsParams` with nullable pointer fields: `DonorName *string`, `Status *DonationStatus`, `ReceiptNo *string`; nil passed as NULL skips the SQL filter.
- `List` handler upgraded to parse query params (`name`, `status`, `from`, `to`, `receipt_no`, `page`) and call `svc.Search()`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] SearchDonationsParams used non-nullable types preventing filter skip**
- **Found during:** Task 2 (GREEN)
- **Issue:** Generated `SearchDonations` params had `string`/`DonationStatus` (non-nullable); SQL uses `$N::TYPE IS NULL` to skip nil filters, requiring NULL propagation.
- **Fix:** Changed `SearchDonationsParams.DonorName`, `.Status`, `.ReceiptNo` to pointer types (`*string`, `*DonationStatus`) in generated code; updated `SearchDonations` function body accordingly.
- **Files modified:** `internal/db/generated/donations.sql.go`
- **Commit:** `6d7463c`

**2. [Rule 1 - Bug] TestVoidAndReissue SoD violation — same checker created and approved replacement**
- **Found during:** Test run after Task 2 (GREEN)
- **Issue:** `Reissue()` sets `created_by = actorUUID` (checker1). Test then called `svc.Approve(replacement.ID, checkerClaims)` with the same checker1 → ErrSoDViolation at line 1186.
- **Fix:** Added `checker2Row`/`checker2Claims` (second checker user); replaced `checkerClaims` with `checker2Claims` in the Approve call. Test comment corrected.
- **Files modified:** `internal/donation/service_integration_test.go`
- **Commit:** `4266e85`

## Test Results

```
30 passed, 0 failed (internal/donation/...)
```

All integration tests run with real PostgreSQL via testcontainers. Tests added/extended:
- `TestCancelRequiresReason` (unit)
- `TestCancelAuthCheckerAdminOnly` (unit)
- `TestEDonationKeyedGuard` (unit)
- `TestCancelRetainsReceiptNumber` (integration)
- `TestVoidAndReissue` (integration)
- `TestPII_RevealRequiresCheckerOrAdmin` (integration)
- `TestSearchDonations` (integration)
- `TestEDonationKeyedGuard_Integration` (integration)

## Known Stubs

None. All endpoints wire to real DB data; no mock/placeholder data flows to responses.

## Threat Flags

None new. All endpoints sit behind existing `RequireAuth()` + role guards. PII reveal adds a new GET endpoint but it is scoped to `donationGroup` (all-staff authenticated) with service-level Checker/Admin gate + mandatory audit — consistent with D-46 threat model entry.

## Self-Check: PASSED

- [x] `internal/donation/service.go` exists with Cancel/Reissue/RevealPII/Search methods
- [x] `internal/donation/handler.go` has Cancel/Reissue/RevealPII handlers
- [x] `cmd/server/main.go` has `/:id/cancel`, `/:id/reissue`, `/:id/pii` routes
- [x] Commits `331116d`, `6d7463c`, `4266e85` exist in log
- [x] 30/30 tests pass
