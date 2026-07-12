// Package worker_test — integration tests for the ack_email outbox job handler
// (Phase 6, plan 06-04). Like the other worker integration tests these require
// Docker (a testcontainers Postgres) and are skipped in -short mode. Unlike the
// issue_receipt tests they need NEITHER a chrome sidecar NOR MinIO: the ack
// email carries no PDF attachment and touches no object storage, so the Worker
// is constructed with nil receiptsStore/renderer to prove the ack path never
// reaches into them.
//
//	TestAckEmail covers the plan 06-04 behavior contract:
//	  - a th-language flow_b donation -> one bilingual email to the donor, subject
//	    + body resolved from the ackEmail.* i18n keys, the explicit
//	    "not yet a receipt" statement present, and the REF- reference number in
//	    the body (FR-05/FR-06, D-84/D-85, T-06-15)
//	  - donor_language 'en' resolves the English catalog (FR-06)
//	  - a donation with no donor email is a terminal success (job -> done), NOT a
//	    fatal error or a retry loop (mirrors issue_receipt's no-email handling)
//	  - an unknown job_type still hits ProcessOnce's default error arm (regression
//	    guard on the switch) and sends no email
package worker_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/donation"
	"github.com/donnarec/donnarec-api/internal/i18n"
	"github.com/donnarec/donnarec-api/internal/testutil"
	"github.com/donnarec/donnarec-api/internal/worker"
)

// seedPendingFlowBDonation inserts a minimal pending_review flow_b donation
// directly via the sqlc queries (CreateDonation draft -> SubmitDonation), under
// the seeded public-web system user (migration 000016). It deliberately does
// NOT go through the full CreatePublicSubmission path (slip upload + envelope
// crypto) — the ack handler only reads donor_email / donor_language / id, so a
// minimal row is sufficient and keeps the test free of MinIO/crypto. donorEmail
// == "" seeds a NULL donor_email (the no-email terminal case). Returns the
// donation id string (the same string plan 03 puts in the outbox payload).
func seedPendingFlowBDonation(t *testing.T, ctx context.Context, queries *db.Queries, donorEmail, donorLanguage string) string {
	t.Helper()

	var createdBy pgtype.UUID
	require.NoError(t, createdBy.Scan(donation.PublicWebUserID))

	var amount pgtype.Numeric
	require.NoError(t, amount.Scan("1500.50"))

	donatedAt := pgtype.Date{Time: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), Valid: true}

	var emailPtr *string
	if strings.TrimSpace(donorEmail) != "" {
		emailPtr = &donorEmail
	}

	row, err := queries.CreateDonation(ctx, db.CreateDonationParams{
		CreatedBy:     createdBy,
		DonorName:     "Ack Test Donor",
		DonorAddress:  "123 Test Rd",
		DonorEmail:    emailPtr,
		DonorTaxIDEnc: []byte("ciphertext-placeholder"),
		DonorTaxIDDek: []byte("dek-placeholder"),
		Amount:        amount,
		DonatedAt:     donatedAt,
		ConsentGiven:  true,
		LegalBasis:    "consent",
		DonorLanguage: donorLanguage,
		Source:        "flow_b",
	})
	require.NoError(t, err, "CreateDonation (flow_b draft) must succeed")

	require.NoError(t, queries.SubmitDonation(ctx, row.ID), "SubmitDonation (draft -> pending_review) must succeed")

	return uuid.UUID(row.ID.Bytes).String()
}

// enqueueAckJob enqueues one ack_email outbox job carrying {"donation_id": id},
// the exact payload shape plan 03's CreatePublicSubmission enqueues in-tx.
func enqueueAckJob(t *testing.T, ctx context.Context, queries *db.Queries, donationID string) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"donation_id": donationID})
	require.NoError(t, err)
	require.NoError(t, queries.EnqueueOutboxJob(ctx, db.EnqueueOutboxJobParams{
		JobType: "ack_email",
		Payload: payload,
	}))
}

// newAckWorker builds a Worker wired for the ack path only: real Postgres +
// i18n bundle + a fresh capturing fakeSender, but nil receiptsStore/renderer
// (the ack path must never touch them) and an hour-long backoff so a
// deliberately-failed job (the unknown-job_type case) is not reclaimed mid-test.
func newAckWorker(t *testing.T, pool *pgxpool.Pool, queries *db.Queries, sender *fakeSender) *worker.Worker {
	t.Helper()
	bundle, err := i18n.SetupBundle("../i18n/locales")
	require.NoError(t, err)
	return worker.New(pool, queries, nil, nil, sender, bundle, zap.NewNop(), worker.Config{
		PollInterval:    time.Second,
		MaxAttempts:     5,
		ComputeBackoff:  func(int32) time.Duration { return time.Hour },
		StuckJobTimeout: time.Hour,
	})
}

func TestAckEmail(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	ctx := context.Background()
	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)

	// Subtests run sequentially and each enqueues exactly one job; successful
	// jobs end 'done' and the failed job backs off an hour, so ClaimNextOutboxJob
	// always claims precisely the job the current subtest enqueued.

	t.Run("Thai_BilingualNotAReceiptWithReference", func(t *testing.T) {
		sender := &fakeSender{}
		w := newAckWorker(t, pool, queries, sender)

		id := seedPendingFlowBDonation(t, ctx, queries, "donor-th@example.com", "th")
		enqueueAckJob(t, ctx, queries, id)

		require.NoError(t, w.ProcessOnce(ctx))

		require.Len(t, sender.sentMessages, 1, "exactly one ack email must be sent")
		msg := sender.sentMessages[0]
		require.Equal(t, "donor-th@example.com", msg.To)
		require.Contains(t, msg.Subject, "ยังไม่ใช่ใบเสร็จ", "TH subject must carry the explicit not-a-receipt statement (D-84/FR-05)")

		body := msg.BodyHTML + "\n" + msg.BodyText
		require.Contains(t, body, "ยังไม่ใช่ใบเสร็จรับเงิน", "TH body must state this is not yet a receipt")

		refCode := donation.PublicReferenceNumber(id)
		require.Contains(t, body, refCode, "the REF- reference number must appear in the ack email body")
		require.True(t, strings.HasPrefix(refCode, "REF-"))

		// The ack email must never carry a receipt number (none exists pre-approval, T-06-15).
		require.NotContains(t, body, "ใบเสร็จเลขที่")
	})

	t.Run("English_ResolvesEnglishCatalog", func(t *testing.T) {
		sender := &fakeSender{}
		w := newAckWorker(t, pool, queries, sender)

		id := seedPendingFlowBDonation(t, ctx, queries, "donor-en@example.com", "en")
		enqueueAckJob(t, ctx, queries, id)

		require.NoError(t, w.ProcessOnce(ctx))

		require.Len(t, sender.sentMessages, 1)
		msg := sender.sentMessages[0]
		require.Equal(t, "donor-en@example.com", msg.To)

		body := msg.BodyHTML + "\n" + msg.BodyText
		lowerSubject := strings.ToLower(msg.Subject)
		lowerBody := strings.ToLower(body)
		require.Contains(t, lowerSubject, "not yet a receipt", "EN subject must state this is not yet a receipt (FR-06)")
		require.Contains(t, lowerBody, "not a receipt", "EN body must carry the explicit not-a-receipt statement")
		require.Contains(t, body, donation.PublicReferenceNumber(id), "the REF- reference number must appear in the EN ack body")
	})

	t.Run("NoDonorEmail_TerminalSuccessNotRetryLoop", func(t *testing.T) {
		sender := &fakeSender{}
		w := newAckWorker(t, pool, queries, sender)

		id := seedPendingFlowBDonation(t, ctx, queries, "", "th") // NULL donor_email
		enqueueAckJob(t, ctx, queries, id)

		require.NoError(t, w.ProcessOnce(ctx), "a no-email donation must not fatally error the tick")
		require.Empty(t, sender.sentMessages, "no email must be sent when the donor has no email on file")

		var status string
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT status FROM outbox_jobs WHERE job_type = 'ack_email' ORDER BY created_at DESC LIMIT 1`,
		).Scan(&status))
		require.Equal(t, "done", status, "no-email is an expected terminal state (job done), not a retry loop")
	})

	t.Run("UnknownJobType_HitsDefaultErrorArm", func(t *testing.T) {
		sender := &fakeSender{}
		w := newAckWorker(t, pool, queries, sender)

		payload, err := json.Marshal(map[string]string{"donation_id": "irrelevant"})
		require.NoError(t, err)
		require.NoError(t, queries.EnqueueOutboxJob(ctx, db.EnqueueOutboxJobParams{
			JobType: "totally_unknown_job_type",
			Payload: payload,
		}))

		require.NoError(t, w.ProcessOnce(ctx), "an unknown job_type is a job-level failure, not an infra error")
		require.Empty(t, sender.sentMessages, "the default arm must send no email")

		var lastError *string
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT last_error FROM outbox_jobs WHERE job_type = 'totally_unknown_job_type' ORDER BY created_at DESC LIMIT 1`,
		).Scan(&lastError))
		require.NotNil(t, lastError)
		require.Contains(t, *lastError, "unknown job_type", "the default arm's error must be recorded on the job")
	})
}
