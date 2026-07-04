---
phase: 03
slug: donation-lifecycle-maker-checker-issuance
status: complete
nyquist_compliant: true
wave_0_complete: true
created: 2026-06-28
validated: 2026-07-04
---

# Phase 03 — Validation Strategy

> Per-phase validation contract, reconstructed from phase artifacts (PLAN/SUMMARY, 03-VERIFICATION.md, 03-UAT.md) and cross-referenced against the live test suite on 2026-07-04.
>
> **Result:** NYQUIST-COMPLIANT — every Phase-3 requirement has automated verification exercised by a committed test. Zero MISSING gaps. The human-UI half of the integration gate (Conventions.md) was completed via UAT (7/7, 2026-07-04).

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Backend framework** | `go test` + `testify` + `testcontainers-go` v0.43 (real Postgres) |
| **Backend config** | `donnarec-api/Makefile`, `donnarec-api/go.mod` |
| **Backend quick run** | `cd donnarec-api && make test-short` (unit only, no Docker) |
| **Backend integration** | `cd donnarec-api && make test-integration` (Docker/testcontainers) |
| **Backend full suite** | `cd donnarec-api && make test` (unit + integration) |
| **Backend real-path E2E** | `cd donnarec-api && go test -run TestE2E_MakerCheckerIssuancePipeline ./cmd/server/...` (Docker) |
| **Frontend framework** | Vitest |
| **Frontend config** | `donnarec-web/vitest.config.ts` |
| **Frontend quick/full run** | `cd donnarec-web && npm test` (= `vitest run`) |
| **Frontend build gate** | `cd donnarec-web && npm run build` + `npx tsc --noEmit` |
| **Estimated runtime** | unit ~seconds; integration/E2E ~1–3 min (testcontainers Postgres spin-up) |

---

## Sampling Rate

- **After every task commit:** `make test-short` (backend) + `npm test` (frontend)
- **After every plan wave:** `make test` (backend, Docker) + `npm test` + `npm run build` (frontend)
- **Before `/gsd:verify-work`:** Full suite green, including `TestE2E_MakerCheckerIssuancePipeline` (7/7) and the donation integration suite (all green)
- **Max feedback latency:** ~180 s (Docker integration path); ~10 s for the unit/quick path

---

## Per-Task Verification Map

Requirement → test file(s) → function(s). All Phase-3 requirements below are marked **Complete** in `.planning/REQUIREMENTS.md` and verified in `03-VERIFICATION.md`.

| Requirement | Behavior | Plans | Test File(s) | Key Test Function(s) | Type | Status |
|-------------|----------|-------|--------------|----------------------|------|--------|
| **FR-07** | Maker creates donation record (consent, mandatory tax ID, correct created_by) | 03-02, 03-03, 03-08, 03-13 | `internal/donation/service_integration_test.go`, `service_test.go`; `cmd/server/e2e_test.go` | `TestCreateDonation`, `TestConsentCapture`, `TestMandatoryTaxID`, `TestCreate_ActingUserIDWritesCorrectCreatedBy`, `TestE2E_…/HappyPath_CreateSubmitApproveList` | integration + E2E | ✅ green |
| **FR-09** | Maker edits while draft; return-to-draft | 03-01, 03-03, 03-04, 03-07, 03-08, 03-13 | `internal/donation/service_integration_test.go`; `cmd/server/e2e_test.go` | `TestEditDraft`, `TestReturnToDraft` | integration + E2E | ✅ green |
| **FR-10** | Search/filter by name, date, status, receipt no + paginated envelope | 03-06, 03-07, 03-09, 03-10 | `internal/donation/service_integration_test.go`; `web/lib/__tests__/donations.test.ts` | `TestSearchDonations`; FE envelope-shape regression suite (D-R2 `{data:{items,total,page,per_page}}`) | integration + FE unit | ✅ green |
| **FR-11** | Lifecycle state machine enforced (invalid transitions rejected) | 03-01, 03-03, 03-05, 03-07 | `internal/donation/service_integration_test.go` | `TestStateMachine_InvalidTransitions`, `TestSubmitMovesToPendingReview`, `TestRejectTerminal`, `TestReturnToDraft` | integration | ✅ green |
| **FR-12** | Return/Reject require mandatory reason | 03-05, 03-07, 03-11, 03-12 | `internal/donation/service_integration_test.go`; `cmd/server/e2e_test.go` | `TestMandatoryReason`, `TestRejectTerminal`, `TestCancelRequiresReason` | integration + E2E | ✅ green |
| **FR-14** | Checker approval; Segregation of Duties (approver ≠ creator); exactly-one under concurrency | 03-01, 03-05, 03-11, 03-12 | `internal/donation/service_integration_test.go`; `internal/auth/rbac_test.go`, `user_resolver_test.go`, `middleware_integration_test.go`; `cmd/server/e2e_test.go` | `TestSoD_ApproverCannotBeCreator`, `TestSoD_DBCheckConstraint`, `TestConcurrentApproval_ExactlyOneSucceeds`, `TestE2E_…/SoD_SelfApprove_403`, `…/RBAC_MakerRejectedFromCheckerOnlyRoute` | integration + E2E | ✅ green |
| **FR-19** | Cancel retains receipt number (never deleted); Void & Reissue; e-Donation-keyed guard | 03-01, 03-06, 03-11, 03-13 | `internal/donation/service_integration_test.go`; `cmd/server/e2e_test.go` | `TestCancelRetainsReceiptNumber`, `TestCancelAuthCheckerAdminOnly`, `TestVoidAndReissue`, `TestEDonationKeyedGuard`, `TestEDonationKeyedGuard_Integration`, `TestE2E_…/Cancel_RetainsReceiptNumber_RealPath` | integration + E2E | ✅ green |
| **FR-29** | PII encrypted at rest, masked by default, reveal restricted to Checker/Admin + audited | 03-01, 03-03, 03-06, 03-08, 03-11, 03-12 | `internal/donation/service_integration_test.go`; `internal/crypto/aes_gcm_test.go`, `keyprovider_test.go`; `internal/pii/mask_test.go`; `internal/audit/service_test.go` | `TestPII_TaxIDStoredEncrypted`, `TestPII_MaskDefault`, `TestPII_RevealRequiresCheckerOrAdmin`, `TestEnvelopeRoundTrip`, `TestAESGCMRoundTrip`, `TestBlindIndex`, `TestMaskNationalID`, `TestCanRevealFull`, `TestPIIRevealAudit` | integration + unit | ✅ green |

### Supporting invariants exercised by the Phase-3 issuance path

| Ref | Behavior | Test File(s) | Key Test Function(s) | Status |
|-----|----------|--------------|----------------------|--------|
| **NFR-04** (Phase 2, reused) | Gap-less, concurrency-safe, per-fiscal-year receipt numbering; rollback leaves no gap | `internal/receiptno/allocator_concurrency_test.go`, `allocator_rollback_test.go`, `allocator_test.go`, `fiscalyear_test.go`; `internal/donation/service_integration_test.go` | `TestAllocator_Concurrency`, `TestAllocator_SequentialGapless`, `TestAllocator_Rollback`, `TestAllocator_RollbackMixedSequence`, `TestAllocator_NewFiscalYearStartsAtOne`, `TestAllocator_UniqueConstraintBackstop`, `TestConcurrentApproval_ExactlyOneSucceeds`, `TestIssuanceTransaction_RollbackOnError` | ✅ green |
| **NFR-05** | Append-only audit log, hash-chain immutability | `internal/audit/immutability_test.go`, `service_test.go`, `concurrent_test.go`, `middleware_test.go` | `TestAuditImmutability`, `TestHashChainVerification`, `TestConcurrentAuditInserts`, `TestAuditMiddlewareCoverage`, `TestAuditMiddlewareNoAbortOnError` | ✅ green |
| **NFR-02** | App-level AES-256-GCM envelope encryption at rest | `internal/crypto/aes_gcm_test.go`, `keyprovider_test.go` | `TestEnvelopeRoundTrip`, `TestEncryptKeyLength`, `TestEnvKeyProvider`, `TestBlindIndex` | ✅ green |
| **D-R1** (trust boundary) | Keycloak token never reaches the browser (BFF proxy) | `web/app/api/bff/donations/__tests__/bff-routes.test.ts` | 8 hermetic BFF tests incl. no-token-leak + 401 gate + field mapping | ✅ green |
| **Seam gate** (Conventions.md) | Real HTTP path: router → RequireAuth → RequireRoles/ResolveAppUser → handler → service → DB with real Keycloak-shaped token | `cmd/server/e2e_test.go` | `TestE2E_MakerCheckerIssuancePipeline` (7 subtests incl. `UnprovisionedSubject_403`, `Audience_WrongClient_401`) | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements. Go (`testify` + `testcontainers-go`) and Vitest were already installed from Phases 1–2; no new framework install was needed. The donation-specific test files (`service_integration_test.go`, `e2e_test.go`, `bff-routes.test.ts`, `donations.test.ts`) were added within Phase 3.

---

## Manual-Only Verifications

The human-UI half of the Conventions.md integration gate — behaviors that require a live browser + live Keycloak session and cannot be grepped or asserted by the httptest-only E2E. **All were performed and passed in the UAT walkthrough on 2026-07-04 (7/7 — see `03-UAT.md`); listed here for the record, not as open gaps.**

| Behavior | Requirement | Why Manual | Result (03-UAT.md) |
|----------|-------------|------------|--------------------|
| Full-stack Maker→Checker handoff (two distinct Keycloak users; create→submit→approve→gap-less receipt) | FR-07/09/14/19 | Live Keycloak login + real cross-user handoff | ✅ Test 2 pass |
| Token absence in browser (DevTools Network) | D-R1 / NFR-02 | Live network inspection | ✅ Test 3 pass |
| SoD-blocked buttons absent from DOM (not disabled) | FR-14 | Server-computed `viewer_is_creator` reflected in real DOM | ✅ Test 4 pass |
| PII reveal UX round-trip (mask → reveal → re-mask) + audit row | FR-29 | Session-only client state + live audit side-effect | ✅ Test 5 pass |
| Filter/pagination interaction + Thai/English i18n rendering | FR-10 | Visual + interactive client state | ✅ Test 6 pass |
| Cancel / Void & Reissue dialogs incl. `edonation_keyed` guard | FR-19 | Multi-step dialog + conditional field rendering | ✅ Test 7 pass |
| Cold-start full-stack smoke (boot + auth redirect) | — | Live stack bring-up | ✅ Test 1 pass |

> **NFR-03** (data-retention / PDPA-vs-tax residency) is **not a Phase-3 deliverable** — it is scoped to Phase 1 (model) and Phase 6 (donor request) and remains Pending there. Out of scope for this validation.

---

## Validation Sign-Off

- [x] All Phase-3 requirements (FR-07, 09, 10, 11, 12, 14, 19, 29) have automated verification
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references (none — existing infra sufficient)
- [x] No watch-mode flags (all commands are `run`-mode: `go test`, `vitest run`)
- [x] Feedback latency < 180 s (Docker integration path)
- [x] `nyquist_compliant: true` set in frontmatter
- [x] Human-UI integration-gate half completed via UAT 7/7 (2026-07-04)

**Approval:** approved 2026-07-04

---

## Validation Audit 2026-07-04

| Metric | Count |
|--------|-------|
| Requirements audited | 8 (FR) + 3 supporting (NFR-02/04/05) + 2 gate refs (D-R1, seam) |
| Gaps found | 0 |
| Resolved (auto-generated tests) | 0 (all pre-existing) |
| Escalated to manual-only | 0 open (7 human-UI items already passed via UAT) |

*Reconstructed from a scaffold-only VALIDATION.md; the placeholder template was replaced with the concrete requirement→test map above. No new test files were generated — the phase was already fully covered.*
