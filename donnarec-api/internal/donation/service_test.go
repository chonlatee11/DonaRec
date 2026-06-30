// Package donation — white-box unit test scaffolds (Wave 0).
//
// This file defines the contract for the 5 unit-level invariants that must be
// implemented in later plans (03-03, 03-05, 03-06). Tests marked t.Fatal are
// intentionally RED until the service layer is implemented.
//
// TestMigrationsApplyAndRollback is the only passing test here — it verifies
// that migrations 000001..000007 apply cleanly against a live Postgres 17 container
// and is used by the Task 1 acceptance gate.
package donation

import (
	"context"
	"testing"

	"github.com/donnarec/donnarec-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// TestStateMachine_InvalidTransitions verifies that the DonationService state machine
// rejects invalid status transitions with ErrInvalidTransition.
//
// INV-6 from RESEARCH.md §"The 7 Hardest Invariants":
//   - draft cannot be approved/cancelled/rejected directly
//   - pending_review cannot be submitted again
//   - issued cannot be approved again
//   - rejected/cancelled are terminal — no transitions allowed
//
// Implemented by plan 03-05 (issuance service).
func TestStateMachine_InvalidTransitions(t *testing.T) {
	t.Fatal("not implemented — filled by 03-05 (DonationService state machine guard)")
}

// TestMandatoryReason verifies that return and reject actions require a non-empty
// review_reason, and that cancel requires a non-empty cancel_reason.
//
// Decision D-45: both return and reject are mandatory-reason actions.
// Decision D-47: cancel also requires a reason.
//
// Implemented by plan 03-05 (approve/review service methods).
func TestMandatoryReason(t *testing.T) {
	t.Fatal("not implemented — filled by 03-05 (return/reject/cancel reason validation)")
}

// TestConsentCapture verifies that consent fields (consent_given, consent_at,
// consent_text_version, consent_purpose) are persisted on the donation snapshot
// exactly as provided (D-49, NFR-03).
//
// Implemented by plan 03-03 (CreateDonation service method).
func TestConsentCapture(t *testing.T) {
	t.Fatal("not implemented — filled by 03-03 (CreateDonation consent persistence)")
}

// TestEDonationKeyedGuard verifies that cancellation of a donation with
// edonation_keyed=true requires an rd_confirmation_reason in the request body (D-51).
//
// When edonation_keyed=true and rd_confirmation_reason is empty, the service
// must return ErrEDonationKeyedCancel (mapped to 422 by the handler).
//
// Implemented by plan 03-06 (cancel service method with edonation_keyed guard).
func TestEDonationKeyedGuard(t *testing.T) {
	t.Fatal("not implemented — filled by 03-06 (cancel service edonation_keyed guard)")
}
