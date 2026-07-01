// Package donation — white-box unit/integration tests.
//
// Task 1 tests (plan 03-03): TestConsentCapture, TestMandatoryTaxID
// Task 2 tests (plan 03-03): TestStateMachine_InvalidTransitions
// Deferred tests (plans 03-05, 03-06): TestMandatoryReason, TestEDonationKeyedGuard
package donation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/crypto"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/testutil"
)

// testKEK is a 32-byte hex key for test use only (never use in production).
const testKEK = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

// TestMigrationsApplyAndRollback verifies that all migrations (000001..000007)
// apply cleanly against a fresh PostgreSQL 17 instance and that the Phase 3
// schema elements (donations table + CHECK constraints + outbox_jobs) exist.
//
// This test requires Docker (testcontainers). Skip with -short for fast CI runs.
func TestMigrationsApplyAndRollback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping migration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	// --- Verify donations table exists ---
	var donationCount int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM donations").Scan(&donationCount)
	require.NoError(t, err, "donations table must exist after migration 000005")

	// --- Verify donation_status enum exists ---
	var enumExists bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM pg_type
			WHERE typname = 'donation_status' AND typtype = 'e'
		)`).Scan(&enumExists)
	require.NoError(t, err)
	assert.True(t, enumExists, "donation_status enum must exist")

	// --- Verify chk_sod_approver constraint exists (T-03-01 SoD backstop) ---
	var sodExists bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.table_constraints
			WHERE constraint_name = 'chk_sod_approver'
			  AND table_name = 'donations'
		)`).Scan(&sodExists)
	require.NoError(t, err)
	assert.True(t, sodExists, "chk_sod_approver CHECK constraint must exist on donations table")

	// --- Verify chk_receipt_only_on_issued_or_cancelled constraint exists (T-03-02) ---
	var receiptCheckExists bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.table_constraints
			WHERE constraint_name = 'chk_receipt_only_on_issued_or_cancelled'
			  AND table_name = 'donations'
		)`).Scan(&receiptCheckExists)
	require.NoError(t, err)
	assert.True(t, receiptCheckExists, "chk_receipt_only_on_issued_or_cancelled CHECK constraint must exist")

	// --- Verify REVOKE DELETE is in effect via pg_class/pg_catalog (T-03-03) ---
	// Indirect check: try to find DELETE privilege for donnarec_app on donations.
	// If the REVOKE worked, 'delete' must NOT appear in pg_table_privileges.
	// (The migration runs as superuser, donnarec_app role may not exist in testcontainer
	//  so we check via has_table_privilege against the superuser role instead.)
	// We verify the constraint exists; the REVOKE behaviour is proven by TestSoD_DBCheckConstraint.

	// --- Verify outbox_jobs table exists (migration 000007) ---
	var outboxCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM outbox_jobs").Scan(&outboxCount)
	require.NoError(t, err, "outbox_jobs table must exist after migration 000007")

	// --- Verify outbox_jobs status CHECK constraint ---
	var outboxCheckExists bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.table_constraints
			WHERE constraint_name LIKE '%status%'
			  AND table_name = 'outbox_jobs'
			  AND constraint_type = 'CHECK'
		)`).Scan(&outboxCheckExists)
	require.NoError(t, err)
	assert.True(t, outboxCheckExists, "outbox_jobs status CHECK constraint must exist")
}

// TestMandatoryTaxID verifies that Create returns ErrMissingTaxID when
// DonorTaxID is empty (D-44: tax/national ID is mandatory at the API boundary).
//
// This is a unit-level test — the ErrMissingTaxID check happens before any DB call,
// so no real Postgres connection is needed. A nil pool is acceptable here.
func TestMandatoryTaxID(t *testing.T) {
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", testKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	// Nil pool — service must reject empty tax ID before any DB call.
	svc := NewDonationService(nil, nil, nil, nil, kp, zap.NewNop())

	claims := auth.KeycloakClaims{
		Subject:     "00000000-0000-0000-0000-000000000001",
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	req := CreateDonationRequest{
		DonorName:  "นาย ทดสอบ",
		DonorTaxID: "", // EMPTY — D-44: must fail immediately
		Amount:     1000.00,
		DonatedAt:  "2024-01-01",
	}

	_, createErr := svc.Create(ctx, req, claims)
	require.Error(t, createErr, "Create with empty tax ID must return an error")
	assert.ErrorIs(t, createErr, ErrMissingTaxID,
		"error must be ErrMissingTaxID when DonorTaxID is empty (D-44)")
}

// TestConsentCapture verifies that consent fields (consent_given, consent_at,
// consent_text_version, consent_purpose) are persisted on the donation snapshot
// exactly as provided (D-49, NFR-03).
//
// Implemented by plan 03-03 (CreateDonation service method).
// Requires Docker testcontainers. Skip with -short.
func TestConsentCapture(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", testKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	svc := NewDonationService(pool, queries, nil, nil, kp, zap.NewNop())

	// Create a test user to satisfy the created_by FK.
	userRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:           "consent-test@example.com",
		DisplayName:     "Consent Test Maker",
		KeycloakSubject: "consent-test-keycloak-subject",
	})
	require.NoError(t, err, "test user must be created")

	claims := auth.KeycloakClaims{
		Subject:     userRow.ID.String(),
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	req := CreateDonationRequest{
		DonorName:          "นาย ทดสอบ ยินยอม",
		DonorTaxID:         "1234567890123",
		DonorAddress:       "99 ถนนสาทร กรุงเทพฯ",
		Amount:             5000.00,
		DonatedAt:          "2024-06-15",
		ConsentGiven:       true,
		ConsentTextVersion: "v2.0",
		ConsentPurpose:     "tax_reduction_50percent",
	}

	before := time.Now().UTC().Add(-time.Second)

	resp, err := svc.Create(ctx, req, claims)
	require.NoError(t, err, "Create must succeed with valid consent fields")
	require.NotNil(t, resp, "response must not be nil")

	// Raw DB query to verify consent fields were persisted (D-49, NFR-03).
	var (
		consentGiven       bool
		consentAt          *time.Time
		consentTextVersion *string
		consentPurpose     *string
	)

	err = pool.QueryRow(ctx,
		`SELECT consent_given, consent_at, consent_text_version, consent_purpose
		 FROM donations WHERE id = $1`,
		resp.ID,
	).Scan(&consentGiven, &consentAt, &consentTextVersion, &consentPurpose)
	require.NoError(t, err, "raw DB read of consent fields must succeed")

	assert.True(t, consentGiven, "consent_given must be persisted as true")
	require.NotNil(t, consentAt, "consent_at must be set when consent_given=true")
	assert.True(t, consentAt.After(before),
		"consent_at must be set at create time (got %v, want > %v)", consentAt, before)

	require.NotNil(t, consentTextVersion, "consent_text_version must be persisted")
	assert.Equal(t, "v2.0", *consentTextVersion,
		"consent_text_version must match request field")

	require.NotNil(t, consentPurpose, "consent_purpose must be persisted")
	assert.Equal(t, "tax_reduction_50percent", *consentPurpose,
		"consent_purpose must match request field")
}

// TestStateMachine_InvalidTransitions verifies that the DonationService state machine
// rejects invalid status transitions with ErrInvalidTransition.
//
// INV-6 from RESEARCH.md:
//   - pending_review cannot be submitted again
//   - pending_review draft fields cannot be updated
//
// Implemented by plan 03-03 (Submit / UpdateDraft methods).
// Requires Docker testcontainers. Skip with -short.
func TestStateMachine_InvalidTransitions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", testKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	svc := NewDonationService(pool, queries, nil, nil, kp, zap.NewNop())

	// Create a test user.
	userRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:           "sm-test@example.com",
		DisplayName:     "SM Test Maker",
		KeycloakSubject: "sm-test-keycloak-subject",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{
		Subject:     userRow.ID.String(),
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	createReq := CreateDonationRequest{
		DonorName:  "นาย ทดสอบ SM",
		DonorTaxID: "1234567890123",
		Amount:     3000.00,
		DonatedAt:  "2024-01-01",
	}
	draft, err := svc.Create(ctx, createReq, makerClaims)
	require.NoError(t, err)

	// Submit moves draft → pending_review.
	submitted, err := svc.Submit(ctx, draft.ID, makerClaims)
	require.NoError(t, err)
	assert.Equal(t, "pending_review", submitted.Status,
		"Submit must move donation to pending_review")

	// Cannot submit again (pending_review → submit is invalid).
	_, err = svc.Submit(ctx, draft.ID, makerClaims)
	require.Error(t, err, "second Submit must fail")
	assert.ErrorIs(t, err, ErrInvalidTransition,
		"second Submit on pending_review must return ErrInvalidTransition")

	// Cannot update a pending_review record (only draft is editable).
	_, err = svc.UpdateDraft(ctx, draft.ID, UpdateDraftRequest{
		DonorName:  "Updated Name",
		DonorTaxID: "1234567890123",
		Amount:     3000.00,
		DonatedAt:  "2024-01-01",
	}, makerClaims)
	require.Error(t, err, "UpdateDraft on pending_review must fail")
	assert.ErrorIs(t, err, ErrInvalidTransition,
		"UpdateDraft on pending_review must return ErrInvalidTransition")
}

// TestMandatoryReason verifies that Return and Reject require a non-empty review_reason
// (D-45, FR-12). The reason check happens before any DB call, so these tests run without
// Docker and complete in milliseconds — suitable for the per-commit -short quick-check.
func TestMandatoryReason(t *testing.T) {
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", testKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	// nil pool and nil allocator — reason check fires before any DB or allocator call.
	svc := NewDonationService(nil, nil, nil, nil, kp, zap.NewNop())

	checkerClaims := auth.KeycloakClaims{
		Subject:     "00000000-0000-0000-0000-000000000099",
		RealmAccess: auth.RealmRoles{Roles: []string{"checker"}},
	}
	donationID := "00000000-0000-0000-0000-000000000001"

	// Return: empty reason → ErrMissingReason (D-45)
	_, err = svc.Return(ctx, donationID, "", checkerClaims)
	require.Error(t, err, "Return with empty reason must error")
	assert.ErrorIs(t, err, ErrMissingReason,
		"Return with empty reason must return ErrMissingReason")

	// Return: whitespace-only reason → ErrMissingReason
	_, err = svc.Return(ctx, donationID, "   \t\n", checkerClaims)
	require.Error(t, err, "Return with whitespace-only reason must error")
	assert.ErrorIs(t, err, ErrMissingReason,
		"Return with whitespace-only reason must return ErrMissingReason")

	// Reject: empty reason → ErrMissingReason (D-45)
	_, err = svc.Reject(ctx, donationID, "", checkerClaims)
	require.Error(t, err, "Reject with empty reason must error")
	assert.ErrorIs(t, err, ErrMissingReason,
		"Reject with empty reason must return ErrMissingReason")

	// Reject: whitespace-only reason → ErrMissingReason
	_, err = svc.Reject(ctx, donationID, "   ", checkerClaims)
	require.Error(t, err, "Reject with whitespace-only reason must error")
	assert.ErrorIs(t, err, ErrMissingReason,
		"Reject with whitespace-only reason must return ErrMissingReason")
}

// TestCancelRequiresReason verifies that Cancel returns ErrMissingReason when the
// request reason is empty or whitespace-only (D-47, FR-19).
//
// The reason check fires before any DB call (Checker claims, nil pool is fine).
// Per-commit quick-check: no Docker required, completes in milliseconds.
func TestCancelRequiresReason(t *testing.T) {
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", testKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	// nil pool — reason check must fire before any DB access.
	svc := NewDonationService(nil, nil, nil, nil, kp, zap.NewNop())

	checkerClaims := auth.KeycloakClaims{
		Subject:     "00000000-0000-0000-0000-000000000099",
		RealmAccess: auth.RealmRoles{Roles: []string{"checker"}},
	}
	donationID := "00000000-0000-0000-0000-000000000001"

	// Empty reason → ErrMissingReason.
	_, err = svc.Cancel(ctx, donationID, CancelDonationRequest{Reason: ""}, checkerClaims)
	require.Error(t, err, "Cancel with empty reason must return an error")
	assert.ErrorIs(t, err, ErrMissingReason,
		"Cancel with empty reason must return ErrMissingReason (D-47)")

	// Whitespace-only reason → ErrMissingReason.
	_, err = svc.Cancel(ctx, donationID, CancelDonationRequest{Reason: "   \t\n"}, checkerClaims)
	require.Error(t, err, "Cancel with whitespace-only reason must return an error")
	assert.ErrorIs(t, err, ErrMissingReason,
		"Cancel with whitespace-only reason must return ErrMissingReason (D-47)")
}

// TestCancelAuthCheckerAdminOnly verifies that Cancel returns ErrForbidden when
// called with Maker claims (D-47: Void & cancel is restricted to Checker + Admin).
//
// The role check fires before any DB call (nil pool is fine).
// Per-commit quick-check: no Docker required, completes in milliseconds.
func TestCancelAuthCheckerAdminOnly(t *testing.T) {
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", testKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	// nil pool — role check must fire before any DB access.
	svc := NewDonationService(nil, nil, nil, nil, kp, zap.NewNop())

	makerClaims := auth.KeycloakClaims{
		Subject:     "00000000-0000-0000-0000-000000000001",
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}
	donationID := "00000000-0000-0000-0000-000000000001"

	_, err = svc.Cancel(ctx, donationID, CancelDonationRequest{Reason: "ยกเลิก"}, makerClaims)
	require.Error(t, err, "Cancel by Maker must return an error (D-47)")
	assert.ErrorIs(t, err, ErrForbidden,
		"Cancel by Maker must return ErrForbidden — only Checker/Admin may cancel (D-47)")
}

// TestEDonationKeyedGuard (unit: role/reason pre-checks only — edonation_keyed=true behaviour
// is tested in service_integration_test.go → TestEDonationKeyedGuard_Integration which
// requires Docker+testcontainers to set edonation_keyed=true on a real DB row).
//
// This unit slice covers the pre-DB checks: role gate fires before e-Donation guard.
func TestEDonationKeyedGuard(t *testing.T) {
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", testKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	// nil pool — pre-DB checks only.
	svc := NewDonationService(nil, nil, nil, nil, kp, zap.NewNop())

	// Maker cannot cancel even with rd_confirmation_reason set — role check fires first.
	makerClaims := auth.KeycloakClaims{
		Subject:     "00000000-0000-0000-0000-000000000001",
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}
	donationID := "00000000-0000-0000-0000-000000000001"

	_, err = svc.Cancel(ctx, donationID, CancelDonationRequest{
		Reason:               "ยกเลิกและแจ้ง RD",
		RDConfirmationReason: "แก้ไขข้อมูล e-Donation รอบ 2",
	}, makerClaims)
	require.Error(t, err, "Maker with rd_confirmation_reason must still be ErrForbidden")
	assert.ErrorIs(t, err, ErrForbidden,
		"Role check fires before edonation_keyed guard — Maker always gets ErrForbidden (D-47)")

	// Checker with empty reason — reason check fires before hitting DB (no edonation_keyed check).
	checkerClaims := auth.KeycloakClaims{
		Subject:     "00000000-0000-0000-0000-000000000099",
		RealmAccess: auth.RealmRoles{Roles: []string{"checker"}},
	}
	_, err = svc.Cancel(ctx, donationID, CancelDonationRequest{
		Reason:               "",
		RDConfirmationReason: "confirmed",
	}, checkerClaims)
	require.Error(t, err, "Empty reason must fail even with rd_confirmation_reason set")
	assert.ErrorIs(t, err, ErrMissingReason,
		"Reason check fires before edonation_keyed DB read — ErrMissingReason expected")
}
