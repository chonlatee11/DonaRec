// Package worker_test — WR-05 regression coverage (04-REVIEW.md): a job that
// has already been dead-lettered (status='failed', terminal per
// MarkOutboxJobFailed's own CASE logic) must NEVER be resurrected by a later
// operator raising WORKER_MAX_ATTEMPTS — 'failed' is meant to be a true
// terminal state, with staff-triggered resend (which enqueues a brand-new
// row) as the only way to retry it.
package worker_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/i18n"
	"github.com/donnarec/donnarec-api/internal/testutil"
	"github.com/donnarec/donnarec-api/internal/worker"
)

// TestDeadLetteredJob_NeverResurrectedByRaisingMaxAttempts proves WR-05's
// fix: once a job is terminally 'failed' (dead-lettered) under a SMALL
// MaxAttempts, a later worker instance configured with a LARGER MaxAttempts
// (simulating an operator raising WORKER_MAX_ATTEMPTS after the fact) must
// NOT be able to claim and reprocess it — ClaimNextOutboxJob's claimable set
// no longer includes 'failed' rows at all.
func TestDeadLetteredJob_NeverResurrectedByRaisingMaxAttempts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	ctx := context.Background()
	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)

	donationRow := seedIssuedDonation(t, ctx, pool, queries, "donor-deadletter@example.com")

	bundle, err := i18n.SetupBundle("../i18n/locales")
	require.NoError(t, err)

	// Freeze a placeholder receipt_pdf_object_key directly (bypassing a live
	// chrome render — D-56's freeze check means getOrRenderReceiptPDF will read
	// this key back, never invoke the renderer) so the job deterministically
	// reaches the email-send step, which the fakeSender below fails every time.
	placeholderKey := "receipts/" + donationRow.ID.String() + ".pdf"
	require.NoError(t, queries.SetReceiptPDFObjectKey(ctx, db.SetReceiptPDFObjectKeyParams{
		ReceiptPdfObjectKey: &placeholderKey,
		ID:                  donationRow.ID,
	}))

	const smallMaxAttempts = int32(2)
	sender := &fakeSender{failCount: int(smallMaxAttempts)}
	smallWorker := worker.New(pool, queries, &alwaysReturningEmptyStore{}, nil, sender, bundle, zap.NewNop(), worker.Config{
		PollInterval:    time.Second,
		MaxAttempts:     smallMaxAttempts,
		StuckJobTimeout: 10 * time.Minute,
		ComputeBackoff:  func(attempts int32) time.Duration { return time.Millisecond },
	})

	for i := int32(0); i < smallMaxAttempts; i++ {
		require.NoError(t, smallWorker.ProcessOnce(ctx))
		time.Sleep(10 * time.Millisecond) // wait out the tiny injected backoff
	}

	var status string
	var attempts int32
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status, attempts FROM outbox_jobs WHERE job_type = 'issue_receipt' ORDER BY created_at DESC LIMIT 1`,
	).Scan(&status, &attempts))
	require.Equal(t, "failed", status, "the job must be terminally dead-lettered after exhausting smallMaxAttempts")
	require.Equal(t, smallMaxAttempts, attempts)

	// Simulate an operator raising WORKER_MAX_ATTEMPTS well above the dead-lettered
	// job's attempts count — a brand-new worker instance with a MUCH larger
	// MaxAttempts must still find nothing claimable.
	largeMaxAttemptsWorker := worker.New(pool, queries, &alwaysReturningEmptyStore{}, nil, sender, bundle, zap.NewNop(), worker.Config{
		PollInterval:    time.Second,
		MaxAttempts:     50,
		StuckJobTimeout: 10 * time.Minute,
		ComputeBackoff:  func(attempts int32) time.Duration { return time.Millisecond },
	})

	err = largeMaxAttemptsWorker.ProcessOnce(ctx)
	require.ErrorIs(t, err, worker.ErrNoJob,
		"a dead-lettered ('failed') job must never be resurrected just because MaxAttempts was later raised (WR-05)")
}

// alwaysReturningEmptyStore is a ReceiptsStore whose GetObject returns the
// frozen placeholder bytes for any key (the freeze check only cares that
// GetObject succeeds) — this test never needs a real MinIO/PDF, since the
// receipt is already "frozen" (D-56) before the worker ever claims the job.
type alwaysReturningEmptyStore struct{}

func (alwaysReturningEmptyStore) GetObject(ctx context.Context, objectKey string) ([]byte, error) {
	return []byte("%PDF-fake-frozen-bytes"), nil
}

func (alwaysReturningEmptyStore) PutObject(ctx context.Context, objectKey string, data []byte, contentType string) error {
	return nil
}
