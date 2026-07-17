// Package donation_test — integration tests for CreatePublicSubmission (plan 06-03).
//
// CreatePublicSubmission is the load-bearing Flow B path: a donor's complete
// request must land ATOMICALLY in pending_review as source=flow_b, with the tax
// ID envelope-encrypted, consent snapshot captured, slip referenced, the submit
// audited under the seeded public-web system actor, and exactly one ack_email
// outbox job enqueued — never a partial/orphan record (D-76/78/79/80/81,
// FR-01/02/03/04). A forced in-tx error must roll the whole thing back.
//
// This mirrors service_integration_test.go's fixtures (testcontainers Postgres,
// full 000001-000016 migration chain so the public-web user is seeded). The slip
// object is pre-uploaded to MinIO by the handler BEFORE the tx in production; at
// the service layer only the object-key REFERENCE is inserted, so no MinIO client
// is needed here (InsertSlip stores a plain string key).
//
// Requires Docker testcontainers. Skip with -short.
package donation_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/donnarec/donnarec-api/internal/audit"
	"github.com/donnarec/donnarec-api/internal/crypto"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/donation"
	"github.com/donnarec/donnarec-api/internal/receiptno"
	"github.com/donnarec/donnarec-api/internal/testutil"
)

// publicSubTestKEK is a 32-byte hex key for integration test use only (same
// value as service_integration_test.go's integTestKEK — test-only, never a real
// secret).
const publicSubTestKEK = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

// newPublicSubmissionSvc wires a DonationService against a fresh testcontainers
// Postgres (full migration chain applied, so the public-web system user is
// seeded), returning the service, the pool (for direct DB assertions), ctx, and
// the resolved public-web UUID.
func newPublicSubmissionSvc(t *testing.T) (*donation.DonationService, *pgxpool.Pool, context.Context, pgtype.UUID) {
	t.Helper()
	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", publicSubTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)
	auditSvc := audit.NewAuditService(pool, queries, zap.NewNop())
	svc := donation.NewDonationService(pool, queries, alloc, auditSvc, kp, zap.NewNop())

	// The public-web system user (id == keycloak_subject == the fixed UUID) is
	// seeded by migration 000016 — Pitfall 1: a UUID-shaped actor, never a
	// human-readable sentinel, so AppendAuditEntryTx's parseUUID never rolls back.
	var publicWebID pgtype.UUID
	require.NoError(t, publicWebID.Scan(donation.PublicWebUserID),
		"donation.PublicWebUserID must be a valid UUID string")

	return svc, pool, ctx, publicWebID
}

// validPublicRequest returns a complete, valid Flow B donor submission.
func validPublicRequest(name string) donation.PublicDonationRequest {
	return donation.PublicDonationRequest{
		DonorName:          name,
		DonorTaxID:         "1234567890123",
		DonorAddress:       "123 ถนนสาธารณะ กรุงเทพฯ",
		DonorEmail:         "donor@example.com",
		Amount:             2500.00,
		DonatedAt:          "2026-03-15",
		ConsentGiven:       true,
		ConsentTextVersion: "public-form-v1",
		ConsentPurpose:     "tax-receipt",
		DonorLanguage:      "th",
	}
}

// TestCreatePublicSubmission proves the atomic Flow B create+submit+slip+audit+outbox path.
//
// Requires Docker testcontainers. Skip with -short.
func TestCreatePublicSubmission(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	svc, pool, ctx, publicWebID := newPublicSubmissionSvc(t)

	t.Run("HappyPath_AtomicPendingReviewFlowB", func(t *testing.T) {
		req := validPublicRequest("นาย ทดสอบ Flow B")
		const objectKey = "slips/public-test/abc123.jpg"
		const mimeType = "image/jpeg"
		const sizeBytes int64 = 4096

		resp, err := svc.CreatePublicSubmission(ctx, req, objectKey, mimeType, sizeBytes, publicWebID)
		require.NoError(t, err, "CreatePublicSubmission must succeed")
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.ID)

		// --- Response-level assertions ---
		assert.Equal(t, "pending_review", resp.Status, "Flow B lands directly in pending_review")
		assert.True(t, resp.ConsentGiven)
		require.NotNil(t, resp.ConsentTextVersion)
		assert.Equal(t, "public-form-v1", *resp.ConsentTextVersion, "Flow-B-specific consent version (D-81)")
		assert.Equal(t, publicWebID.String(), resp.CreatedByID, "created_by is the public-web system user")
		assert.Nil(t, resp.ReceiptFormatted, "no receipt number until Checker approval (D-84)")

		// --- DB-level assertions on the donations row ---
		var (
			status       string
			source       string
			createdBy    pgtype.UUID
			taxIDEnc     []byte
			consentGiven bool
			receiptID    *int64
		)
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT status, source, created_by, donor_tax_id_enc, consent_given, receipt_number_id
			   FROM donations WHERE id = $1`, resp.ID,
		).Scan(&status, &source, &createdBy, &taxIDEnc, &consentGiven, &receiptID))
		assert.Equal(t, "pending_review", status)
		assert.Equal(t, "flow_b", source, "public submissions must be source=flow_b (D-77)")
		assert.Equal(t, publicWebID.String(), createdBy.String())
		assert.NotEmpty(t, taxIDEnc, "tax ID ciphertext must be present (envelope-encrypted, D-79)")
		assert.NotContains(t, string(taxIDEnc), "1234567890123", "plaintext tax ID must NEVER reach Postgres")
		assert.True(t, consentGiven)
		assert.Nil(t, receiptID, "receipt_number_id must be NULL until approval")

		// --- Exactly one slip_attachments row referencing the passed object key ---
		var slipCount int
		var storedKey, storedMime string
		var storedSize int64
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT count(*) FROM slip_attachments WHERE donation_id = $1 AND deleted_at IS NULL`, resp.ID,
		).Scan(&slipCount))
		assert.Equal(t, 1, slipCount, "exactly one slip row (D-80 mandatory slip)")
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT object_key, mime_type, size_bytes FROM slip_attachments WHERE donation_id = $1`, resp.ID,
		).Scan(&storedKey, &storedMime, &storedSize))
		assert.Equal(t, objectKey, storedKey)
		assert.Equal(t, mimeType, storedMime)
		assert.Equal(t, sizeBytes, storedSize)

		// --- Exactly one ack_email outbox job carrying the donation id; NO issue_receipt ---
		var ackCount int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT count(*) FROM outbox_jobs WHERE job_type = 'ack_email' AND payload->>'donation_id' = $1`, resp.ID,
		).Scan(&ackCount))
		assert.Equal(t, 1, ackCount, "exactly one ack_email job with the donation id (D-85)")

		var issueCount int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT count(*) FROM outbox_jobs WHERE job_type = 'issue_receipt' AND payload->>'donation_id' = $1`, resp.ID,
		).Scan(&issueCount))
		assert.Equal(t, 0, issueCount, "NO issue_receipt job at submit (receipts only at approval)")

		// --- Exactly one audit row for the public submit under the public-web UUID actor ---
		var auditCount int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT count(*) FROM audit_log WHERE actor_id = $1 AND action = 'donation.public_submit'`,
			publicWebID.String(),
		).Scan(&auditCount))
		assert.Equal(t, 1, auditCount, "one audit row, public-web UUID actor (Pitfall 1 — no parseUUID rollback)")
	})

	t.Run("Rollback_ForcedError_NoOrphanRow", func(t *testing.T) {
		// A non-provisioned created_by UUID makes CreateDonation's FK fail INSIDE the
		// WithTx closure — the whole tx must roll back, leaving zero new donations rows
		// (proves the all-or-nothing guarantee; a partial pending_review record without
		// a slip would silently violate D-80).
		var donationsBefore int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM donations`).Scan(&donationsBefore))

		var bogusUserID pgtype.UUID
		require.NoError(t, bogusUserID.Scan("00000000-0000-4000-8000-0000deadbeef"))

		req := validPublicRequest("นาย ทดสอบ Rollback")
		resp, err := svc.CreatePublicSubmission(ctx, req, "slips/x/y.jpg", "image/jpeg", 1024, bogusUserID)
		require.Error(t, err, "a non-provisioned actor must fail the created_by FK and error out")
		assert.Nil(t, resp)

		var donationsAfter int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM donations`).Scan(&donationsAfter))
		assert.Equal(t, donationsBefore, donationsAfter, "no donation row may persist after a rolled-back submission")
	})
}
