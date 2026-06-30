// Package donation_test — integration tests for the donation service.
//
// This file implements the 7 hardest invariants from RESEARCH.md §"The 7 Hardest Invariants".
// All tests require a live PostgreSQL container (testcontainers). Skip with -short.
//
// Plan 03-05 tests (issuance tx, SoD, concurrency, return/reject):
//   INV-1: TestIssuanceTransaction_RollbackOnError  — atomicity of 7-step approve tx
//   INV-2: TestOutboxAtomicity                      — outbox row IFF receipt issued
//   INV-3: TestSoD_ApproverCannotBeCreator          — code-level SoD guard
//   INV-3: TestSoD_DBCheckConstraint                — DB CHECK constraint backstop
//   INV-4: TestConcurrentApproval_ExactlyOneSucceeds — FOR UPDATE serializes N goroutines
//          TestReturnToDraft                        — pending_review → draft + audit
//          TestRejectTerminal                       — pending_review → rejected (terminal)
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
	donorName, taxID, date string,
	amount float64,
) *donation.DonationResponse {
	t.Helper()
	d, err := svc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  donorName,
		DonorTaxID: taxID,
		Amount:     amount,
		DonatedAt:  date,
	}, makerClaims)
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
//   A: rollback after Allocate (before IssueDonation)  → ledger 0 rows, status=pending_review
//   B: rollback after IssueDonation (before audit)     → status=pending_review, outbox 0 rows
//   C: happy path (full commit)                         → status=issued, 1 ledger row, 1 outbox
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

	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "maker-rollback@example.com", DisplayName: "Maker Rollback",
		KeycloakSubject: "maker-rollback-kc",
	})
	require.NoError(t, err)
	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-rollback@example.com", DisplayName: "Checker Rollback",
		KeycloakSubject: "checker-rollback-kc",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{Subject: makerRow.ID.String(), RealmAccess: auth.RealmRoles{Roles: []string{"maker"}}}
	checkerClaims := auth.KeycloakClaims{Subject: checkerRow.ID.String(), RealmAccess: auth.RealmRoles{Roles: []string{"checker"}}}

	// ---- Scenario A: rollback after Allocate, before IssueDonation ----------------
	t.Run("ScenarioA_RollbackAfterAllocate", func(t *testing.T) {
		d := createAndSubmit(t, ctx, svc, makerClaims,
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
		d := createAndSubmit(t, ctx, svc, makerClaims,
			"นาย ทดสอบ Rollback B", "9876543210987", "2026-03-15", 2000.00)

		var pgUUID pgtype.UUID
		require.NoError(t, pgUUID.Scan(d.ID))

		var checkerUUID pgtype.UUID
		require.NoError(t, checkerUUID.Scan(checkerClaims.Subject))

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
		d := createAndSubmit(t, ctx, svc, makerClaims,
			"นาย ทดสอบ Happy Path", "1111222233334", "2026-03-15", 5000.00)

		approved, err := svc.Approve(ctx, d.ID, checkerClaims)
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

	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "maker-outbox@example.com", DisplayName: "Maker Outbox",
		KeycloakSubject: "maker-outbox-kc",
	})
	require.NoError(t, err)
	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-outbox@example.com", DisplayName: "Checker Outbox",
		KeycloakSubject: "checker-outbox-kc",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{Subject: makerRow.ID.String(), RealmAccess: auth.RealmRoles{Roles: []string{"maker"}}}
	checkerClaims := auth.KeycloakClaims{Subject: checkerRow.ID.String(), RealmAccess: auth.RealmRoles{Roles: []string{"checker"}}}

	// Part 1: Rollback → no outbox row persisted.
	t.Run("RollbackNoOutbox", func(t *testing.T) {
		d := createAndSubmit(t, ctx, svc, makerClaims,
			"นาย ทดสอบ Outbox Rollback", "1234512345123", "2026-04-01", 3000.00)

		var pgUUID pgtype.UUID
		require.NoError(t, pgUUID.Scan(d.ID))

		var checkerUUID pgtype.UUID
		require.NoError(t, checkerUUID.Scan(checkerClaims.Subject))

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
		d := createAndSubmit(t, ctx, svc, makerClaims,
			"นาย ทดสอบ Outbox Success", "9999888877776", "2026-04-01", 4000.00)

		_, err := svc.Approve(ctx, d.ID, checkerClaims)
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
	userRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "dual-role@example.com", DisplayName: "Dual Role",
		KeycloakSubject: "dual-role-kc",
	})
	require.NoError(t, err)

	dualClaims := auth.KeycloakClaims{
		Subject:     userRow.ID.String(),
		RealmAccess: auth.RealmRoles{Roles: []string{"maker", "checker"}},
	}

	d := createAndSubmit(t, ctx, svc, dualClaims,
		"นาย ทดสอบ SoD Code", "1231231231231", "2026-05-01", 7500.00)

	// Approve with the same user who created — must return ErrSoDViolation.
	_, approveErr := svc.Approve(ctx, d.ID, dualClaims)
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

	userRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "sod-db-test@example.com", DisplayName: "SoD DB Test",
		KeycloakSubject: "sod-db-kc",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{
		Subject:     userRow.ID.String(),
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	d, err := svc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  "นาย ทดสอบ SoD DB",
		DonorTaxID: "1111333355557",
		Amount:     1500.00,
		DonatedAt:  "2026-06-01",
	}, makerClaims)
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

	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "maker-conc@example.com", DisplayName: "Maker Concurrent",
		KeycloakSubject: "maker-conc-kc",
	})
	require.NoError(t, err)
	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-conc@example.com", DisplayName: "Checker Concurrent",
		KeycloakSubject: "checker-conc-kc",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{Subject: makerRow.ID.String(), RealmAccess: auth.RealmRoles{Roles: []string{"maker"}}}
	checkerClaims := auth.KeycloakClaims{Subject: checkerRow.ID.String(), RealmAccess: auth.RealmRoles{Roles: []string{"checker"}}}

	d := createAndSubmit(t, ctx, svc, makerClaims,
		"นาย ทดสอบ Concurrent", "5555444433332", "2026-07-01", 12000.00)

	const N = 5
	type result struct{ err error }
	results := make([]result, N)

	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	for i := 0; i < N; i++ {
		i := i
		g.Go(func() error {
			_, approveErr := svc.Approve(gctx, d.ID, checkerClaims)
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
		KeycloakSubject: "maker-return-kc",
	})
	require.NoError(t, err)
	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-return@example.com", DisplayName: "Checker Return",
		KeycloakSubject: "checker-return-kc",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{Subject: makerRow.ID.String(), RealmAccess: auth.RealmRoles{Roles: []string{"maker"}}}
	checkerClaims := auth.KeycloakClaims{Subject: checkerRow.ID.String(), RealmAccess: auth.RealmRoles{Roles: []string{"checker"}}}

	d := createAndSubmit(t, ctx, svc, makerClaims,
		"นาย ทดสอบ Return", "3334445556667", "2026-08-01", 2500.00)
	require.Equal(t, "pending_review", d.Status)

	const returnReason = "ข้อมูลผู้บริจาคไม่ครบถ้วน กรุณาแก้ไข"

	returnedBefore := time.Now().UTC().Add(-time.Second)
	returned, err := svc.Return(ctx, d.ID, returnReason, checkerClaims)
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
	_, err = svc.Return(ctx, d.ID, "second return attempt", checkerClaims)
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
		KeycloakSubject: "maker-reject-kc",
	})
	require.NoError(t, err)
	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-reject@example.com", DisplayName: "Checker Reject",
		KeycloakSubject: "checker-reject-kc",
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{Subject: makerRow.ID.String(), RealmAccess: auth.RealmRoles{Roles: []string{"maker"}}}
	checkerClaims := auth.KeycloakClaims{Subject: checkerRow.ID.String(), RealmAccess: auth.RealmRoles{Roles: []string{"checker"}}}

	d := createAndSubmit(t, ctx, svc, makerClaims,
		"นาย ทดสอบ Reject Terminal", "7778889990001", "2026-09-01", 8000.00)

	const rejectReason = "หลักฐานการบริจาคปลอมแปลง — ปฏิเสธถาวร"

	rejected, err := svc.Reject(ctx, d.ID, rejectReason, checkerClaims)
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
	_, err = svc.Approve(ctx, d.ID, checkerClaims)
	require.Error(t, err, "Approve on rejected record must fail")
	assert.ErrorIs(t, err, donation.ErrInvalidTransition,
		"Approve on rejected must return ErrInvalidTransition (terminal state)")

	// Return → ErrInvalidTransition.
	_, err = svc.Return(ctx, d.ID, "irrelevant", checkerClaims)
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
// Remaining Wave 0 scaffolds — implemented in later plans
// ---------------------------------------------------------------------------

// TestCancelRetainsReceiptNumber verifies INV-5 (FR-19, D-47):
// after an issued donation is cancelled, receipt_number_id and receipt_formatted
// remain set on the donation row (no gap in the receipt sequence).
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
