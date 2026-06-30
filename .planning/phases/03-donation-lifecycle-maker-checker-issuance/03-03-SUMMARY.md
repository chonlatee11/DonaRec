---
phase: 03-donation-lifecycle-maker-checker-issuance
plan: "03"
subsystem: api
tags: [go, gin, postgres, pgx, aes-256-gcm, pdpa, pii, state-machine, rbac, tdd]

requires:
  - phase: 03-01
    provides: sqlc-generated donation queries (CreateDonation, GetDonationByID, UpdateDraftDonation, SubmitDonation, LockDonationForUpdate), migrations 000005/000007, testutil.SetupTestPostgres
  - phase: 01-foundation
    provides: internal/crypto (EncryptField/DecryptField/NewEnvKeyProvider), internal/pii (MaskNationalID/CanRevealFull), internal/audit (AuditService/AppendAuditEntryTx), internal/auth (KeycloakClaims/RequireRoles/RoleMaker/RoleChecker/RoleAdmin), internal/db helpers (WithTx)
  - phase: 02-gap-less-receipt-numbering-core
    provides: receiptno.NewAllocator (SELECT FOR UPDATE gap-less counter)

provides:
  - DonationService.Create — AES-256-GCM encrypt donor tax ID, mask in response, mandatory tax ID (D-44), consent snapshot (D-49)
  - DonationService.GetByID — decrypt → mask only; plaintext never in response (T-03-09)
  - DonationService.UpdateDraft — re-encrypt on every update; ErrInvalidTransition if not draft
  - DonationService.Submit — draft → pending_review transition with row lock (D-45)
  - DonationService.List — paginated masked list; full filter wiring deferred to 03-06
  - canTransition helper — single D-45 state machine source of truth
  - DonationHandler (Create/GetByID/Update/Submit/List) — Pattern A claims extraction, Pattern C no-PII logs, audit_after markers
  - /api/donations maker route group — RequireRoles(Maker|Checker|Admin) guard
  - cmd/server/main.go wiring — keyProvider, allocator, donationSvc, donationHandler constructed and injected

affects: [03-04, 03-05, 03-06, 03-07, 03-08]

tech-stack:
  added: []
  patterns:
    - "Pattern A: claims extraction — verbatim block in every handler"
    - "Pattern C: no-PII logs — donation_id + operation UUID only, never donor name/tax_id"
    - "Pattern B: WithTx wrapper — all multi-step mutations inside dbhelpers.WithTx"
    - "D-44: mandatory tax ID — fail fast before any DB call if DonorTaxID is empty"
    - "D-45: canTransition helper — single state machine source, submit/update only here; approve/reject/cancel in 03-05/06"
    - "D-49: consent snapshot — consent_given/at/text_version/purpose captured per-donation on create and update"
    - "T-03-08: EncryptField before any DB write — plaintext never reaches Postgres"
    - "T-03-09: DonationResponse exposes DonorTaxIDMasked only — DonorTaxIDEnc never in response struct"

key-files:
  created:
    - donnarec-api/internal/donation/model.go
    - donnarec-api/internal/donation/errors.go
    - donnarec-api/internal/donation/service.go
    - donnarec-api/internal/donation/handler.go
    - donnarec-api/internal/donation/service_test.go
    - donnarec-api/internal/donation/service_integration_test.go
  modified:
    - donnarec-api/cmd/server/main.go

key-decisions:
  - "D-44 enforced at service layer: ErrMissingTaxID returned before any DB call when DonorTaxID is empty"
  - "D-45 canTransition is the sole state machine source — submit/update arms here; approve/return/reject/cancel wired in 03-05/06"
  - "D-49 consent captured per-donation snapshot on Create and UpdateDraft"
  - "T-03-08/09/10: app-level AES-256-GCM encrypt before write; mask-only in response; no PII ever in logs"
  - "Handler error mapping: ErrInvalidTransition→409, ErrMissingTaxID→422, ErrForbidden→403, ErrNotFound→404, default→500"
  - "Route group /api/donations guarded by RequireRoles(Maker|Checker|Admin) — checker/cancel/slip/reveal routes deferred to 03-04/05/06"
  - "List uses raw pool.Query for 03-03 baseline; full SearchDonations sqlc wiring in 03-06"

patterns-established:
  - "DonationHandler follows users/handler.go patterns exactly — verbatim Pattern A claims block, Pattern C log discipline"
  - "All sentinel errors are package-level vars in errors.go; mapped to HTTP codes only at handler layer"
  - "audit_after set on every handler success — AuditMiddleware captures the after-state for immutable trail"

requirements-completed: [FR-07, FR-09, FR-11, FR-29]

duration: multi-session (Tasks 1+2 prior sessions; Task 3 this session)
completed: 2026-07-01
---

# Phase 03 Plan 03: Maker Create/Edit Draft Slice Summary

**Donation service + Gin handlers: AES-256-GCM PII encryption, mandatory tax ID (D-44), consent snapshot (D-49), state machine guard (D-45), and maker route group wired under RBAC**

## Performance

- **Duration:** multi-session (Tasks 1+2 completed prior; Task 3 this session)
- **Tasks:** 3/3
- **Files modified:** 7 (5 created, 2 modified)

## Accomplishments

- Donor tax/national ID stored as AES-256-GCM ciphertext (DEK/KEK envelope); plaintext never reaches Postgres; masked in all responses (DonorTaxIDMasked — last-4 reveal)
- State machine guard enforced via canTransition — draft-only edit/submit; ErrInvalidTransition (409) for invalid transitions; row-level lock ensures concurrency safety
- Mandatory tax ID (D-44) and per-donation consent snapshot (D-49) enforced at service layer before any DB call
- Gin handlers (Create/GetByID/Update/Submit/List) follow Pattern A/C exactly; audit_after markers set for AuditMiddleware
- `/api/donations` route group registered in main.go under RequireRoles(Maker|Checker|Admin); full service wiring pool→queries→keyProvider→allocator→donationSvc→donationHandler

## Task Commits

Each task was committed atomically:

1. **Task 1 RED: failing tests for create/PII/consent/mandatory taxID** — `ebf009c` (test)
2. **Task 1 GREEN: Create/GetByID with PII encrypt+mask, mandatory taxID, consent** — `f740c46` (feat)
3. **Task 2 RED: failing tests for submit/updateDraft/list** — `487b63c` (test)
4. **Task 2 GREEN: UpdateDraft/Submit/List with state machine guard** — `b4071bc` (feat)
5. **Task 3: Gin handlers + maker route group wiring** — `4776db8` (feat)

_Note: TDD plan — RED commits precede each GREEN commit per TDD gate protocol_

## Files Created/Modified

- `donnarec-api/internal/donation/model.go` — CreateDonationRequest, UpdateDraftRequest, DonationResponse (DonorTaxIDMasked only), ListFilter
- `donnarec-api/internal/donation/errors.go` — sentinel errors (ErrMissingTaxID, ErrInvalidTransition, ErrSoDViolation, ErrMissingReason, ErrNotFound, ErrForbidden, ErrEDonationKeyedCancel)
- `donnarec-api/internal/donation/service.go` — DonationService with Create/GetByID/UpdateDraft/Submit/List, canTransition helper, donationRowToResponse, numericStr/dateStr helpers
- `donnarec-api/internal/donation/handler.go` — DonationHandler with Create/GetByID/Update/Submit/List; Pattern A/C; sentinel error → HTTP mapping
- `donnarec-api/internal/donation/service_test.go` — unit tests: TestMandatoryTaxID, TestCreateDonation, TestPII_MaskDefault, TestConsentCapture, TestEditDraft, TestStateMachine_InvalidTransitions, TestSubmitMovesToPendingReview; stubs for 03-05/06
- `donnarec-api/internal/donation/service_integration_test.go` — integration tests: TestPII_TaxIDStoredEncrypted (DB-level PII assertion); testcontainers-backed
- `donnarec-api/cmd/server/main.go` — added crypto.NewEnvKeyProvider, receiptno.NewAllocator, donation.NewDonationService, donation.NewDonationHandler, /api/donations route group

## Decisions Made

- **ErrMissingTaxID at service layer, not handler validation only:** Handler validates format (len=13,numeric via go-playground/validator); service also checks empty as defense-in-depth per D-44
- **canTransition as pure helper:** Encodes D-45 state machine; submit/update arms here; approve/return/reject/cancel arms wired in 03-05/06 — future plans extend the switch, not rewrite it
- **List uses raw pool.Query in 03-03:** SearchDonations sqlc query exists but full filter wiring (ILIKE, date range, status) deferred to 03-06 where the filter struct is fully populated
- **Allocator injected but not called in 03-03:** Constructor receives allocator for future Approve call in 03-05; no allocator usage in this plan's methods

## Deviations from Plan

None — plan executed exactly as written. The two pre-existing `t.Fatal("not implemented")` test stubs (`TestMandatoryReason`, `TestEDonationKeyedGuard`) are intentional scaffolds for plans 03-05 and 03-06 respectively; they pre-date Task 3 and are explicitly documented in the test file as deferred.

## Known Stubs

- `DonationService.List` uses a raw `pool.Query` instead of `qtx.SearchDonations` — full filter parameter wiring (ILIKE donor name, date range, status filter, receipt number) is deferred to plan 03-06. Current implementation returns all records ordered by `created_at DESC` with limit/offset defaults. This is intentional per plan 03-03 scope.

## Issues Encountered

- Pre-existing `TestMandatoryReason` and `TestEDonationKeyedGuard` tests fail with `t.Fatal("not implemented...")` — these are Wave 0 scaffolds created in plan 03-01 for plans 03-05/06. Not caused by this plan; not fixed (out of scope per deviation rules scope boundary).

## User Setup Required

None — no external service configuration required beyond DONAREC_KEK (existing env var from Phase 1).

## Next Phase Readiness

- 03-04 (slip upload): DonationHandler struct ready to receive `storageSvc *storage.Client` field addition; route `POST /:id/slip` not yet registered
- 03-05 (checker approve/return/reject): ErrSoDViolation, ErrMissingReason sentinels defined; canTransition arms for approve/return/reject to be added; TestMandatoryReason scaffold in place
- 03-06 (cancel + filters): ErrEDonationKeyedCancel sentinel defined; TestEDonationKeyedGuard scaffold in place; List filter wiring to SearchDonations sqlc query ready to be completed
- 03-08 (maker UI): `/api/donations` REST endpoints fully operational

---
*Phase: 03-donation-lifecycle-maker-checker-issuance*
*Completed: 2026-07-01*
