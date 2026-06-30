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

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/crypto"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/donation"
	"github.com/donnarec/donnarec-api/internal/testutil"
)

// integTestKEK is a 32-byte hex key for integration test use only.
const integTestKEK = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

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
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", integTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	svc := donation.NewDonationService(pool, queries, nil, nil, kp, zap.NewNop())

	userRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:           "pii-enc-test@example.com",
		DisplayName:     "PII Enc Test Maker",
		KeycloakSubject: "pii-enc-test-keycloak-subject",
	})
	require.NoError(t, err)

	const plainTaxID = "1234567890123"

	claims := auth.KeycloakClaims{
		Subject:     userRow.ID.String(),
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	resp, err := svc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  "นาย ทดสอบ PII",
		DonorTaxID: plainTaxID,
		Amount:     2500.00,
		DonatedAt:  "2024-03-01",
	}, claims)
	require.NoError(t, err)
	require.NotEmpty(t, resp.ID, "donation ID must be set")

	// Raw DB read — bypasses service to verify what was actually stored.
	var encBytes, dekBytes []byte
	err = pool.QueryRow(ctx,
		`SELECT donor_tax_id_enc, donor_tax_id_dek FROM donations WHERE id = $1`,
		resp.ID,
	).Scan(&encBytes, &dekBytes)
	require.NoError(t, err)

	// INV-7a: ciphertext must be non-empty and must NOT equal the plaintext bytes.
	assert.NotEmpty(t, encBytes, "donor_tax_id_enc must not be empty")
	assert.False(t, bytes.Equal(encBytes, []byte(plainTaxID)),
		"donor_tax_id_enc must not equal the plaintext tax ID (PDPA: plaintext must never be stored)")

	// The wrapped DEK must also be stored alongside the ciphertext.
	assert.NotEmpty(t, dekBytes, "donor_tax_id_dek must not be empty")
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
// For a 13-digit Thai national ID "1234567890123", pii.MaskNationalID returns
// "x-xxxx-xxxxx-x0123" (last 4 revealed: "0123").
//
// Requires Docker testcontainers. Skip with -short.
func TestPII_MaskDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", integTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	svc := donation.NewDonationService(pool, queries, nil, nil, kp, zap.NewNop())

	userRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:           "pii-mask-test@example.com",
		DisplayName:     "PII Mask Test Maker",
		KeycloakSubject: "pii-mask-test-keycloak-subject",
	})
	require.NoError(t, err)

	const plainTaxID = "1234567890123"
	// pii.MaskNationalID("1234567890123") → "x-xxxx-xxxxx-x" + "0123"
	const expectedMask = "x-xxxx-xxxxx-x0123"

	claims := auth.KeycloakClaims{
		Subject:     userRow.ID.String(),
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	created, err := svc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  "นาย ทดสอบ Mask",
		DonorTaxID: plainTaxID,
		Amount:     1500.00,
		DonatedAt:  "2024-04-01",
	}, claims)
	require.NoError(t, err)

	// GetByID must return masked value, not plaintext.
	got, err := svc.GetByID(ctx, created.ID, claims)
	require.NoError(t, err)

	assert.Equal(t, expectedMask, got.DonorTaxIDMasked,
		"GetByID must return masked tax ID (last-4 reveal) — never plaintext (T-03-09)")
	assert.NotEqual(t, plainTaxID, got.DonorTaxIDMasked,
		"DonorTaxIDMasked must not equal the plaintext tax ID")
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

// TestSubmitMovesToPendingReview verifies FR-11 / D-45:
// Submit transitions a draft donation to pending_review and sets submitted_at.
//
// Requires Docker testcontainers. Skip with -short.
func TestSubmitMovesToPendingReview(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", integTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	svc := donation.NewDonationService(pool, queries, nil, nil, kp, zap.NewNop())

	userRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:           "submit-test@example.com",
		DisplayName:     "Submit Test Maker",
		KeycloakSubject: "submit-test-keycloak-subject",
	})
	require.NoError(t, err)

	claims := auth.KeycloakClaims{
		Subject:     userRow.ID.String(),
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	// Create a draft donation.
	before := time.Now().UTC().Add(-time.Second)
	draft, err := svc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  "นาย ทดสอบ Submit",
		DonorTaxID: "1111222233334",
		Amount:     7500.00,
		DonatedAt:  "2024-07-01",
	}, claims)
	require.NoError(t, err)
	assert.Equal(t, "draft", draft.Status, "new donation must be in draft status")

	// Submit moves draft → pending_review.
	submitted, err := svc.Submit(ctx, draft.ID, claims)
	require.NoError(t, err, "Submit must succeed on a draft")
	require.NotNil(t, submitted)

	assert.Equal(t, "pending_review", submitted.Status,
		"Submit must transition status to pending_review (D-45, FR-11)")
	require.NotNil(t, submitted.SubmittedAt,
		"submitted_at must be set after Submit")
	assert.True(t, submitted.SubmittedAt.After(before),
		"submitted_at must be a recent timestamp (got %v)", submitted.SubmittedAt)
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
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", integTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	svc := donation.NewDonationService(pool, queries, nil, nil, kp, zap.NewNop())

	userRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:           "create-test@example.com",
		DisplayName:     "Create Test Maker",
		KeycloakSubject: "create-test-keycloak-subject",
	})
	require.NoError(t, err)

	claims := auth.KeycloakClaims{
		Subject:     userRow.ID.String(),
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	req := donation.CreateDonationRequest{
		DonorName:          "นาย ทดสอบ สร้าง",
		DonorTaxID:         "9876543210987",
		DonorAddress:       "55 ถนนพระราม4 กรุงเทพฯ",
		DonorEmail:         "donor@example.com",
		Amount:             10000.50,
		DonatedAt:          "2024-05-10",
		Notes:              "ทดสอบการสร้างรายการบริจาค",
		ConsentGiven:       true,
		ConsentTextVersion: "v1.0",
		ConsentPurpose:     "tax_reduction_100percent",
	}

	resp, err := svc.Create(ctx, req, claims)
	require.NoError(t, err, "Create must succeed with valid request")
	require.NotNil(t, resp, "response must not be nil")

	// FR-07: record is returned in 'draft' status with a generated UUID.
	assert.NotEmpty(t, resp.ID, "donation ID must be a non-empty UUID")
	assert.Equal(t, "draft", resp.Status, "new donation must start in draft status")
	assert.Equal(t, req.DonorName, resp.DonorName, "donor name must be set")
	assert.NotEmpty(t, resp.DonorTaxIDMasked, "masked tax ID must be set")
	assert.Equal(t, req.DonorAddress, resp.DonorAddress, "donor address must be set")
	assert.Equal(t, userRow.ID.String(), resp.CreatedBy, "created_by must be set to the maker's user ID")

	// Verify the record exists in the database.
	var count int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM donations WHERE id = $1", resp.ID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "exactly one donation row must exist after Create")
}

// TestEditDraft verifies FR-09 edit-before-submit behaviour:
// a Maker can update donor fields on their own draft donation; the update is
// rejected with ErrInvalidTransition once the donation has been submitted (status != draft).
//
// Requires Docker testcontainers. Skip with -short.
func TestEditDraft(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", integTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	svc := donation.NewDonationService(pool, queries, nil, nil, kp, zap.NewNop())

	userRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:           "edit-draft-test@example.com",
		DisplayName:     "Edit Draft Test Maker",
		KeycloakSubject: "edit-draft-test-keycloak-subject",
	})
	require.NoError(t, err)

	claims := auth.KeycloakClaims{
		Subject:     userRow.ID.String(),
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	// Create a draft first.
	draft, err := svc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  "นาย ทดสอบ แก้ไข",
		DonorTaxID: "1112223334445",
		Amount:     8000.00,
		DonatedAt:  "2024-02-14",
	}, claims)
	require.NoError(t, err)
	assert.Equal(t, "draft", draft.Status)

	// Update the draft — must succeed.
	updated, err := svc.UpdateDraft(ctx, draft.ID, donation.UpdateDraftRequest{
		DonorName:  "นาย ทดสอบ แก้ไขแล้ว",
		DonorTaxID: "1112223334445",
		Amount:     9000.00,
		DonatedAt:  "2024-02-14",
	}, claims)
	require.NoError(t, err, "UpdateDraft on a draft record must succeed")
	assert.Equal(t, "นาย ทดสอบ แก้ไขแล้ว", updated.DonorName,
		"UpdateDraft must persist the updated donor name")

	// Submit the draft.
	_, err = svc.Submit(ctx, draft.ID, claims)
	require.NoError(t, err, "Submit must succeed on a draft")

	// UpdateDraft on a non-draft (pending_review) must fail.
	_, err = svc.UpdateDraft(ctx, draft.ID, donation.UpdateDraftRequest{
		DonorName:  "Should Not Update",
		DonorTaxID: "1112223334445",
		Amount:     9000.00,
		DonatedAt:  "2024-02-14",
	}, claims)
	require.Error(t, err, "UpdateDraft after Submit must fail")
	assert.ErrorIs(t, err, donation.ErrInvalidTransition,
		"UpdateDraft on pending_review must return ErrInvalidTransition (FR-09)")
}
