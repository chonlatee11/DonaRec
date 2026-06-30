// Package donation_test — black-box integration test scaffolds (Wave 0).
//
// This file defines the contract for the 7 hardest invariants from RESEARCH.md §"The 7
// Hardest Invariants". All tests use t.Skip and require Docker + testcontainers to run.
//
// Each test name maps to an invariant that must be implemented in later plans:
//   INV-1: TestIssuanceTransaction_RollbackOnError  — plan 03-05
//   INV-2: TestOutboxAtomicity                      — plan 03-05
//   INV-3: TestSoD_ApproverCannotBeCreator          — plan 03-05
//   INV-3: TestSoD_DBCheckConstraint                — plan 03-05 (DB constraint backstop)
//   INV-4: TestConcurrentApproval_ExactlyOneSucceeds — plan 03-05
//   INV-5: TestCancelRetainsReceiptNumber            — plan 03-06
//   INV-6: (see service_test.go TestStateMachine_InvalidTransitions)
//   INV-7: TestPII_TaxIDStoredEncrypted             — plan 03-03
//          TestPII_RevealRequiresCheckerOrAdmin      — plan 03-05/06
//          TestPII_MaskDefault                       — plan 03-03
//
// Additional scaffold tests cover FR-07, FR-09, FR-10, FR-19, D-50.
package donation_test

import "testing"

// TestIssuanceTransaction_RollbackOnError verifies INV-1: when any step of the
// issuance transaction fails (allocate, update status, audit, enqueue outbox),
// ALL effects are rolled back — no partial state is persisted.
//
// Scenarios:
//   A: error after Allocate, before IssueDonation → ledger has 0 rows, status = pending_review
//   B: error after IssueDonation, before audit    → status = pending_review, outbox has 0 rows
//   C: happy path                                  → status = issued, 1 receipt row, 1 outbox row
//
// Requires Docker testcontainers. Skip with -short.
func TestIssuanceTransaction_RollbackOnError(t *testing.T) {
	if testing.Short() {
		t.Skip("Wave 0 scaffold — implemented in plan 03-05 (issuance transaction atomicity)")
	}
	t.Skip("Wave 0 scaffold — implemented in plan 03-05 (issuance transaction atomicity)")
}

// TestOutboxAtomicity verifies INV-2: an outbox_jobs row exists IFF the receipt
// was issued — both effects commit together or neither does.
//
// Requires Docker testcontainers. Skip with -short.
func TestOutboxAtomicity(t *testing.T) {
	if testing.Short() {
		t.Skip("Wave 0 scaffold — implemented in plan 03-05 (outbox atomicity)")
	}
	t.Skip("Wave 0 scaffold — implemented in plan 03-05 (outbox atomicity)")
}

// TestSoD_ApproverCannotBeCreator verifies INV-3 (code guard layer):
// DonationService.Approve returns ErrSoDViolation when the approver UUID
// matches the donation's created_by UUID.
//
// Requires Docker testcontainers. Skip with -short.
func TestSoD_ApproverCannotBeCreator(t *testing.T) {
	if testing.Short() {
		t.Skip("Wave 0 scaffold — implemented in plan 03-05 (SoD code guard)")
	}
	t.Skip("Wave 0 scaffold — implemented in plan 03-05 (SoD code guard)")
}

// TestSoD_DBCheckConstraint verifies INV-3 (DB backstop layer):
// a raw UPDATE that sets approved_by = created_by on the donations table
// is rejected by the chk_sod_approver CHECK constraint with a PgError.
//
// This proves defense-in-depth (CLAUDE.md): even if the service guard is bypassed,
// the DB-level constraint catches the violation.
//
// Requires Docker testcontainers. Skip with -short.
func TestSoD_DBCheckConstraint(t *testing.T) {
	if testing.Short() {
		t.Skip("Wave 0 scaffold — implemented in plan 03-05 (SoD DB constraint backstop)")
	}
	t.Skip("Wave 0 scaffold — implemented in plan 03-05 (SoD DB constraint backstop)")
}

// TestConcurrentApproval_ExactlyOneSucceeds verifies INV-4:
// when N goroutines simultaneously attempt to approve the same pending_review donation,
// exactly ONE approval succeeds (gets a receipt number) and the rest return
// ErrInvalidTransition (not an internal server error).
//
// Mirrors TestAllocator_Concurrency from receiptno package (same errgroup pattern).
// The SELECT … FOR UPDATE on the donation row serializes the N goroutines (D-52).
//
// Requires Docker testcontainers. Skip with -short.
func TestConcurrentApproval_ExactlyOneSucceeds(t *testing.T) {
	if testing.Short() {
		t.Skip("Wave 0 scaffold — implemented in plan 03-05 (concurrent approval lock)")
	}
	t.Skip("Wave 0 scaffold — implemented in plan 03-05 (concurrent approval lock)")
}

// TestCancelRetainsReceiptNumber verifies INV-5 (FR-19, D-47):
// after an issued donation is cancelled, receipt_number_id and receipt_formatted
// remain set on the donation row (no gap is created in the receipt sequence).
//
// Also verifies the chk_receipt_only_on_issued_or_cancelled CHECK constraint
// enforces that cancelled records retain their receipt fields.
//
// Requires Docker testcontainers. Skip with -short.
func TestCancelRetainsReceiptNumber(t *testing.T) {
	if testing.Short() {
		t.Skip("Wave 0 scaffold — implemented in plan 03-06 (cancel retains receipt)")
	}
	t.Skip("Wave 0 scaffold — implemented in plan 03-06 (cancel retains receipt)")
}

// TestPII_TaxIDStoredEncrypted verifies INV-7a (NFR-02, PDPA):
// after CreateDonation, the donor_tax_id_enc column contains ciphertext (non-empty bytes)
// and is NOT equal to the plaintext tax ID that was passed to the service.
// The donor_tax_id_dek column contains the wrapped DEK (non-empty bytes).
//
// Requires Docker testcontainers. Skip with -short.
func TestPII_TaxIDStoredEncrypted(t *testing.T) {
	if testing.Short() {
		t.Skip("Wave 0 scaffold — implemented in plan 03-03 (PII encrypt on create)")
	}
	t.Skip("Wave 0 scaffold — implemented in plan 03-03 (PII encrypt on create)")
}

// TestPII_RevealRequiresCheckerOrAdmin verifies INV-7b (D-46, NFR-02):
// GET /api/donations/:id/pii returns the plaintext tax ID only when called with
// Checker or Admin claims. Maker claims receive ErrForbidden.
//
// Requires Docker testcontainers. Skip with -short.
func TestPII_RevealRequiresCheckerOrAdmin(t *testing.T) {
	if testing.Short() {
		t.Skip("Wave 0 scaffold — implemented in plan 03-05 (PII reveal gate)")
	}
	t.Skip("Wave 0 scaffold — implemented in plan 03-05 (PII reveal gate)")
}

// TestPII_MaskDefault verifies INV-7c (NFR-02):
// the default GetDonation response always returns a masked tax ID placeholder,
// never the plaintext or raw ciphertext bytes, regardless of caller role.
//
// Requires Docker testcontainers. Skip with -short.
func TestPII_MaskDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("Wave 0 scaffold — implemented in plan 03-03 (PII mask in response)")
	}
	t.Skip("Wave 0 scaffold — implemented in plan 03-03 (PII mask in response)")
}

// TestVoidAndReissue verifies D-50 void & reissue flow:
// after cancelling an issued donation and creating a replacement, the
// replaces/replaced_by self-FK links are set correctly on both records.
//
// Requires Docker testcontainers. Skip with -short.
func TestVoidAndReissue(t *testing.T) {
	if testing.Short() {
		t.Skip("Wave 0 scaffold — implemented in plan 03-06 (void & reissue links)")
	}
	t.Skip("Wave 0 scaffold — implemented in plan 03-06 (void & reissue links)")
}

// TestSearchDonations verifies FR-10 / D-53 search behaviour:
// donations can be filtered by donor name (ILIKE), date range, status, and receipt number;
// each filter is independent and can be nil (no restriction).
//
// Requires Docker testcontainers. Skip with -short.
func TestSearchDonations(t *testing.T) {
	if testing.Short() {
		t.Skip("Wave 0 scaffold — implemented in plan 03-07 (list/search API)")
	}
	t.Skip("Wave 0 scaffold — implemented in plan 03-07 (list/search API)")
}

// TestCreateDonation verifies FR-07 / D-43 end-to-end donation creation:
// a Maker can create a donation with donor snapshot + PII ciphertext + consent fields,
// and the record is returned in 'draft' status with a generated UUID.
//
// Requires Docker testcontainers. Skip with -short.
func TestCreateDonation(t *testing.T) {
	if testing.Short() {
		t.Skip("Wave 0 scaffold — implemented in plan 03-03 (CreateDonation service)")
	}
	t.Skip("Wave 0 scaffold — implemented in plan 03-03 (CreateDonation service)")
}

// TestEditDraft verifies FR-09 edit-before-submit behaviour:
// a Maker can update donor fields on their own draft donation; the update is
// rejected with ErrDraftOnly once the donation has been submitted (status != draft).
//
// Requires Docker testcontainers. Skip with -short.
func TestEditDraft(t *testing.T) {
	if testing.Short() {
		t.Skip("Wave 0 scaffold — implemented in plan 03-03 (UpdateDraft service method)")
	}
	t.Skip("Wave 0 scaffold — implemented in plan 03-03 (UpdateDraft service method)")
}
