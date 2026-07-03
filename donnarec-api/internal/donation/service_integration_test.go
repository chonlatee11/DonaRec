// Package donation_test — integration tests for the donation service.
//
// This file implements the 7 hardest invariants from RESEARCH.md §"The 7 Hardest Invariants".
// All tests require a live PostgreSQL container (testcontainers). Skip with -short.
//
// Plan 03-05 tests (issuance tx, SoD, concurrency, return/reject):
//
//	INV-1: TestIssuanceTransaction_RollbackOnError  — atomicity of 7-step approve tx
//	INV-2: TestOutboxAtomicity                      — outbox row IFF receipt issued
//	INV-3: TestSoD_ApproverCannotBeCreator          — code-level SoD guard
//	INV-3: TestSoD_DBCheckConstraint                — DB CHECK constraint backstop
//	INV-4: TestConcurrentApproval_ExactlyOneSucceeds — FOR UPDATE serializes N goroutines
//	       TestReturnToDraft                        — pending_review → draft + audit
//	       TestRejectTerminal                       — pending_review → rejected (terminal)
//
// Plan 03-06 tests (cancel, void, reissue) remain as scaffolds (t.Skip).
// Plan 03-07 tests (search) remain as scaffolds (t.Skip).
package donation_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/donnarec/donnarec-api/internal/audit"
	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/crypto"
	dbhelpers "github.com/donnarec/donnarec-api/internal/db"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/donation"
	"github.com/donnarec/donnarec-api/internal/receiptno"
	"github.com/donnarec/donnarec-api/internal/testutil"
)

// integTestKEK is a 32-byte hex key for integration test use only.
const integTestKEK = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

// errDeliberateRollback is a sentinel returned from a WithTx closure to trigger an
// explicit rollback without surfacing as a "real" test failure (same pattern as
// allocator_rollback_test.go).
var errDeliberateRollback = errors.New("deliberate rollback for atomicity test")

// --- helpers ---------------------------------------------------------------

// createAndSubmit creates a draft donation and submits it (pending_review).
// Returns the submitted DonationResponse. Skips if any step errors.
func createAndSubmit(
	t *testing.T,
	ctx context.Context,
	svc *donation.DonationService,
	makerClaims auth.KeycloakClaims,
	makerID pgtype.UUID,
	donorName, taxID, date string,
	amount float64,
) *donation.DonationResponse {
	t.Helper()
	d, err := svc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  donorName,
		DonorTaxID: taxID,
		Amount:     amount,
		DonatedAt:  date,
	}, makerID, makerClaims)
	require.NoError(t, err, "Create must succeed")

	submitted, err := svc.Submit(ctx, d.ID, makerClaims)
	require.NoError(t, err, "Submit must succeed")
	require.Equal(t, "pending_review", submitted.Status)
	return submitted
}

// ---------------------------------------------------------------------------
// INV-1: TestIssuanceTransaction_RollbackOnError
// ---------------------------------------------------------------------------

// TestIssuanceTransaction_RollbackOnError verifies that all 7 effects of the
// issuance transaction are rolled back atomically when any step fails (INV-1).
//
// Scenarios:
//
//	A: rollback after Allocate (before IssueDonation)  → ledger 0 rows, status=pending_review
//	B: rollback after IssueDonation (before audit)     → status=pending_review, outbox 0 rows
//	C: happy path (full commit)                         → status=issued, 1 ledger row, 1 outbox
//
// Requires Docker testcontainers. Skip with -short.
func TestIssuanceTransaction_RollbackOnError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", integTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)
	auditSvc := audit.NewAuditService(pool, queries, zap.NewNop())
	svc := donation.NewDonationService(pool, queries, alloc, auditSvc, kp, zap.NewNop())

	// Resolution now happens in auth.ResolveAppUser middleware (created-by-fk-mismatch fix);
	// calling the service directly, the tests pass the resolved users.id (makerRow.ID /
	// checkerRow.ID) as actingUserID.
	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "maker-rollback@example.com", DisplayName: "Maker Rollback",
		KeycloakSubject: "9ac95dbf-af10-42cb-936e-ab94c8fb1516",
	})
	require.NoError(t, err)
	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-rollback@example.com", DisplayName: "Checker Rollback",
		KeycloakSubject: "176741a9-865a-4492-89b4-d093c7747787",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{Subject: "9ac95dbf-af10-42cb-936e-ab94c8fb1516", RealmAccess: auth.RealmRoles{Roles: []string{"maker"}}}
	checkerClaims := auth.KeycloakClaims{Subject: "176741a9-865a-4492-89b4-d093c7747787", RealmAccess: auth.RealmRoles{Roles: []string{"checker"}}}

	// ---- Scenario A: rollback after Allocate, before IssueDonation ----------------
	t.Run("ScenarioA_RollbackAfterAllocate", func(t *testing.T) {
		d := createAndSubmit(t, ctx, svc, makerClaims, makerRow.ID,
			"นาย ทดสอบ Rollback A", "1234567890123", "2026-03-15", 1000.00)

		var pgUUID pgtype.UUID
		require.NoError(t, pgUUID.Scan(d.ID))

		// Simulate: allocate inside a tx, then force rollback.
		rollErr := dbhelpers.WithTx(ctx, pool, func(tx pgx.Tx) error {
			qtx := queries.WithTx(tx)
			_, lockErr := qtx.LockDonationForUpdate(ctx, pgUUID)
			require.NoError(t, lockErr, "LockDonationForUpdate must succeed")

			_, allocErr := alloc.Allocate(ctx, tx, time.Now())
			require.NoError(t, allocErr, "Allocate must succeed inside tx")

			return errDeliberateRollback // WithTx rolls back counter + ledger INSERT
		})
		require.ErrorIs(t, rollErr, errDeliberateRollback)

		// Assert: no receipt_numbers row was persisted (rollback undid ledger INSERT).
		var ledgerCount int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM receipt_numbers`).Scan(&ledgerCount))
		assert.Equal(t, 0, ledgerCount,
			"Scenario A: ledger must have 0 rows after rollback")

		// Assert: donation status still pending_review.
		var status db.DonationStatus
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT status FROM donations WHERE id = $1`, d.ID).Scan(&status))
		assert.Equal(t, db.DonationStatusPendingReview, status,
			"Scenario A: status must remain pending_review after rollback")
	})

	// ---- Scenario B: rollback after IssueDonation, before audit ------------------
	t.Run("ScenarioB_RollbackAfterIssueDonation", func(t *testing.T) {
		d := createAndSubmit(t, ctx, svc, makerClaims, makerRow.ID,
			"นาย ทดสอบ Rollback B", "9876543210987", "2026-03-15", 2000.00)

		var pgUUID pgtype.UUID
		require.NoError(t, pgUUID.Scan(d.ID))

		// checkerRow.ID (users.id) — not checkerClaims.Subject (raw keycloak_subject
		// literal, not a UUID) — this test writes directly via qtx.IssueDonation,
		// bypassing DonationService.Approve entirely (created-by-fk-mismatch: approved_by
		// REFERENCES users(id), so it must be the resolved users.id).
		checkerUUID := checkerRow.ID

		rollErr := dbhelpers.WithTx(ctx, pool, func(tx pgx.Tx) error {
			qtx := queries.WithTx(tx)

			_, lockErr := qtx.LockDonationForUpdate(ctx, pgUUID)
			require.NoError(t, lockErr)

			receipt, allocErr := alloc.Allocate(ctx, tx, time.Now())
			require.NoError(t, allocErr)

			// Stamp issued — then force rollback before audit/outbox.
			receiptID := receipt.ID
			formatted := receipt.Formatted
			issueErr := qtx.IssueDonation(ctx, db.IssueDonationParams{
				ApprovedBy:       checkerUUID,
				ApprovedAt:       pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
				ReceiptNumberID:  &receiptID,
				ReceiptFormatted: &formatted,
				ID:               pgUUID,
			})
			require.NoError(t, issueErr, "IssueDonation inside tx must succeed")

			return errDeliberateRollback // rolls back UPDATE + ledger INSERT + counter
		})
		require.ErrorIs(t, rollErr, errDeliberateRollback)

		// Assert: status still pending_review (UPDATE was rolled back).
		var status db.DonationStatus
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT status FROM donations WHERE id = $1`, d.ID).Scan(&status))
		assert.Equal(t, db.DonationStatusPendingReview, status,
			"Scenario B: status must remain pending_review after rollback")

		// Assert: still 0 receipt_numbers rows (from previous scenario A rollback +
		// this scenario B rollback — no committed allocations yet).
		var ledgerCount int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM receipt_numbers`).Scan(&ledgerCount))
		assert.Equal(t, 0, ledgerCount,
			"Scenario B: no receipt_numbers rows must exist after rollback")

		// Assert: 0 outbox rows.
		var outboxCount int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM outbox_jobs`).Scan(&outboxCount))
		assert.Equal(t, 0, outboxCount,
			"Scenario B: no outbox_jobs rows must exist after rollback")
	})

	// ---- Scenario C: happy path — all 7 effects commit together ------------------
	t.Run("ScenarioC_HappyPath", func(t *testing.T) {
		d := createAndSubmit(t, ctx, svc, makerClaims, makerRow.ID,
			"นาย ทดสอบ Happy Path", "1111222233334", "2026-03-15", 5000.00)

		approved, err := svc.Approve(ctx, d.ID, checkerRow.ID, checkerClaims)
		require.NoError(t, err, "Approve must succeed on pending_review donation")
		require.NotNil(t, approved)

		assert.Equal(t, "issued", approved.Status,
			"Scenario C: status must be issued after Approve")
		assert.NotNil(t, approved.ReceiptFormatted,
			"Scenario C: receipt_formatted must be set after issuance")
		assert.NotNil(t, approved.ApprovedAt,
			"Scenario C: approved_at must be set after issuance")

		// Assert via DB: 1 receipt_numbers row, 1 audit row, 1 outbox row.
		var ledgerCount int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM receipt_numbers`).Scan(&ledgerCount))
		assert.Equal(t, 1, ledgerCount,
			"Scenario C: exactly 1 receipt_numbers row must exist after issuance")

		var auditCount int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM audit_log WHERE action = 'donation.approve'`).Scan(&auditCount))
		assert.Equal(t, 1, auditCount,
			"Scenario C: exactly 1 audit row for donation.approve must exist")

		var outboxCount int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM outbox_jobs WHERE job_type = 'issue_receipt'`).Scan(&outboxCount))
		assert.Equal(t, 1, outboxCount,
			"Scenario C: exactly 1 outbox_jobs row must exist after issuance")

		// Assert donations.receipt_number_id FK is set on the issued row.
		var receiptNumberID *int64
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT receipt_number_id FROM donations WHERE id = $1`, d.ID).Scan(&receiptNumberID))
		require.NotNil(t, receiptNumberID,
			"Scenario C: receipt_number_id must be non-null on the issued donation row (D-38 FK)")
	})
}

// ---------------------------------------------------------------------------
// INV-2: TestOutboxAtomicity
// ---------------------------------------------------------------------------

// TestOutboxAtomicity verifies that an outbox_jobs row exists IFF the receipt
// was issued — both effects commit together or neither does (INV-2).
//
// Requires Docker testcontainers. Skip with -short.
func TestOutboxAtomicity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", integTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)
	auditSvc := audit.NewAuditService(pool, queries, zap.NewNop())
	svc := donation.NewDonationService(pool, queries, alloc, auditSvc, kp, zap.NewNop())

	// Resolution now happens in auth.ResolveAppUser middleware (created-by-fk-mismatch fix);
	// calling the service directly, pass the resolved users.id as actingUserID.
	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "maker-outbox@example.com", DisplayName: "Maker Outbox",
		KeycloakSubject: "e8da7327-5a73-4708-962b-e66cbf07d0e1",
	})
	require.NoError(t, err)
	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-outbox@example.com", DisplayName: "Checker Outbox",
		KeycloakSubject: "a8faa76b-9f25-47ab-962f-8a14bb541a89",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{Subject: "e8da7327-5a73-4708-962b-e66cbf07d0e1", RealmAccess: auth.RealmRoles{Roles: []string{"maker"}}}
	checkerClaims := auth.KeycloakClaims{Subject: "a8faa76b-9f25-47ab-962f-8a14bb541a89", RealmAccess: auth.RealmRoles{Roles: []string{"checker"}}}

	// Part 1: Rollback → no outbox row persisted.
	t.Run("RollbackNoOutbox", func(t *testing.T) {
		d := createAndSubmit(t, ctx, svc, makerClaims, makerRow.ID,
			"นาย ทดสอบ Outbox Rollback", "1234512345123", "2026-04-01", 3000.00)

		var pgUUID pgtype.UUID
		require.NoError(t, pgUUID.Scan(d.ID))

		// checkerRow.ID (users.id) — not checkerClaims.Subject (raw keycloak_subject
		// literal, not a UUID) — this test writes directly via qtx.IssueDonation,
		// bypassing DonationService.Approve entirely (created-by-fk-mismatch: approved_by
		// REFERENCES users(id), so it must be the resolved users.id).
		checkerUUID := checkerRow.ID

		rollErr := dbhelpers.WithTx(ctx, pool, func(tx pgx.Tx) error {
			qtx := queries.WithTx(tx)
			_, _ = qtx.LockDonationForUpdate(ctx, pgUUID)
			receipt, _ := alloc.Allocate(ctx, tx, time.Now())
			receiptID, formatted := receipt.ID, receipt.Formatted
			_ = qtx.IssueDonation(ctx, db.IssueDonationParams{
				ApprovedBy:       checkerUUID,
				ApprovedAt:       pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
				ReceiptNumberID:  &receiptID,
				ReceiptFormatted: &formatted,
				ID:               pgUUID,
			})
			// Simulate error BEFORE outbox INSERT → rollback undoes all above.
			return errDeliberateRollback
		})
		require.ErrorIs(t, rollErr, errDeliberateRollback)

		var outboxCount int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM outbox_jobs`).Scan(&outboxCount))
		assert.Equal(t, 0, outboxCount,
			"No outbox row must exist after rollback (INV-2)")
	})

	// Part 2: Successful Approve → outbox row exists.
	t.Run("SuccessHasOutbox", func(t *testing.T) {
		d := createAndSubmit(t, ctx, svc, makerClaims, makerRow.ID,
			"นาย ทดสอบ Outbox Success", "9999888877776", "2026-04-01", 4000.00)

		_, err := svc.Approve(ctx, d.ID, checkerRow.ID, checkerClaims)
		require.NoError(t, err, "Approve must succeed")

		var outboxCount int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM outbox_jobs WHERE job_type = 'issue_receipt'`).Scan(&outboxCount))
		assert.Equal(t, 1, outboxCount,
			"Exactly 1 outbox row must exist after successful Approve (INV-2)")
	})
}

// ---------------------------------------------------------------------------
// INV-3 (code layer): TestSoD_ApproverCannotBeCreator
// ---------------------------------------------------------------------------

// TestSoD_ApproverCannotBeCreator verifies that DonationService.Approve returns
// ErrSoDViolation when approverID == donation.CreatedBy (INV-3 — code guard layer).
// No receipt number must be allocated on an SoD violation.
//
// Requires Docker testcontainers. Skip with -short.
func TestSoD_ApproverCannotBeCreator(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", integTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)
	auditSvc := audit.NewAuditService(pool, queries, zap.NewNop())
	svc := donation.NewDonationService(pool, queries, alloc, auditSvc, kp, zap.NewNop())

	// Single user who is both maker AND checker — SoD violation when they self-approve.
	// The SAME resolved users.id (dualRow.ID) is passed as actingUserID to both Create and
	// Approve, so the service's approverID == created_by check fires (created-by-fk-mismatch fix).
	dualRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "dual-role@example.com", DisplayName: "Dual Role",
		KeycloakSubject: "e02c7017-5397-420b-9681-31fced49a01c",
	})
	require.NoError(t, err)

	dualClaims := auth.KeycloakClaims{
		Subject:     "e02c7017-5397-420b-9681-31fced49a01c",
		RealmAccess: auth.RealmRoles{Roles: []string{"maker", "checker"}},
	}

	d := createAndSubmit(t, ctx, svc, dualClaims, dualRow.ID,
		"นาย ทดสอบ SoD Code", "1231231231231", "2026-05-01", 7500.00)

	// Approve with the same user who created — must return ErrSoDViolation.
	_, approveErr := svc.Approve(ctx, d.ID, dualRow.ID, dualClaims)
	require.Error(t, approveErr, "Approve with creator's own claims must return an error")
	assert.ErrorIs(t, approveErr, donation.ErrSoDViolation,
		"Approve by creator must return ErrSoDViolation (INV-3 code guard)")

	// Assert: no receipt_numbers row was allocated on SoD violation.
	var ledgerCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM receipt_numbers`).Scan(&ledgerCount))
	assert.Equal(t, 0, ledgerCount,
		"No receipt_numbers row must exist after ErrSoDViolation — no number allocated")

	// Assert: donation status remains pending_review (not issued).
	var status db.DonationStatus
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status FROM donations WHERE id = $1`, d.ID).Scan(&status))
	assert.Equal(t, db.DonationStatusPendingReview, status,
		"Status must remain pending_review after SoD violation")
}

// ---------------------------------------------------------------------------
// INV-3 (DB layer): TestSoD_DBCheckConstraint
// ---------------------------------------------------------------------------

// TestSoD_DBCheckConstraint verifies the chk_sod_approver CHECK constraint fires
// when a raw UPDATE sets approved_by = created_by (INV-3 — DB backstop layer).
//
// This proves defense-in-depth (CLAUDE.md): even if the service guard is bypassed,
// the DB-level constraint rejects the write.
//
// Requires Docker testcontainers. Skip with -short.
func TestSoD_DBCheckConstraint(t *testing.T) {
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

	// Resolution now happens in auth.ResolveAppUser middleware (created-by-fk-mismatch fix);
	// calling the service directly, pass makerRow.ID as actingUserID.
	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "sod-db-test@example.com", DisplayName: "SoD DB Test",
		KeycloakSubject: "0b77b14a-2f8d-46f6-8a78-bbc6e990b077",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{
		Subject:     "0b77b14a-2f8d-46f6-8a78-bbc6e990b077",
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	d, err := svc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  "นาย ทดสอบ SoD DB",
		DonorTaxID: "1111333355557",
		Amount:     1500.00,
		DonatedAt:  "2026-06-01",
	}, makerRow.ID, makerClaims)
	require.NoError(t, err)

	// Attempt raw UPDATE: approved_by = created_by  →  violates chk_sod_approver.
	// The constraint is: CHECK (approved_by IS NULL OR approved_by != created_by)
	// Setting approved_by = created_by makes (approved_by IS NULL) = FALSE and
	// (approved_by != created_by) = FALSE → constraint fires.
	_, dbErr := pool.Exec(ctx,
		`UPDATE donations SET approved_by = created_by WHERE id = $1`, d.ID)
	require.Error(t, dbErr,
		"Setting approved_by = created_by must violate the SoD check constraint")

	var pgErr *pgconn.PgError
	require.True(t, errors.As(dbErr, &pgErr),
		"Error must be a *pgconn.PgError, got: %T", dbErr)
	assert.Equal(t, pgerrcode.CheckViolation, pgErr.Code,
		"SQLSTATE must be 23514 (check_violation) — chk_sod_approver constraint is active")
	assert.Equal(t, "chk_sod_approver", pgErr.ConstraintName,
		"Constraint name must be chk_sod_approver")
}

// ---------------------------------------------------------------------------
// INV-4: TestConcurrentApproval_ExactlyOneSucceeds
// ---------------------------------------------------------------------------

// TestConcurrentApproval_ExactlyOneSucceeds verifies that when N goroutines
// simultaneously attempt to approve the same pending_review donation, exactly
// ONE approval succeeds and the rest return ErrInvalidTransition (not internal
// errors). The SELECT … FOR UPDATE in Approve serializes the goroutines (D-52).
//
// Also asserts exactly 1 receipt_numbers row after the race (zero gaps, zero dupes).
//
// Run under -race (invoked by the plan's per-wave gate: go test -race ...).
// Requires Docker testcontainers. Skip with -short.
func TestConcurrentApproval_ExactlyOneSucceeds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", integTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)
	auditSvc := audit.NewAuditService(pool, queries, zap.NewNop())
	svc := donation.NewDonationService(pool, queries, alloc, auditSvc, kp, zap.NewNop())

	// Resolution now happens in auth.ResolveAppUser middleware (created-by-fk-mismatch fix);
	// calling the service directly, pass the resolved users.id as actingUserID.
	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "maker-conc@example.com", DisplayName: "Maker Concurrent",
		KeycloakSubject: "0a0ab46c-ee1f-4f33-a0f7-848f0b580609",
	})
	require.NoError(t, err)
	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-conc@example.com", DisplayName: "Checker Concurrent",
		KeycloakSubject: "5ae41b76-6d30-43f5-b0db-45977e255bfd",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{Subject: "0a0ab46c-ee1f-4f33-a0f7-848f0b580609", RealmAccess: auth.RealmRoles{Roles: []string{"maker"}}}
	checkerClaims := auth.KeycloakClaims{Subject: "5ae41b76-6d30-43f5-b0db-45977e255bfd", RealmAccess: auth.RealmRoles{Roles: []string{"checker"}}}

	d := createAndSubmit(t, ctx, svc, makerClaims, makerRow.ID,
		"นาย ทดสอบ Concurrent", "5555444433332", "2026-07-01", 12000.00)

	const N = 5
	type result struct{ err error }
	results := make([]result, N)

	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	for i := 0; i < N; i++ {
		i := i
		g.Go(func() error {
			_, approveErr := svc.Approve(gctx, d.ID, checkerRow.ID, checkerClaims)
			mu.Lock()
			results[i] = result{err: approveErr}
			mu.Unlock()
			// Don't propagate individual errors — we assert counts after g.Wait().
			return nil
		})
	}
	require.NoError(t, g.Wait(), "errgroup must not fail (individual errors collected separately)")

	// Count successes vs ErrInvalidTransition.
	var successes, transitions, unexpected int
	for _, r := range results {
		switch {
		case r.err == nil:
			successes++
		case errors.Is(r.err, donation.ErrInvalidTransition):
			transitions++
		default:
			unexpected++
			t.Errorf("unexpected error from concurrent Approve: %v", r.err)
		}
	}

	assert.Equal(t, 1, successes,
		"exactly 1 concurrent Approve must succeed (INV-4)")
	assert.Equal(t, N-1, transitions,
		"%d concurrent Approve attempts must return ErrInvalidTransition (INV-4)", N-1)
	assert.Equal(t, 0, unexpected,
		"no unexpected errors from concurrent Approve")

	// DB-level assertion: exactly 1 receipt_numbers row (zero gaps, zero dupes).
	var ledgerCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM receipt_numbers`).Scan(&ledgerCount))
	assert.Equal(t, 1, ledgerCount,
		"exactly 1 receipt_numbers row must exist after %d concurrent approvals (INV-4)", N)

	// Assert: donation status is issued.
	var status db.DonationStatus
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status FROM donations WHERE id = $1`, d.ID).Scan(&status))
	assert.Equal(t, db.DonationStatusIssued, status,
		"donation must be in issued status after successful concurrent approval")
}

// ---------------------------------------------------------------------------
// Task 2: TestReturnToDraft
// ---------------------------------------------------------------------------

// TestReturnToDraft verifies that Return transitions pending_review → draft
// with the mandatory reason persisted and audit recorded (D-45, FR-12).
//
// Requires Docker testcontainers. Skip with -short.
func TestReturnToDraft(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", integTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)
	auditSvc := audit.NewAuditService(pool, queries, zap.NewNop())
	svc := donation.NewDonationService(pool, queries, alloc, auditSvc, kp, zap.NewNop())

	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "maker-return@example.com", DisplayName: "Maker Return",
		KeycloakSubject: "b6cf12ef-e2f9-40e4-9786-a3d5b8cca920",
	})
	require.NoError(t, err)
	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-return@example.com", DisplayName: "Checker Return",
		KeycloakSubject: "f867f504-6266-4d86-aeb0-81648c31fb07",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{Subject: "b6cf12ef-e2f9-40e4-9786-a3d5b8cca920", RealmAccess: auth.RealmRoles{Roles: []string{"maker"}}}
	checkerClaims := auth.KeycloakClaims{Subject: "f867f504-6266-4d86-aeb0-81648c31fb07", RealmAccess: auth.RealmRoles{Roles: []string{"checker"}}}

	d := createAndSubmit(t, ctx, svc, makerClaims, makerRow.ID,
		"นาย ทดสอบ Return", "3334445556667", "2026-08-01", 2500.00)
	require.Equal(t, "pending_review", d.Status)

	const returnReason = "ข้อมูลผู้บริจาคไม่ครบถ้วน กรุณาแก้ไข"

	returnedBefore := time.Now().UTC().Add(-time.Second)
	returned, err := svc.Return(ctx, d.ID, returnReason, checkerRow.ID, checkerClaims)
	require.NoError(t, err, "Return must succeed on pending_review donation")
	require.NotNil(t, returned)

	// Status must be draft (non-terminal — Maker can re-edit and re-submit).
	assert.Equal(t, "draft", returned.Status,
		"Return must transition status to draft (D-45)")

	// Review reason must be persisted in the response.
	require.NotNil(t, returned.ReviewReason,
		"review_reason must be set in response after Return")
	assert.Equal(t, returnReason, *returned.ReviewReason,
		"review_reason must match the provided reason (D-45)")

	// Reviewer identity and timestamp must be set.
	require.NotNil(t, returned.ReviewedBy,
		"reviewed_by must be set after Return")
	assert.Equal(t, checkerRow.ID.String(), *returned.ReviewedBy,
		"reviewed_by must be the checker's user ID")
	require.NotNil(t, returned.ReviewedAt,
		"reviewed_at must be set after Return")
	assert.True(t, returned.ReviewedAt.After(returnedBefore),
		"reviewed_at must be a recent timestamp")

	// Assert via DB: review_reason persisted in the donation row.
	var dbReason *string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT review_reason FROM donations WHERE id = $1`, d.ID).Scan(&dbReason))
	require.NotNil(t, dbReason, "review_reason must be non-null in DB after Return")
	assert.Equal(t, returnReason, *dbReason,
		"review_reason in DB must match the provided reason")

	// Assert: 1 audit row for donation.return action.
	var auditCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE action = 'donation.return'`).Scan(&auditCount))
	assert.Equal(t, 1, auditCount, "exactly 1 audit row for donation.return must exist")

	// Assert: draft can be returned-for-edit; returning a second time from draft → ErrInvalidTransition.
	_, err = svc.Return(ctx, d.ID, "second return attempt", checkerRow.ID, checkerClaims)
	require.Error(t, err, "Return on a draft record must fail")
	assert.ErrorIs(t, err, donation.ErrInvalidTransition,
		"Return on draft must return ErrInvalidTransition (only pending_review is returneable)")
}

// ---------------------------------------------------------------------------
// Task 2: TestRejectTerminal
// ---------------------------------------------------------------------------

// TestRejectTerminal verifies that Reject transitions pending_review → rejected
// (terminal state) with the mandatory reason persisted, and that no further
// transitions are possible on a rejected record (D-45, FR-12).
//
// Requires Docker testcontainers. Skip with -short.
func TestRejectTerminal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", integTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)
	auditSvc := audit.NewAuditService(pool, queries, zap.NewNop())
	svc := donation.NewDonationService(pool, queries, alloc, auditSvc, kp, zap.NewNop())

	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "maker-reject@example.com", DisplayName: "Maker Reject",
		KeycloakSubject: "35a7170d-8a9e-4f95-861f-f30596cae59a",
	})
	require.NoError(t, err)
	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-reject@example.com", DisplayName: "Checker Reject",
		KeycloakSubject: "d00798ce-a214-47fe-b456-4ef7d01d9ab7",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{Subject: "35a7170d-8a9e-4f95-861f-f30596cae59a", RealmAccess: auth.RealmRoles{Roles: []string{"maker"}}}
	checkerClaims := auth.KeycloakClaims{Subject: "d00798ce-a214-47fe-b456-4ef7d01d9ab7", RealmAccess: auth.RealmRoles{Roles: []string{"checker"}}}

	d := createAndSubmit(t, ctx, svc, makerClaims, makerRow.ID,
		"นาย ทดสอบ Reject Terminal", "7778889990001", "2026-09-01", 8000.00)

	const rejectReason = "หลักฐานการบริจาคปลอมแปลง — ปฏิเสธถาวร"

	rejected, err := svc.Reject(ctx, d.ID, rejectReason, checkerRow.ID, checkerClaims)
	require.NoError(t, err, "Reject must succeed on pending_review donation")
	require.NotNil(t, rejected)

	// Status must be rejected (terminal).
	assert.Equal(t, "rejected", rejected.Status,
		"Reject must transition status to rejected (D-45 terminal state)")

	// Review reason must be persisted.
	require.NotNil(t, rejected.ReviewReason,
		"review_reason must be set in response after Reject")
	assert.Equal(t, rejectReason, *rejected.ReviewReason,
		"review_reason must match the provided reason")

	// Assert: 1 audit row for donation.reject.
	var auditCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE action = 'donation.reject'`).Scan(&auditCount))
	assert.Equal(t, 1, auditCount, "exactly 1 audit row for donation.reject must exist")

	// Assert terminal: no further transitions allowed on rejected record.
	// Approve → ErrInvalidTransition.
	_, err = svc.Approve(ctx, d.ID, checkerRow.ID, checkerClaims)
	require.Error(t, err, "Approve on rejected record must fail")
	assert.ErrorIs(t, err, donation.ErrInvalidTransition,
		"Approve on rejected must return ErrInvalidTransition (terminal state)")

	// Return → ErrInvalidTransition.
	_, err = svc.Return(ctx, d.ID, "irrelevant", checkerRow.ID, checkerClaims)
	require.Error(t, err, "Return on rejected record must fail")
	assert.ErrorIs(t, err, donation.ErrInvalidTransition,
		"Return on rejected must return ErrInvalidTransition (terminal state)")

	// Submit → ErrInvalidTransition.
	_, err = svc.Submit(ctx, d.ID, makerClaims)
	require.Error(t, err, "Submit on rejected record must fail")
	assert.ErrorIs(t, err, donation.ErrInvalidTransition,
		"Submit on rejected must return ErrInvalidTransition (terminal state)")

	// Assert: no receipt_numbers row (rejected records never get a receipt number).
	var ledgerCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM receipt_numbers`).Scan(&ledgerCount))
	assert.Equal(t, 0, ledgerCount,
		"No receipt_numbers row must exist for a rejected donation")
}

// ---------------------------------------------------------------------------
// Remaining Wave 0 scaffolds — implemented in plan 03-06
// ---------------------------------------------------------------------------

// createAndIssue is a test helper that creates, submits, and approves a donation
// so it reaches 'issued' status with a receipt number. Returns the issued DonationResponse.
func createAndIssue(
	t *testing.T,
	ctx context.Context,
	svc *donation.DonationService,
	makerClaims, checkerClaims auth.KeycloakClaims,
	makerID, checkerID pgtype.UUID,
	donorName, taxID, date string,
	amount float64,
) *donation.DonationResponse {
	t.Helper()
	submitted := createAndSubmit(t, ctx, svc, makerClaims, makerID, donorName, taxID, date, amount)
	issued, err := svc.Approve(ctx, submitted.ID, checkerID, checkerClaims)
	require.NoError(t, err, "Approve must succeed")
	require.Equal(t, "issued", issued.Status, "donation must be issued after Approve")
	require.NotNil(t, issued.ReceiptFormatted, "receipt_formatted must be set after issuance")
	return issued
}

// TestCancelRetainsReceiptNumber verifies INV-5 (FR-19, D-47):
// after an issued donation is cancelled, receipt_number_id and receipt_formatted
// remain set on the donation row (no gap in the receipt sequence).
// Issuing a subsequent donation yields the consecutive next number.
//
// Requires Docker testcontainers. Skip with -short.
func TestCancelRetainsReceiptNumber(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", integTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)
	auditSvc := audit.NewAuditService(pool, queries, zap.NewNop())
	svc := donation.NewDonationService(pool, queries, alloc, auditSvc, kp, zap.NewNop())

	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "maker-cancel@example.com", DisplayName: "Maker Cancel",
		KeycloakSubject: "2c9c9237-fc74-4977-8ab5-a383d9761bdd",
	})
	require.NoError(t, err)
	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-cancel@example.com", DisplayName: "Checker Cancel",
		KeycloakSubject: "161a0286-6932-4f80-8063-9989f24e0850",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{Subject: "2c9c9237-fc74-4977-8ab5-a383d9761bdd", RealmAccess: auth.RealmRoles{Roles: []string{"maker"}}}
	checkerClaims := auth.KeycloakClaims{Subject: "161a0286-6932-4f80-8063-9989f24e0850", RealmAccess: auth.RealmRoles{Roles: []string{"checker"}}}

	// Issue donation A — gets receipt number N (first in the fiscal year).
	donationA := createAndIssue(t, ctx, svc, makerClaims, checkerClaims, makerRow.ID, checkerRow.ID,
		"นาย ทดสอบ ยกเลิก A", "1234567890123", "2026-07-01", 5000.00)
	require.Equal(t, "issued", donationA.Status)
	require.NotNil(t, donationA.ReceiptFormatted, "donation A must have a receipt number")
	receiptA := *donationA.ReceiptFormatted

	// Cancel donation A (Checker cancels with reason).
	cancelledA, err := svc.Cancel(ctx, donationA.ID, donation.CancelDonationRequest{
		Reason: "ยกเลิกเนื่องจากข้อมูลผิดพลาด",
	}, checkerRow.ID, checkerClaims)
	require.NoError(t, err, "Cancel must succeed on an issued donation")
	require.NotNil(t, cancelledA)

	// --- INV-5: Cancel retains receipt_number_id and receipt_formatted (FR-19, D-47) ---
	assert.Equal(t, "cancelled", cancelledA.Status,
		"cancelled donation must have status=cancelled")
	require.NotNil(t, cancelledA.ReceiptFormatted,
		"receipt_formatted must be set on a cancelled donation (no gap — FR-19)")
	assert.Equal(t, receiptA, *cancelledA.ReceiptFormatted,
		"receipt_formatted must NOT change after cancellation — number is retained (D-47)")

	// Verify receipt_number_id retained in raw DB (load-bearing invariant).
	var receiptNumberID *int64
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT receipt_number_id FROM donations WHERE id = $1`, donationA.ID).Scan(&receiptNumberID))
	require.NotNil(t, receiptNumberID,
		"receipt_number_id must be non-NULL on cancelled row (FR-19: number never deleted)")

	// Verify audit row for donation.cancel exists.
	var auditCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE action = 'donation.cancel'`).Scan(&auditCount))
	assert.Equal(t, 1, auditCount, "exactly 1 audit row for donation.cancel must exist")

	// Issue donation B — must get the consecutive next receipt number (no gap).
	donationB := createAndIssue(t, ctx, svc, makerClaims, checkerClaims, makerRow.ID, checkerRow.ID,
		"นาย ทดสอบ ยกเลิก B", "9876543210987", "2026-07-01", 3000.00)
	require.NotNil(t, donationB.ReceiptFormatted,
		"donation B must have a receipt number")

	// Both A and B receipt numbers must be in receipt_numbers ledger (2 total, no gaps).
	var ledgerCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM receipt_numbers`).Scan(&ledgerCount))
	assert.Equal(t, 2, ledgerCount,
		"exactly 2 receipt_numbers rows must exist (A issued, A cancelled but kept, B issued)")

	// Consecutive: B's running_no = A's running_no + 1.
	var runNoA, runNoB int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT rn.running_no FROM receipt_numbers rn
		 JOIN donations d ON d.receipt_number_id = rn.id
		 WHERE d.id = $1`, donationA.ID).Scan(&runNoA))
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT rn.running_no FROM receipt_numbers rn
		 JOIN donations d ON d.receipt_number_id = rn.id
		 WHERE d.id = $1`, donationB.ID).Scan(&runNoB))
	assert.Equal(t, runNoA+1, runNoB,
		"B's running_no must be A's running_no + 1 (gap-less after cancel, INV-5)")
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
		KeycloakSubject: "3ab05e3e-5f45-45b1-9b0b-20c33f8a0c29",
	})
	require.NoError(t, err)

	const plainTaxID = "1234567890123"

	claims := auth.KeycloakClaims{
		Subject:     "3ab05e3e-5f45-45b1-9b0b-20c33f8a0c29",
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	resp, err := svc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  "นาย ทดสอบ PII",
		DonorTaxID: plainTaxID,
		Amount:     2500.00,
		DonatedAt:  "2024-03-01",
	}, userRow.ID, claims)
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
// RevealPII returns the plaintext tax ID only when called with Checker or Admin claims.
// Maker claims receive ErrForbidden. Every reveal writes a pii.reveal audit row (D-13).
//
// Requires Docker testcontainers. Skip with -short.
func TestPII_RevealRequiresCheckerOrAdmin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", integTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)
	auditSvc := audit.NewAuditService(pool, queries, zap.NewNop())
	svc := donation.NewDonationService(pool, queries, alloc, auditSvc, kp, zap.NewNop())

	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "maker-pii@example.com", DisplayName: "Maker PII",
		KeycloakSubject: "9f275860-eff4-4ea5-b140-1536df5acc9a",
	})
	require.NoError(t, err)
	// checker row is created only so RevealPII's checker claims map to a real identity;
	// RevealPII does not write a REFERENCES users(id) column, so its ID is not threaded.
	_, err = queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-pii@example.com", DisplayName: "Checker PII",
		KeycloakSubject: "d773c130-5923-446b-ae72-95de41e5e679",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{Subject: "9f275860-eff4-4ea5-b140-1536df5acc9a", RealmAccess: auth.RealmRoles{Roles: []string{"maker"}}}
	checkerClaims := auth.KeycloakClaims{Subject: "d773c130-5923-446b-ae72-95de41e5e679", RealmAccess: auth.RealmRoles{Roles: []string{"checker"}}}

	const plainTaxID = "1234567890123"

	// Create a draft donation.
	d, err := svc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  "นาย ทดสอบ PII Reveal",
		DonorTaxID: plainTaxID,
		Amount:     7500.00,
		DonatedAt:  "2026-07-01",
	}, makerRow.ID, makerClaims)
	require.NoError(t, err)

	// --- Maker → ErrForbidden (D-46) ---
	_, err = svc.RevealPII(ctx, d.ID, makerClaims)
	require.Error(t, err, "RevealPII by Maker must return an error (D-46)")
	assert.ErrorIs(t, err, donation.ErrForbidden,
		"RevealPII by Maker must return ErrForbidden — only Checker/Admin may reveal (D-46)")

	// Assert: no audit row yet (reveal was forbidden — no audit for a denied request).
	var auditCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE action = 'pii.reveal'`).Scan(&auditCount))
	assert.Equal(t, 0, auditCount,
		"no pii.reveal audit row must exist after a forbidden reveal attempt (D-13)")

	// --- Checker → plaintext returned + audit row written (D-46, D-13) ---
	resp, err := svc.RevealPII(ctx, d.ID, checkerClaims)
	require.NoError(t, err, "RevealPII by Checker must succeed (D-46)")
	require.NotNil(t, resp)
	assert.Equal(t, plainTaxID, resp.DonorTaxIDPlaintext,
		"RevealPII must return the plaintext tax ID to authorized caller (D-46)")
	assert.Equal(t, d.ID, resp.DonationID,
		"RevealPII must include the donation ID in the response")

	// Audit row MUST exist (D-13: audit before returning plaintext).
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE action = 'pii.reveal'`).Scan(&auditCount))
	assert.Equal(t, 1, auditCount,
		"exactly 1 pii.reveal audit row must exist after Checker reveal (D-13)")
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
		KeycloakSubject: "bbad0171-6c36-47fc-915f-32604f33e0de",
	})
	require.NoError(t, err)

	const plainTaxID = "1234567890123"
	// pii.MaskNationalID("1234567890123") → "x-xxxx-xxxxx-x" + "0123"
	const expectedMask = "x-xxxx-xxxxx-x0123"

	claims := auth.KeycloakClaims{
		Subject:     "bbad0171-6c36-47fc-915f-32604f33e0de",
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	created, err := svc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  "นาย ทดสอบ Mask",
		DonorTaxID: plainTaxID,
		Amount:     1500.00,
		DonatedAt:  "2024-04-01",
	}, userRow.ID, claims)
	require.NoError(t, err)

	// GetByID must return masked value, not plaintext.
	got, err := svc.GetByID(ctx, created.ID, claims)
	require.NoError(t, err)

	assert.Equal(t, expectedMask, got.DonorTaxIDMasked,
		"GetByID must return masked tax ID (last-4 reveal) — never plaintext (T-03-09)")
	assert.NotEqual(t, plainTaxID, got.DonorTaxIDMasked,
		"DonorTaxIDMasked must not equal the plaintext tax ID")
}

// TestVoidAndReissue verifies D-50 void & reissue flow (FR-19):
// Reissue cancels the original (sets replaced_by), creates a corrected draft (sets replaces),
// the original retains its receipt number (no gap), and the replacement earns a fresh
// consecutive number only via the normal Submit → Approve path (no bypass of SoD).
//
// Requires Docker testcontainers. Skip with -short.
func TestVoidAndReissue(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", integTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)
	auditSvc := audit.NewAuditService(pool, queries, zap.NewNop())
	svc := donation.NewDonationService(pool, queries, alloc, auditSvc, kp, zap.NewNop())

	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "maker-reissue@example.com", DisplayName: "Maker Reissue",
		KeycloakSubject: "745dd9e9-5444-496c-a27f-d8389f600e03",
	})
	require.NoError(t, err)
	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-reissue@example.com", DisplayName: "Checker Reissue",
		KeycloakSubject: "7ca65f0e-7550-4d67-b704-08d3cde5d09b",
	})
	require.NoError(t, err)
	// checker2: approves the replacement draft.
	// Needed because Reissue sets created_by=checker1, so checker1 cannot also Approve (SoD).
	checker2Row, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker2-reissue@example.com", DisplayName: "Checker2 Reissue",
		KeycloakSubject: "cd32185b-082a-4763-9df3-3207620d978e",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{Subject: "745dd9e9-5444-496c-a27f-d8389f600e03", RealmAccess: auth.RealmRoles{Roles: []string{"maker"}}}
	checkerClaims := auth.KeycloakClaims{Subject: "7ca65f0e-7550-4d67-b704-08d3cde5d09b", RealmAccess: auth.RealmRoles{Roles: []string{"checker"}}}
	checker2Claims := auth.KeycloakClaims{Subject: "cd32185b-082a-4763-9df3-3207620d978e", RealmAccess: auth.RealmRoles{Roles: []string{"checker"}}}

	// Issue the original donation (gets receipt number 1).
	original := createAndIssue(t, ctx, svc, makerClaims, checkerClaims, makerRow.ID, checkerRow.ID,
		"นาย ทดสอบ Reissue Original", "1234567890123", "2026-07-01", 10000.00)
	require.Equal(t, "issued", original.Status)
	require.NotNil(t, original.ReceiptFormatted)
	origReceipt := *original.ReceiptFormatted

	// Perform Void & Reissue (Checker cancels + creates corrected draft).
	replacement, err := svc.Reissue(ctx, original.ID, donation.ReissueDonationRequest{
		Reason:     "ยกเลิกเพื่อแก้ไขข้อมูลผู้บริจาค",
		DonorName:  "นาย ทดสอบ Reissue แก้ไข",
		DonorTaxID: "9876543210987",
		Amount:     10000.00,
		DonatedAt:  "2026-07-01",
	}, checkerRow.ID, checkerClaims)
	require.NoError(t, err, "Reissue must succeed on an issued donation")
	require.NotNil(t, replacement)

	// --- Replacement draft is at status='draft' (no bypass of maker-checker, D-50) ---
	assert.Equal(t, "draft", replacement.Status,
		"replacement donation must be created at draft status — no bypass of SoD (D-50)")
	assert.Nil(t, replacement.ReceiptFormatted,
		"replacement draft must NOT have a receipt number — earned only via Approve (D-50)")

	// --- replaces link: new.replaces = original.ID ---
	require.NotNil(t, replacement.Replaces,
		"replacement.Replaces must be set to original ID (D-50)")
	assert.Equal(t, original.ID, *replacement.Replaces,
		"replacement.Replaces must point to the original donation ID")

	// --- replaced_by link on original: original.replaced_by = replacement.ID ---
	var origReplacedBy *string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT replaced_by::text FROM donations WHERE id = $1`, original.ID).Scan(&origReplacedBy))
	require.NotNil(t, origReplacedBy, "original.replaced_by must be set to replacement ID (D-50)")
	assert.Equal(t, replacement.ID, *origReplacedBy,
		"original.replaced_by must point to the replacement donation ID")

	// --- Original retains its receipt number (no gap) ---
	var origStatus string
	var origReceiptFormatted *string
	var origReceiptNumberID *int64
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status, receipt_formatted, receipt_number_id FROM donations WHERE id = $1`,
		original.ID).Scan(&origStatus, &origReceiptFormatted, &origReceiptNumberID))
	assert.Equal(t, "cancelled", origStatus,
		"original must be cancelled after Reissue")
	require.NotNil(t, origReceiptFormatted,
		"original.receipt_formatted must be retained after Reissue (no gap — FR-19)")
	assert.Equal(t, origReceipt, *origReceiptFormatted,
		"original.receipt_formatted must not change after Reissue")
	require.NotNil(t, origReceiptNumberID,
		"original.receipt_number_id must be retained after Reissue (no gap)")

	// Audit row for donation.reissue must exist.
	var auditCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE action = 'donation.reissue'`).Scan(&auditCount))
	assert.Equal(t, 1, auditCount, "exactly 1 audit row for donation.reissue must exist")

	// --- Replacement earns a fresh consecutive number via normal Submit → Approve (D-50) ---
	// Submit the replacement draft (a different maker, or same maker is fine since it's a NEW record).
	submittedRep, err := svc.Submit(ctx, replacement.ID, makerClaims)
	require.NoError(t, err, "Submit on replacement draft must succeed")
	require.Equal(t, "pending_review", submittedRep.Status)

	// Approve the replacement via a DIFFERENT checker (SoD: created_by=checker1, approver=checker2).
	// Reissue sets created_by=checkerClaims.Subject, so checkerClaims cannot also Approve.
	issuedRep, err := svc.Approve(ctx, replacement.ID, checker2Row.ID, checker2Claims)
	require.NoError(t, err, "Approve on submitted replacement must succeed")
	require.NotNil(t, issuedRep)
	assert.Equal(t, "issued", issuedRep.Status,
		"replacement must reach issued status via normal Approve")
	require.NotNil(t, issuedRep.ReceiptFormatted,
		"replacement must earn a fresh receipt number via normal approval (D-50)")

	// The replacement's receipt number must be fresh (different from original).
	assert.NotEqual(t, origReceipt, *issuedRep.ReceiptFormatted,
		"replacement must earn a DIFFERENT receipt number (fresh, not reuse of original)")

	// Exactly 2 receipt_numbers rows: original + replacement (both in ledger, no gaps).
	var ledgerCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM receipt_numbers`).Scan(&ledgerCount))
	assert.Equal(t, 2, ledgerCount,
		"exactly 2 receipt_numbers rows must exist (original + replacement)")
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
		KeycloakSubject: "c3f22de7-8266-4cb4-9f7d-5238a4923b83",
	})
	require.NoError(t, err)

	claims := auth.KeycloakClaims{
		Subject:     "c3f22de7-8266-4cb4-9f7d-5238a4923b83",
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	// Create a draft donation.
	before := time.Now().UTC().Add(-time.Second)
	draft, err := svc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  "นาย ทดสอบ Submit",
		DonorTaxID: "1111222233334",
		Amount:     7500.00,
		DonatedAt:  "2024-07-01",
	}, userRow.ID, claims)
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
// each filter is independent and can be nil (no restriction applied to that dimension).
// Tax ID is NOT a searchable field (D-53, T-03-29).
//
// Requires Docker testcontainers. Skip with -short.
func TestSearchDonations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", integTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)
	auditSvc := audit.NewAuditService(pool, queries, zap.NewNop())
	svc := donation.NewDonationService(pool, queries, alloc, auditSvc, kp, zap.NewNop())

	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "maker-search@example.com", DisplayName: "Maker Search",
		KeycloakSubject: "bcbdc376-fb64-42b9-b1f2-0651cb8834ff",
	})
	require.NoError(t, err)
	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-search@example.com", DisplayName: "Checker Search",
		KeycloakSubject: "09682ff1-e1ff-4572-ac6e-b59cc3586847",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{Subject: "bcbdc376-fb64-42b9-b1f2-0651cb8834ff", RealmAccess: auth.RealmRoles{Roles: []string{"maker"}}}
	checkerClaims := auth.KeycloakClaims{Subject: "09682ff1-e1ff-4572-ac6e-b59cc3586847", RealmAccess: auth.RealmRoles{Roles: []string{"checker"}}}

	// Create test donations:
	//   - "สมชาย" status=draft, donated 2026-01-10
	//   - "สมหญิง" status=pending_review, donated 2026-02-15
	//   - "ประยุทธ" status=issued (has receipt), donated 2026-03-20

	dA, err := svc.Create(ctx, donation.CreateDonationRequest{
		DonorName: "สมชาย สุขใจ", DonorTaxID: "1111111111111", Amount: 1000.00, DonatedAt: "2026-01-10",
	}, makerRow.ID, makerClaims)
	require.NoError(t, err, "Create donation A must succeed")

	dBSubmit := createAndSubmit(t, ctx, svc, makerClaims, makerRow.ID, "สมหญิง ดีใจ", "2222222222222", "2026-02-15", 2000.00)

	dC := createAndIssue(t, ctx, svc, makerClaims, checkerClaims, makerRow.ID, checkerRow.ID,
		"ประยุทธ เก่งมาก", "3333333333333", "2026-03-20", 3000.00)
	receiptC := *dC.ReceiptFormatted

	// --- No filters: returns all 3 donations, total is a real COUNT (not len(items)) ---
	all, allTotal, err := svc.Search(ctx, donation.ListFilter{Limit: 20, Offset: 0}, checkerClaims)
	require.NoError(t, err, "Search with no filters must succeed")
	assert.GreaterOrEqual(t, len(all), 3,
		"no-filter search must return at least the 3 donations we created")
	assert.Equal(t, int64(len(all)), allTotal,
		"total must equal the real COUNT for a single-page result (D-R2)")

	// --- Filter by donor_name ILIKE "สมชาย" → only donation A ---
	name := "สมชาย"
	byName, byNameTotal, err := svc.Search(ctx, donation.ListFilter{DonorName: &name, Limit: 20}, checkerClaims)
	require.NoError(t, err, "Search by donor_name must succeed")
	require.Len(t, byName, 1, "exactly 1 donation must match 'สมชาย' ILIKE filter")
	assert.Equal(t, dA.ID, byName[0].ID, "donor_name filter must return donation A")
	assert.Equal(t, dA.CreatedBy, byName[0].CreatedByID, "created_by_id must be the raw creator UUID")
	assert.Equal(t, "Maker Search", byName[0].CreatedBy, "created_by must be the creator's display name (join)")
	assert.Equal(t, int64(1), byNameTotal, "CountDonations total must mirror the donor_name filter")

	// --- Filter by status=draft → only donation A (สมชาย is still draft) ---
	statusDraft := "draft"
	byStatus, byStatusTotal, err := svc.Search(ctx, donation.ListFilter{Status: &statusDraft, Limit: 20}, checkerClaims)
	require.NoError(t, err, "Search by status must succeed")
	ids := make([]string, len(byStatus))
	for i, r := range byStatus {
		ids[i] = r.ID
	}
	assert.Contains(t, ids, dA.ID, "status=draft filter must include donation A")
	for _, r := range byStatus {
		assert.Equal(t, "draft", r.Status, "all results of status=draft filter must have status=draft")
	}
	assert.Equal(t, int64(len(byStatus)), byStatusTotal, "total must mirror the status=draft filter count")

	// --- Filter by from_date / to_date ---
	// from=2026-02-01, to=2026-02-28 → only "สมหญิง" (donated 2026-02-15)
	from := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	byDate, byDateTotal, err := svc.Search(ctx, donation.ListFilter{FromDate: &from, ToDate: &to, Limit: 20}, checkerClaims)
	require.NoError(t, err, "Search by date range must succeed")
	require.Len(t, byDate, 1, "exactly 1 donation must fall in 2026-02 range")
	assert.Equal(t, dBSubmit.ID, byDate[0].ID, "date-range filter must return donation B (สมหญิง)")
	assert.Equal(t, int64(1), byDateTotal, "total must mirror the date-range filter count")

	// --- Filter by receipt_no → only issued donation C ---
	byReceipt, byReceiptTotal, err := svc.Search(ctx, donation.ListFilter{ReceiptNo: &receiptC, Limit: 20}, checkerClaims)
	require.NoError(t, err, "Search by receipt_no must succeed")
	require.Len(t, byReceipt, 1, "exactly 1 donation must match the receipt number filter")
	assert.Equal(t, dC.ID, byReceipt[0].ID, "receipt_no filter must return donation C")
	assert.Equal(t, int64(1), byReceiptTotal, "total must mirror the receipt_no filter count")

	// --- total must be a real COUNT, not len(items): request a page smaller than the
	// result set and assert total still reflects the full filtered count (D-R2, T-09) ---
	firstPage, firstPageTotal, err := svc.Search(ctx, donation.ListFilter{Limit: 1, Offset: 0}, checkerClaims)
	require.NoError(t, err, "Search with a 1-row page must succeed")
	require.Len(t, firstPage, 1, "page size must be honoured")
	assert.GreaterOrEqual(t, firstPageTotal, int64(3),
		"total on a partial page must be the full filtered COUNT, not len(items)==1")
}

// TestEDonationKeyedGuard_Integration verifies the edonation_keyed=true guard (D-51):
// when a donation has edonation_keyed=true, cancellation requires a non-empty
// rd_confirmation_reason; with it, cancellation succeeds and the reason is in audit.
//
// Requires Docker testcontainers. Skip with -short.
func TestEDonationKeyedGuard_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", integTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)
	auditSvc := audit.NewAuditService(pool, queries, zap.NewNop())
	svc := donation.NewDonationService(pool, queries, alloc, auditSvc, kp, zap.NewNop())

	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "maker-edon@example.com", DisplayName: "Maker EDon",
		KeycloakSubject: "550ae95a-f2f1-455f-8a26-278899531ebd",
	})
	require.NoError(t, err)
	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-edon@example.com", DisplayName: "Checker EDon",
		KeycloakSubject: "f74794d0-b6a8-49ac-9cc2-bfc00ebdf8b9",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{Subject: "550ae95a-f2f1-455f-8a26-278899531ebd", RealmAccess: auth.RealmRoles{Roles: []string{"maker"}}}
	checkerClaims := auth.KeycloakClaims{Subject: "f74794d0-b6a8-49ac-9cc2-bfc00ebdf8b9", RealmAccess: auth.RealmRoles{Roles: []string{"checker"}}}

	// Issue a donation.
	d := createAndIssue(t, ctx, svc, makerClaims, checkerClaims, makerRow.ID, checkerRow.ID,
		"นาย ทดสอบ eDonation Keyed", "5555666677778", "2026-07-01", 20000.00)
	require.Equal(t, "issued", d.Status)

	// Set edonation_keyed=true via raw SQL (simulating that it was keyed into RD system).
	_, err = pool.Exec(ctx, `UPDATE donations SET edonation_keyed = true WHERE id = $1`, d.ID)
	require.NoError(t, err, "raw UPDATE of edonation_keyed must succeed")

	// Attempt cancel WITHOUT rd_confirmation_reason → ErrEDonationKeyedCancel (D-51).
	_, err = svc.Cancel(ctx, d.ID, donation.CancelDonationRequest{
		Reason:               "ยกเลิก",
		RDConfirmationReason: "", // missing!
	}, checkerRow.ID, checkerClaims)
	require.Error(t, err, "Cancel with edonation_keyed=true and no rd_confirmation_reason must error")
	assert.ErrorIs(t, err, donation.ErrEDonationKeyedCancel,
		"Must return ErrEDonationKeyedCancel when edonation_keyed=true and rd_confirmation_reason is empty (D-51)")

	// Status must still be 'issued' (the cancel was rejected).
	var status string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status FROM donations WHERE id = $1`, d.ID).Scan(&status))
	assert.Equal(t, "issued", status, "status must remain issued after rejected cancel attempt")

	// Cancel WITH rd_confirmation_reason → succeeds (D-51).
	const rdReason = "ยืนยันการแก้ไขข้อมูลกับ e-Donation รอบที่ 3"
	cancelled, err := svc.Cancel(ctx, d.ID, donation.CancelDonationRequest{
		Reason:               "ยกเลิกหลังยืนยัน RD",
		RDConfirmationReason: rdReason,
	}, checkerRow.ID, checkerClaims)
	require.NoError(t, err, "Cancel with edonation_keyed=true and rd_confirmation_reason must succeed (D-51)")
	assert.Equal(t, "cancelled", cancelled.Status, "status must be cancelled after successful cancel")

	// Audit row must exist and contain the rd_confirmation_reason.
	var auditAfterJSON []byte
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT after_json FROM audit_log WHERE action = 'donation.cancel' ORDER BY created_at DESC LIMIT 1`,
	).Scan(&auditAfterJSON))
	require.NotNil(t, auditAfterJSON, "audit after_json must be set for donation.cancel")
	assert.Contains(t, string(auditAfterJSON), "rd_confirmation_reason",
		"audit after_json must contain rd_confirmation_reason when edonation_keyed (D-51)")
	assert.Contains(t, string(auditAfterJSON), rdReason,
		"audit after_json must contain the actual rd_confirmation_reason value")
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
		KeycloakSubject: "32c1f156-fb70-4b31-a8c7-141d6a5131fb",
	})
	require.NoError(t, err)

	claims := auth.KeycloakClaims{
		Subject:     "32c1f156-fb70-4b31-a8c7-141d6a5131fb",
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

	resp, err := svc.Create(ctx, req, userRow.ID, claims)
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
		KeycloakSubject: "5968bf3c-f47b-43f6-9117-55eb5b3e79b9",
	})
	require.NoError(t, err)

	claims := auth.KeycloakClaims{
		Subject:     "5968bf3c-f47b-43f6-9117-55eb5b3e79b9",
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	// Create a draft first.
	draft, err := svc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  "นาย ทดสอบ แก้ไข",
		DonorTaxID: "1112223334445",
		Amount:     8000.00,
		DonatedAt:  "2024-02-14",
	}, userRow.ID, claims)
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
