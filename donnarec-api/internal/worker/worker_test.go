// Package worker_test — integration tests for the outbox worker (Phase 4,
// plan 04-05). All tests require Docker (testcontainers Postgres + a real
// chrome sidecar + MinIO) and are skipped in -short mode.
//
//	TestProcessJob_RenderFreezeEmailRecordAndIdempotency — NFR-07/FR-24/FR-25/
//	      FR-27/D-56: claim → render once → freeze to MinIO → email → record
//	      email_delivery → mark done; then a SECOND job for the same donation
//	      (simulating a 04-06 resend re-enqueue) must NOT re-invoke the
//	      renderer (D-56 freeze) and must still send + record a new delivery.
//	TestProcessJobLatency — NFR-07: one job (render+store+email) completes
//	      within the ~2-3s budget, measured off the issuance lock path.
//	TestEmailRetryBackoff — D-57/FR-27: a sender failing every attempt drives
//	      attempts to max_attempts, next_attempt_at advances per the backoff
//	      function, and the job becomes terminally 'failed' (dead-letter).
package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/donnarec/donnarec-api/internal/audit"
	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/crypto"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/donation"
	"github.com/donnarec/donnarec-api/internal/i18n"
	"github.com/donnarec/donnarec-api/internal/mailer"
	"github.com/donnarec/donnarec-api/internal/pdf"
	"github.com/donnarec/donnarec-api/internal/receiptno"
	"github.com/donnarec/donnarec-api/internal/storage"
	"github.com/donnarec/donnarec-api/internal/testutil"
	"github.com/donnarec/donnarec-api/internal/worker"
)

// testKEK is a 32-byte hex key for integration test use only (mirrors
// internal/donation/service_integration_test.go's integTestKEK convention).
const testKEK = "aa11bb22cc33dd44ee55ff66aa77bb88cc99dd00ee11ff22aa33bb44cc55dd66"

// --- test doubles ------------------------------------------------------

// fakeSender is a mailer.EmailSender that fails its first failCount calls
// (decrementing per call) then succeeds, recording every attempted message.
type fakeSender struct {
	mu           sync.Mutex
	failCount    int
	calls        int
	sentMessages []mailer.Message
}

func (f *fakeSender) Send(_ context.Context, msg mailer.Message) (mailer.SendResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.sentMessages = append(f.sentMessages, msg)
	if f.failCount > 0 {
		f.failCount--
		return mailer.SendResult{}, errors.New("fakeSender: induced send failure")
	}
	return mailer.SendResult{SentAt: time.Now(), ProviderMessageID: "fake-msg-id"}, nil
}

// countingRenderer wraps a real worker.PDFRenderer (backed by the live chrome
// sidecar) and counts invocations, so tests can prove the freeze-idempotency
// invariant (D-56: no re-render on an already-frozen receipt) without
// reimplementing the render pipeline — the FIRST render is still fully real.
type countingRenderer struct {
	mu    sync.Mutex
	inner worker.PDFRenderer
	calls int
}

func (c *countingRenderer) RenderPDF(ctx context.Context, html string) ([]byte, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.inner.RenderPDF(ctx, html)
}

// --- seeding helper ------------------------------------------------------

// seedIssuedDonation drives a donation through the real Create -> Submit ->
// Approve pipeline (internal/donation) so the Phase 3 issuance transaction
// enqueues a genuine "issue_receipt" outbox job (Approve Step 7) — the same
// row shape and payload the worker consumes in production. Returns the
// resulting donation row (post-issuance, carrying receipt_formatted,
// approved_at, donor_language, etc).
func seedIssuedDonation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, queries *db.Queries, donorEmail string) db.Donation {
	t.Helper()

	t.Setenv("DONAREC_KEK", testKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	alloc := receiptno.NewAllocator(queries)
	auditSvc := audit.NewAuditService(pool, queries, zap.NewNop())
	donationSvc := donation.NewDonationService(pool, queries, alloc, auditSvc, kp, zap.NewNop())

	makerSub := uuid.NewString()
	checkerSub := uuid.NewString()

	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:           fmt.Sprintf("maker-%s@example.com", makerSub),
		DisplayName:     "Worker Test Maker",
		KeycloakSubject: makerSub,
	})
	require.NoError(t, err)

	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:           fmt.Sprintf("checker-%s@example.com", checkerSub),
		DisplayName:     "Worker Test Checker",
		KeycloakSubject: checkerSub,
	})
	require.NoError(t, err)

	makerClaims := auth.KeycloakClaims{Subject: makerSub, RealmAccess: auth.RealmRoles{Roles: []string{"maker"}}}
	checkerClaims := auth.KeycloakClaims{Subject: checkerSub, RealmAccess: auth.RealmRoles{Roles: []string{"checker"}}}

	created, err := donationSvc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  "Worker Test Donor",
		DonorTaxID: "1234567890123",
		Amount:     1500.50,
		DonatedAt:  "2026-06-01",
		DonorEmail: donorEmail,
	}, makerRow.ID, makerClaims)
	require.NoError(t, err, "Create must succeed")

	_, err = donationSvc.Submit(ctx, created.ID, makerClaims)
	require.NoError(t, err, "Submit must succeed")

	approved, err := donationSvc.Approve(ctx, created.ID, checkerRow.ID, checkerClaims)
	require.NoError(t, err, "Approve must succeed — this is what enqueues the issue_receipt outbox job")
	require.Equal(t, "issued", approved.Status)

	var pgID pgtype.UUID
	require.NoError(t, pgID.Scan(created.ID))

	row, err := queries.GetDonationByID(ctx, pgID)
	require.NoError(t, err)
	return row
}

// --- tests ---------------------------------------------------------------

// TestProcessJob_RenderFreezeEmailRecordAndIdempotency covers the plan's
// primary integration scenario AND the freeze-idempotency regression in one
// test function (sharing one Postgres/chrome/MinIO container set, mirroring
// how 04-03's golden tests each spin their own infra per Test function while
// avoiding a second full container spin-up for a closely related assertion).
func TestProcessJob_RenderFreezeEmailRecordAndIdempotency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	ctx := context.Background()
	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)

	donationRow := seedIssuedDonation(t, ctx, pool, queries, "donor@example.com")

	endpoint, accessKey, secretKey := testutil.StartMinio(t, "test-receipts")
	store, err := storage.NewStorageClient(endpoint, accessKey, secretKey, "test-receipts", false)
	require.NoError(t, err)

	wsURL, cleanup := testutil.StartChrome(t)
	defer cleanup()
	realRenderer, err := pdf.NewRenderer(wsURL)
	require.NoError(t, err)
	renderer := &countingRenderer{inner: realRenderer}

	bundle, err := i18n.SetupBundle("../i18n/locales")
	require.NoError(t, err)

	sender := &fakeSender{}

	w := worker.New(pool, queries, store, renderer, sender, bundle, zap.NewNop(), worker.Config{
		PollInterval:   time.Second,
		MaxAttempts:    5,
		ComputeBackoff: func(attempts int32) time.Duration { return time.Millisecond },
	})

	// --- First job: the auto-enqueued issue_receipt job from Approve Step 7 ---
	err = w.ProcessOnce(ctx)
	require.NoError(t, err)

	updated, err := queries.GetDonationByID(ctx, donationRow.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.ReceiptPdfObjectKey, "receipt_pdf_object_key must be set after the first render (D-56 freeze)")

	pdfBytes, err := store.GetObject(ctx, *updated.ReceiptPdfObjectKey)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(pdfBytes), 4)
	require.Equal(t, "%PDF", string(pdfBytes[:4]), "stored object must be a real rendered PDF")

	delivery, err := queries.GetLatestEmailDeliveryForDonation(ctx, donationRow.ID)
	require.NoError(t, err)
	require.Equal(t, "sent", delivery.Status)
	require.Equal(t, 1, sender.calls)
	require.Equal(t, 1, renderer.calls, "renderer must be invoked exactly once for the first (unfrozen) render")

	var jobStatus string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status FROM outbox_jobs WHERE job_type = 'issue_receipt' ORDER BY created_at DESC LIMIT 1`,
	).Scan(&jobStatus))
	require.Equal(t, "done", jobStatus)

	// --- Second job for the SAME donation (simulates a 04-06 resend re-enqueue) ---
	payload, err := json.Marshal(map[string]string{"donation_id": donationRow.ID.String()})
	require.NoError(t, err)
	require.NoError(t, queries.EnqueueOutboxJob(ctx, db.EnqueueOutboxJobParams{
		JobType: "issue_receipt",
		Payload: payload,
	}))

	err = w.ProcessOnce(ctx)
	require.NoError(t, err)

	require.Equal(t, 1, renderer.calls, "renderer must NOT be invoked again — the frozen PDF must be reused (D-56)")
	require.Equal(t, 2, sender.calls, "the second job must still trigger a real send attempt")

	var deliveryCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM email_delivery WHERE donation_id = $1`, donationRow.ID,
	).Scan(&deliveryCount))
	require.Equal(t, 2, deliveryCount, "a second send attempt must record a NEW email_delivery row, never overwrite the first")
}

// TestProcessJobLatency measures one full job (render+store+email, against
// the real chrome sidecar and the real dev/local EmailSender — no fakes) and
// asserts it completes within the ~2-3s NFR-07 budget, off the issuance lock
// path (the issuance transaction itself already committed before this ever
// runs).
func TestProcessJobLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	ctx := context.Background()
	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)

	seedIssuedDonation(t, ctx, pool, queries, "donor-latency@example.com")

	endpoint, accessKey, secretKey := testutil.StartMinio(t, "test-receipts-latency")
	store, err := storage.NewStorageClient(endpoint, accessKey, secretKey, "test-receipts-latency", false)
	require.NoError(t, err)

	wsURL, cleanup := testutil.StartChrome(t)
	defer cleanup()
	renderer, err := pdf.NewRenderer(wsURL)
	require.NoError(t, err)

	bundle, err := i18n.SetupBundle("../i18n/locales")
	require.NoError(t, err)

	sender := &mailer.DevSender{OutDir: t.TempDir()}

	w := worker.New(pool, queries, store, renderer, sender, bundle, zap.NewNop(), worker.Config{
		PollInterval:   time.Second,
		MaxAttempts:    5,
		ComputeBackoff: func(attempts int32) time.Duration { return time.Millisecond },
	})

	start := time.Now()
	err = w.ProcessOnce(ctx)
	elapsed := time.Since(start)
	require.NoError(t, err)

	require.Less(t, elapsed, 3*time.Second,
		"render+store+email for one job must complete within the ~2-3s NFR-07 budget; took %s", elapsed)
}

// TestEmailRetryBackoff drives a sender that fails every attempt through
// exactly max_attempts ProcessOnce calls, asserting attempts increments by 1
// per attempt, next_attempt_at advances per the injected backoff function,
// each failed attempt records its own 'failed' email_delivery row, and the
// job becomes terminally 'failed' (dead-letter, D-57) once attempts reaches
// max_attempts — after which a further poll finds nothing claimable.
func TestEmailRetryBackoff(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	ctx := context.Background()
	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)

	donationRow := seedIssuedDonation(t, ctx, pool, queries, "donor-retry@example.com")

	endpoint, accessKey, secretKey := testutil.StartMinio(t, "test-receipts-retry")
	store, err := storage.NewStorageClient(endpoint, accessKey, secretKey, "test-receipts-retry", false)
	require.NoError(t, err)

	wsURL, cleanup := testutil.StartChrome(t)
	defer cleanup()
	renderer, err := pdf.NewRenderer(wsURL)
	require.NoError(t, err)

	bundle, err := i18n.SetupBundle("../i18n/locales")
	require.NoError(t, err)

	const maxAttempts = int32(3)
	sender := &fakeSender{failCount: int(maxAttempts)} // fails every attempt through dead-letter

	var lastNextAttemptAt time.Time
	w := worker.New(pool, queries, store, renderer, sender, bundle, zap.NewNop(), worker.Config{
		PollInterval: time.Second,
		MaxAttempts:  maxAttempts,
		ComputeBackoff: func(attempts int32) time.Duration {
			// Distinguishable-but-tiny per-attempt delay: proves next_attempt_at
			// advances per the schedule shape without slowing down the test.
			return time.Duration(attempts+1) * time.Millisecond
		},
	})

	var lastAttempts int32
	var lastStatus string
	for i := int32(0); i < maxAttempts; i++ {
		callStart := time.Now()
		err := w.ProcessOnce(ctx)
		require.NoError(t, err)

		var nextAttemptAt time.Time
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT status, attempts, next_attempt_at FROM outbox_jobs WHERE job_type = 'issue_receipt' ORDER BY created_at DESC LIMIT 1`,
		).Scan(&lastStatus, &lastAttempts, &nextAttemptAt))

		require.Equal(t, i+1, lastAttempts, "attempts must increment by exactly 1 per failed processing attempt")
		require.True(t, nextAttemptAt.After(callStart) || nextAttemptAt.Equal(callStart),
			"next_attempt_at must be pushed to (now + backoff), not left in the past")
		if !lastNextAttemptAt.IsZero() {
			require.True(t, nextAttemptAt.After(lastNextAttemptAt) || i == maxAttempts-1,
				"next_attempt_at must advance across retries per the backoff schedule")
		}
		lastNextAttemptAt = nextAttemptAt

		var deliveryStatus string
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT status FROM email_delivery WHERE donation_id = $1 ORDER BY created_at DESC LIMIT 1`, donationRow.ID,
		).Scan(&deliveryStatus))
		require.Equal(t, "failed", deliveryStatus)

		// Wait out the tiny injected backoff so the NEXT ProcessOnce call's
		// ClaimNextOutboxJob (next_attempt_at <= now()) can actually reclaim it.
		time.Sleep(10 * time.Millisecond)
	}

	require.Equal(t, "failed", lastStatus, "job must be terminally 'failed' (dead-letter) once attempts reaches max_attempts (D-57)")

	// A further poll must find nothing claimable — the dead-lettered job is excluded.
	err = w.ProcessOnce(ctx)
	require.ErrorIs(t, err, worker.ErrNoJob)

	var deliveryCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM email_delivery WHERE donation_id = $1`, donationRow.ID,
	).Scan(&deliveryCount))
	require.Equal(t, int(maxAttempts), deliveryCount, "exactly one email_delivery row must be recorded per failed send attempt")
}
