---
phase: 03-donation-lifecycle-maker-checker-issuance
verified: 2026-07-01T00:00:00Z
status: human_needed
score: 5/5 must-haves verified
overrides_applied: 0
human_verification:
  - test: "Run integration tests with Docker available"
    expected: "All tests in internal/donation/service_integration_test.go pass including TestConcurrentApproval_ExactlyOneSucceeds (N=5, -race), TestIssuanceTransaction_RollbackOnError, TestSoD_DBCheckConstraint, TestCancelRetainsReceiptNumber, TestVoidAndReissue, TestPII_RevealRequiresCheckerOrAdmin"
    why_human: "Tests require Docker/testcontainers (skip with -short). Cannot verify without live Postgres container."
  - test: "View donation list page in browser at /donations"
    expected: "Table shows donations with Thai text, masked national IDs, status badges, date ranges. Filter bar with name/status/date/receipt_no works. Pagination works."
    why_human: "Visual rendering and i18n (Thai/English) cannot be verified programmatically."
  - test: "View donation detail page as Checker for a record created by a different Maker"
    expected: "Review action panel shows: Approve, Return (ตีกลับแก้ไข), Reject (ปฏิเสธถาวร) buttons. Reason dialog appears with textarea before confirm."
    why_human: "Role-based UI branching via viewer_is_creator requires live Keycloak session."
  - test: "View donation detail page as Checker for a record they created (SoD)"
    expected: "SoDBlockedAlert renders. Approve/Return/Reject buttons are absent from DOM (not just disabled)."
    why_human: "SoD blocked state requires live Keycloak session with matching created_by."
  - test: "PII reveal flow in browser"
    expected: "Masked ID shows (e.g. x-xxxx-xxxxx-x0123). Clicking reveal button shows full national ID. Reloading the page re-masks it."
    why_human: "Session-only reveal state and audit write require live session."
  - test: "Cancel issued receipt with edonation_keyed=true via UI"
    expected: "Cancel dialog shows rd_confirmation_reason field with warning text. Submit without filling in rd_confirmation_reason is blocked. Filling in the field and submitting sets status=cancelled."
    why_human: "Dialog interaction and warning display requires manual browser testing."
---

# Phase 3: Donation Lifecycle & Maker-Checker Issuance — Verification Report

**Phase Goal:** A Maker can create and submit a donation record with encrypted donor details, a Checker (who is never the Maker) can approve or return it with a reason, and approval issues a numbered receipt in one atomic transaction.

**Verified:** 2026-07-01

**Status:** HUMAN_NEEDED — all 5 success criteria pass automated checks. 6 items require human/Docker verification (integration tests + visual/session-dependent UI flows).

**Re-verification:** No — initial verification.

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Maker can create draft, edit while draft, view attached slip, submit; lifecycle draft → pending_review → issued/rejected/cancelled is explicit and enforced | VERIFIED | `service.go:81-98` canTransition() is single source of truth; `UpdateDraft` guards with `canTransition("update")`; `Submit` guards with `canTransition("submit")`; `slip_handler.go` + `storage/client.go` implement MinIO upload with presigned URL; migration `000005` defines `donation_status` enum with 5 values + REVOKE DELETE |
| 2 | Checker can approve or return with a mandatory reason on return; server blocks a user approving a record they created (SoD) in service code AND DB CHECK constraint | VERIFIED | `service.go:564-567` `locked.CreatedBy == approverUUID → ErrSoDViolation`; `migrations/000005_donations.up.sql:101-103` `CONSTRAINT chk_sod_approver CHECK (approved_by IS NULL OR approved_by != created_by)`; `Return()` and `Reject()` both check `strings.TrimSpace(reason) == "" → ErrMissingReason`; integration tests `TestSoD_ApproverCannotBeCreator` and `TestSoD_DBCheckConstraint` |
| 3 | On approval, a SINGLE DB transaction sets status=issued, allocates gap-less number (via Phase 2 allocator, not SEQUENCE), writes audit row, and enqueues outbox job; receipt number exists ONLY for issued/cancelled records; no PDF/email in the tx | VERIFIED | `service.go:543-623` Approve() is a single `dbhelpers.WithTx` closure with 7 ordered steps (lock, status check, SoD, `s.allocator.Allocate(ctx, tx, ...)`, `qtx.IssueDonation`, `auditSvc.AppendAuditEntryTx`, `qtx.EnqueueOutboxJob`); `migrations/000005:104-110` `CONSTRAINT chk_receipt_only_on_issued_or_cancelled` enforces receipt_number_id IS NOT NULL IFF status IN ('issued','cancelled'); outbox comment: "Do NOT render PDF or send email here" (`service.go:610`); integration test `TestIssuanceTransaction_RollbackOnError` (3 scenarios) + `TestOutboxAtomicity` |
| 4 | Cancelling an issued receipt sets status "ยกเลิก"/cancelled and RETAINS its number (no gap, never deleted); action audited | VERIFIED | `service.go:812-919` Cancel() does NOT null out receipt_number_id or receipt_formatted; `CancelDonation` SQL only updates status/cancelled_by/cancelled_at/cancel_reason; `chk_receipt_only_on_issued_or_cancelled` CHECK allows non-null receipt on cancelled; `AppendAuditEntryTx` called in same TX; `D-51` e-Donation keyed guard enforced; integration test `TestCancelRetainsReceiptNumber` asserts receipt_number_id stays non-null + B.running_no = A.running_no + 1 |
| 5 | Donor national/tax ID encrypted at rest (AES-256-GCM); masked everywhere except authorized audited reveals; staff can search/filter by name, date range, status, receipt number (NOT tax ID) | VERIFIED | `service.go:123-125` `crypto.EncryptField` called before any DB write; `migrations/000005:57-58` `donor_tax_id_enc BYTEA NOT NULL, donor_tax_id_dek BYTEA NOT NULL`; all responses use `pii.MaskNationalID`; `RevealPII()` checks `pii.CanRevealFull(claims)` + audits before returning plaintext; `Search()` excludes tax ID filter (D-53, `model.go:88-98`); `List` handler calls `svc.Search()` with name/status/from_date/to_date/receipt_no params (`handler.go:435-483`); integration tests `TestPII_TaxIDStoredEncrypted`, `TestPII_RevealRequiresCheckerOrAdmin`, `TestPII_MaskDefault`, `TestSearchDonations` |

**Score:** 5/5 truths verified

---

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/donation/service.go` | Core lifecycle + issuance TX | VERIFIED | 1375 lines; Create, UpdateDraft, Submit, Approve (7-step TX), Return, Reject, Cancel, Reissue, RevealPII, Search all implemented substantively |
| `internal/donation/model.go` | Request/response types + ListFilter | VERIFIED | DonationResponse never exposes plaintext tax ID (only DonorTaxIDMasked); ListFilter excludes TaxID; PIIRevealResponse separate type |
| `internal/donation/handler.go` | HTTP handlers wired | VERIFIED | All endpoints present; `List` delegates to `svc.Search` with query params; error sentinel → HTTP status mapping correct |
| `internal/donation/errors.go` | Sentinel errors | VERIFIED | ErrMissingTaxID, ErrInvalidTransition, ErrSoDViolation, ErrMissingReason, ErrNotFound, ErrForbidden, ErrEDonationKeyedCancel defined |
| `migrations/000005_donations.up.sql` | Donations table + DB constraints | VERIFIED | `donation_status` enum; 5-state lifecycle; `chk_sod_approver` CHECK; `chk_receipt_only_on_issued_or_cancelled` CHECK; REVOKE DELETE; FR-10 search indexes |
| `migrations/000006_slip_attachments.up.sql` | Slip attachments table | VERIFIED | SlipAttachment table with soft-delete (deleted_at) per D-54 |
| `migrations/000007_outbox_jobs.up.sql` | Outbox jobs table | VERIFIED | `outbox_jobs` with status CHECK + partial index for Phase 4 worker polling |
| `internal/storage/client.go` | MinIO storage + magic-byte validation | VERIFIED | `validateSlip()` uses `gabriel-vasile/mimetype` magic bytes; size limit 10MB; presigned URLs 15-min TTL; `ErrUnsupportedFileType`, `ErrFileTooLarge` |
| `internal/donation/service_integration_test.go` | Integration tests for all 7 hardest invariants | VERIFIED | 1576 lines; TestIssuanceTransaction_RollbackOnError (3 scenarios), TestOutboxAtomicity, TestSoD_ApproverCannotBeCreator, TestSoD_DBCheckConstraint, TestConcurrentApproval_ExactlyOneSucceeds (N=5), TestReturnToDraft, TestRejectTerminal, TestCancelRetainsReceiptNumber, TestVoidAndReissue, TestPII_TaxIDStoredEncrypted, TestPII_RevealRequiresCheckerOrAdmin, TestPII_MaskDefault, TestSearchDonations, TestEDonationKeyedGuard_Integration — all skip with `-short` (require Docker) |
| `donnarec-web/app/donations/page.tsx` | Donation list with search/filter | VERIFIED (build passes) | Next.js build successful; 13.6 kB bundle |
| `donnarec-web/app/donations/[id]/page.tsx` | Donation detail + review actions | VERIFIED (build passes) | 479-line Server Component with all 5 server actions (approve, return, reject, cancel, reissue); slip view; PII reveal; replace chain |
| `donnarec-web/components/ReviewActionPanel.tsx` | Checker action buttons + SoD blocked state | VERIFIED (build passes) | 4 cases: Maker's draft, SoD blocked, Checker review, Cancel/Reissue; buttons absent from DOM (not disabled) when SoD blocked |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `service.go:Approve` | `receiptno.Allocator.Allocate` | `s.allocator.Allocate(ctx, tx, approvedAt.Time)` — passes live `tx` | WIRED | `service.go:573-575`; only call site in codebase (D-35) |
| `service.go:Approve` | `audit.AppendAuditEntryTx` | inside same `WithTx` closure | WIRED | `service.go:595-607`; failure rolls back entire TX (not best-effort) |
| `service.go:Approve` | `outbox_jobs` (INSERT) | `qtx.EnqueueOutboxJob` inside `WithTx` | WIRED | `service.go:609-616`; job_type="issue_receipt" with donation_id payload |
| `handler.go:List` | `service.go:Search` | `h.svc.Search(ctx, filter, claims)` with parsed query params | WIRED | `handler.go:479`; name/status/from_date/to_date/receipt_no parsed from query |
| `service.go:RevealPII` | `audit.AppendAuditEntryTx` | inside `WithTx` closure before plaintext returned | WIRED | `service.go:1168-1181`; D-13 enforced — audit before reveal |
| `main.go:checkerGroup` | `donation.Handler` reviewer endpoints | `checkerGroup.Use(auth.RequireRoles(checker, admin))` | WIRED | `main.go:247-253`; approve/return/reject/cancel/reissue all require Checker+Admin |
| `donations/[id]/page.tsx` | Go API `/api/donations/:id` | `getDonation(id)` → server action → fetch | WIRED | `[id]/page.tsx:46-53`; all 5 server actions (`approve`, `returnForEdit`, `reject`, `cancelDonation`, `reissueDonation`) wired |

---

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `service.go:Approve` | `issuedRow` | `qtx.GetDonationByID` inside TX after IssueDonation | DB query, returns committed state | FLOWING |
| `service.go:Search` | `rows` | `s.queries.SearchDonations(ctx, params)` — sqlc generated query | DB query with ILIKE/date/status/receipt filters | FLOWING |
| `service.go:RevealPII` | `plaintext` | `crypto.DecryptField(ctx, s.keyProvider, row.DonorTaxIDEnc, row.DonorTaxIDDek)` | AES-256-GCM decryption of real ciphertext from DB | FLOWING |
| `storage/client.go:PutSlip` | `objectKey` | MinIO `client.PutObject` with `io.MultiReader` reassembly | Real object storage write after magic-byte validation | FLOWING |

---

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Go backend compiles | `cd donnarec-api && go build ./...` | exit 0 | PASS |
| Unit tests (no Docker) | `cd donnarec-api && go test -short ./internal/donation/...` | 5 passed | PASS |
| Frontend build | `cd donnarec-web && npm run build` | exit 0; 6 routes generated | PASS |
| Integration tests (Docker required) | `go test ./internal/donation/...` without `-short` | SKIP — requires Docker/testcontainers | SKIP (human verification item #1) |

---

### Probe Execution

No probe scripts found under `scripts/*/tests/probe-*.sh`. Step 7c: SKIPPED (no probe scripts).

---

### Requirements Coverage

| Requirement | Description (from context) | Status | Evidence |
|-------------|---------------------------|--------|----------|
| FR-07 | Maker creates donation record | SATISFIED | `service.go:Create()` + `handler.go:Create` + `POST /api/donations` |
| FR-09 | Maker edits while draft | SATISFIED | `service.go:UpdateDraft()` + `canTransition("update")` guard + `PUT /api/donations/:id` |
| FR-10 | Search/filter by name, date, status, receipt no | SATISFIED | `service.go:Search()` + `handler.go:List` parses query params; tax ID excluded per D-53 |
| FR-11 | Lifecycle state machine enforced | SATISFIED | `canTransition()` single source of truth; 5-state enum; `LockDonationForUpdate` in each TX |
| FR-12 | Return (non-terminal) and Reject (terminal) with mandatory reason | SATISFIED | `service.go:Return()` + `Reject()`; both require non-empty reason; Reject has `canTransition("reject")` guard preventing further transitions |
| FR-14 | Checker approval; SoD approver != creator | SATISFIED | `service.go:Approve():564-567` code guard + `migrations/000005:101-103` DB CHECK `chk_sod_approver` |
| FR-19 | Cancel issued receipt: retains number, audited; no hard-delete | SATISFIED | `service.go:Cancel()` does not null receipt fields; `chk_receipt_only_on_issued_or_cancelled` allows non-null on cancelled; REVOKE DELETE on donations table |
| FR-29 | PII encrypted at rest, masked by default, role-gated reveal, audited | SATISFIED | `crypto.EncryptField` before DB write; `pii.MaskNationalID` on all responses; `pii.CanRevealFull` gate; `AppendAuditEntryTx` for `pii.reveal` |

---

### Load-Bearing Invariant Confirmation

| Invariant | Verified | Evidence |
|-----------|----------|----------|
| Issuance TX is atomic (all-or-nothing) | YES | `service.go:543-623` single `WithTx` closure; 3 rollback scenarios tested in `TestIssuanceTransaction_RollbackOnError` |
| Concurrency test exists + fires N parallel approvals asserting zero gaps / exactly one issued | YES (exists; Docker-gated) | `service_integration_test.go:492-579` `TestConcurrentApproval_ExactlyOneSucceeds` N=5 goroutines via `errgroup`, asserts exactly 1 success + exactly 1 `receipt_numbers` row; runs under `-race`; skips without Docker |
| SoD: approver_id != created_by in service code | YES | `service.go:565`: `if locked.CreatedBy == approverUUID { return ErrSoDViolation }` |
| SoD: approver_id != created_by as DB CHECK | YES | `migrations/000005:101-103`: `CONSTRAINT chk_sod_approver CHECK (approved_by IS NULL OR approved_by != created_by)` |
| Receipt number null on draft/pending/rejected; non-null only on issued/cancelled | YES | `migrations/000005:104-110`: `CONSTRAINT chk_receipt_only_on_issued_or_cancelled` |
| national/tax ID stored as ciphertext (never plaintext to DB) | YES | `service.go:123-125`: `crypto.EncryptField` before `CreateDonation`; columns `donor_tax_id_enc BYTEA NOT NULL, donor_tax_id_dek BYTEA NOT NULL` |
| Blind index NOT used this phase (D-43 snapshot-only) | YES | No blind index in migrations 000005–000007; `ListFilter` has no TaxID field; `Search()` excludes tax ID |
| Outbox ONLY enqueued this phase (no PDF/email in issuance TX) | YES | `service.go:610`: comment "Do NOT render PDF or send email here"; `EnqueueOutboxJob` only inserts a row with `job_type="issue_receipt"` |
| Every action audited (audit in TX, not best-effort) | YES | All TX closures call `auditSvc.AppendAuditEntryTx` inside `WithTx`; failure rolls back entire TX |

---

### Anti-Patterns Found

No blockers found in phase-modified files:
- No `TBD`, `FIXME`, or `XXX` markers in `internal/donation/`, `internal/storage/`, migrations 000005–000007, or frontend components.
- No `return nil` / `return []` stub returns in service or handler.
- `service.go:List` method uses a raw query for the basic list (03-03 plan) while `Search` uses the sqlc `SearchDonations` query — both are substantive and the handler always calls `Search`. The `List` raw-query code is dead code since the handler delegates to `Search`, but it is not a stub (it implements the same logic). This is a minor code smell (INFO) but does not block the goal.

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/donation/service.go` | 437–514 | `List()` method uses raw query but handler calls `Search()` — `List()` is effectively dead code | INFO | None on goal; `Search()` is called by handler |

---

### Human Verification Required

#### 1. Integration Tests (Docker)

**Test:** Run `cd donnarec-api && go test ./internal/donation/... -v -race` with Docker available (removes `-short` flag).

**Expected:** All 14 integration tests pass including:
- `TestConcurrentApproval_ExactlyOneSucceeds` (N=5, -race): exactly 1 success, 4 ErrInvalidTransition, 1 receipt_numbers row
- `TestIssuanceTransaction_RollbackOnError` (Scenarios A, B, C): rollbacks leave 0 ledger rows; happy path commits all 7 effects
- `TestSoD_DBCheckConstraint`: raw UPDATE with approved_by = created_by triggers SQLSTATE 23514 on `chk_sod_approver`
- `TestCancelRetainsReceiptNumber`: B.running_no = A.running_no + 1 after cancel of A
- `TestVoidAndReissue`: replacement starts at draft, original retains receipt number, 2 ledger rows total

**Why human:** Requires Docker daemon to start Postgres testcontainer. Tests skip with `-short`.

---

#### 2. Donation List Page Visual

**Test:** Open `/donations` in browser as any staff role.

**Expected:** Table renders with Thai column headers, status badges (colour-coded), masked national ID format `x-xxxx-xxxxx-xNNNN`, pagination. Filter bar allows filtering by donor name, status, date range, receipt number.

**Why human:** Visual rendering, i18n (Thai/English toggle), and layout quality cannot be verified programmatically.

---

#### 3. Checker Review Panel (not own record)

**Test:** Log in as Checker B. Open a donation created by Maker A.

**Expected:** Right panel shows three buttons: อนุมัติ (blue), ตีกลับแก้ไข (outline), ปฏิเสธถาวร (destructive). Clicking ตีกลับแก้ไข opens a dialog with a textarea for reason. Reason field is required before confirm.

**Why human:** Requires live Keycloak session with two distinct users and correct `viewer_is_creator` propagation from API.

---

#### 4. SoD Blocked State (own record)

**Test:** Log in as Checker. Create a donation yourself (or have it assigned to your user ID as creator). Open the detail page.

**Expected:** Right panel shows `SoDBlockedAlert` warning only. Approve/Return/Reject buttons are absent from the DOM entirely (not just disabled).

**Why human:** `viewer_is_creator` value comes from API comparing `created_by` to `claims.Subject`; requires a live Keycloak session to verify the flag is correctly set.

---

#### 5. PII Reveal UX

**Test:** Open a donation detail page as Checker/Admin. National ID shows masked (e.g. `x-xxxx-xxxxx-x0123`). Click reveal button.

**Expected:** Plaintext national ID replaces masked value in the UI. Reloading the page re-masks it. Check audit_log table: one `pii.reveal` row created.

**Why human:** Session-only reveal state and server-side audit write require live browser session.

---

#### 6. Cancel with edonation_keyed=true Dialog

**Test:** Set `edonation_keyed = true` on an issued donation via DB. Open detail page as Checker. Click ยกเลิกใบเสร็จ.

**Expected:** CancelDialog shows warning about RD system. The rd_confirmation_reason field is required. Attempting to submit without it should be blocked. With a reason, cancel succeeds and status becomes cancelled.

**Why human:** Dialog interaction and warning rendering require manual browser testing.

---

### Gaps Summary

No gaps found. All 5 success criteria are verified at the code level. The 6 human verification items are UX/visual checks and Docker-dependent integration tests — they are not code gaps but verification completeness requirements.

The REQUIREMENTS.md tracking table shows FR-07, FR-09, FR-10, FR-19, FR-29 as "Pending" — this appears to be a pre-completion snapshot of the tracking file that was not updated after the phase completed. All these requirements are substantively implemented in the codebase (verified above).

---

_Verified: 2026-07-01_
_Verifier: Claude (gsd-verifier)_

---

## ⚠️ Addendum — Phase REOPENED 2026-07-02 (integration gate not met)

The 5/5 above was **unit/service-level** verification (artifacts + isolated service tests with
hand-constructed claims/users). When the human-verification items were driven for real — full
stack up (docker compose), a real Keycloak token issued to a seeded user — **three runtime
request-seam bugs surfaced that the unit tests structurally could not catch**:

1. **`created-by-fk-mismatch`** — services wrote `claims.Subject` (Keycloak `sub`) into columns
   that `REFERENCES users(id)`, but `users.id` is an independent `gen_random_uuid()`. Every real
   login FK-violated on write. Unit tests masked it by setting `claims.Subject = users.id`.
   **FIXED + committed** (ef7ede6; refactored into `auth.ResolveAppUser` middleware, a1e348e).
2. **`fe-be-audience-mismatch`** — Keycloak issued frontend tokens with `aud=account`; the Go
   verifier requires `aud ∋ donnarec-backend` → **401 on every UI→API call**. Plus the frontend
   NextAuth config assumes a confidential client while the realm defined a public one, and there
   was no web env file. **FIXED** (audience mapper + confidential client persisted to
   `keycloak/realm-donnarec.json`; `donnarec-web/.env.example`); **commit pending**.
3. **RBAC AND-bug** — `RequireRoles(...)` enforces AND (all listed roles), but
   `donationGroup RequireRoles(Maker,Checker,Admin)` and `checkerGroup RequireRoles(Checker,Admin)`
   intended "any of" → **403 for every real user**. **OPEN.**

**Process fix:** a new **Integration-test gate** was added as a phase done-criterion
(Conventions → Integration-test gate; ROADMAP Phase 3 criterion 6). Phase 3 remains **REOPENED**
until: bugs #2–#3 fixed + committed, an automated **E2E HTTP integration test** (real router +
realistic token) covers the critical Maker/Checker flows, and the human UI walkthrough passes.

_Reopened: 2026-07-02 — Claude (orchestrator, human-directed)_
