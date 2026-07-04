// Package worker_test — WR-06 regression coverage (04-REVIEW.md): a
// transient object-storage failure fetching a non-critical branding image
// (letterhead/seal/signature/watermark) must not fail the entire receipt
// render/email pipeline — the legally-required content (donor name, amount,
// receipt number, section 6 text) does not depend on these decorative
// assets.
package worker_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/i18n"
	"github.com/donnarec/donnarec-api/internal/pdf"
	"github.com/donnarec/donnarec-api/internal/storage"
	"github.com/donnarec/donnarec-api/internal/testutil"
	"github.com/donnarec/donnarec-api/internal/worker"
)

// TestProcessOnce_ContinuesWhenBrandingImageFetchFails proves WR-06's fix: a
// watermark_object_key that points at an object which does NOT exist in the
// receipts bucket (simulating a transient MinIO blip on a non-critical asset)
// must not prevent the donation's receipt from being rendered, frozen, and
// emailed.
func TestProcessOnce_ContinuesWhenBrandingImageFetchFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	ctx := context.Background()
	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)

	donationRow := seedIssuedDonation(t, ctx, pool, queries, "donor-branding-fail@example.com")

	// Point the (single, shared) receipt_template_config's watermark slot at an
	// object key that was never uploaded — GetObject on it will error, exactly
	// like a transient object-storage blip.
	_, err := pool.Exec(ctx,
		`UPDATE receipt_template_config SET watermark_object_key = 'does-not-exist.png' WHERE id = true`,
	)
	require.NoError(t, err)

	endpoint, accessKey, secretKey := testutil.StartMinio(t, "test-receipts-branding-fail")
	store, err := storage.NewStorageClient(endpoint, accessKey, secretKey, "test-receipts-branding-fail", false)
	require.NoError(t, err)

	wsURL, cleanup := testutil.StartChrome(t)
	defer cleanup()
	renderer, err := pdf.NewRenderer(wsURL)
	require.NoError(t, err)

	bundle, err := i18n.SetupBundle("../i18n/locales")
	require.NoError(t, err)

	sender := &fakeSender{}

	w := worker.New(pool, queries, store, renderer, sender, bundle, zap.NewNop(), worker.Config{
		PollInterval:    time.Second,
		MaxAttempts:     5,
		StuckJobTimeout: 10 * time.Minute,
		ComputeBackoff:  func(attempts int32) time.Duration { return time.Millisecond },
	})

	require.NoError(t, w.ProcessOnce(ctx))

	updated, err := queries.GetDonationByID(ctx, donationRow.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.ReceiptPdfObjectKey,
		"the receipt must still be rendered and frozen even though the watermark image fetch failed")

	require.Equal(t, 1, sender.calls,
		"email must still be sent despite the non-critical branding-image fetch failure")

	var jobStatus string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status FROM outbox_jobs WHERE job_type = 'issue_receipt' ORDER BY created_at DESC LIMIT 1`,
	).Scan(&jobStatus))
	require.Equal(t, "done", jobStatus,
		"the job must complete successfully — a decorative-asset fetch failure must not burn a retry/backoff attempt")
}
