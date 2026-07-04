// Package worker_test — CR-02 regression coverage: a panic while processing
// ONE outbox job must never crash the entire donnarec-api process (which
// would take the HTTP API down with it) — Worker.ProcessOnceSafe recovers the
// panic, logs it, and the worker keeps processing subsequent jobs normally
// (04-REVIEW.md CR-02).
package worker_test

import (
	"context"
	"sync"
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

// panicOnceThenRealRenderer panics on its FIRST call (simulating a
// third-party CDP/template-execution edge case panicking mid-render), then
// delegates every subsequent call to a real *pdf.Renderer — so the SAME
// Worker instance can be proven to both survive the panic AND keep
// processing later jobs normally.
type panicOnceThenRealRenderer struct {
	mu       sync.Mutex
	panicked bool
	inner    worker.PDFRenderer
}

func (p *panicOnceThenRealRenderer) RenderPDF(ctx context.Context, html string) ([]byte, error) {
	p.mu.Lock()
	if !p.panicked {
		p.panicked = true
		p.mu.Unlock()
		panic("boom: simulated renderer panic (CR-02 regression test)")
	}
	p.mu.Unlock()
	return p.inner.RenderPDF(ctx, html)
}

// TestProcessOnceSafe_RecoversFromPanicAndWorkerKeepsRunning proves CR-02's
// fix: a panic inside job processing (here: the PDF renderer) is recovered by
// ProcessOnceSafe rather than propagating and crashing the process, and the
// SAME worker instance still processes a subsequent, healthy job correctly
// right afterwards.
func TestProcessOnceSafe_RecoversFromPanicAndWorkerKeepsRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	ctx := context.Background()
	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)

	endpoint, accessKey, secretKey := testutil.StartMinio(t, "test-receipts-panic")
	store, err := storage.NewStorageClient(endpoint, accessKey, secretKey, "test-receipts-panic", false)
	require.NoError(t, err)

	wsURL, cleanup := testutil.StartChrome(t)
	defer cleanup()
	realRenderer, err := pdf.NewRenderer(wsURL)
	require.NoError(t, err)
	renderer := &panicOnceThenRealRenderer{inner: realRenderer}

	bundle, err := i18n.SetupBundle("../i18n/locales")
	require.NoError(t, err)

	sender := &fakeSender{}

	w := worker.New(pool, queries, store, renderer, sender, bundle, zap.NewNop(), worker.Config{
		PollInterval:    time.Second,
		MaxAttempts:     5,
		StuckJobTimeout: 10 * time.Minute,
		ComputeBackoff:  func(attempts int32) time.Duration { return time.Millisecond },
	})

	donationA := seedIssuedDonation(t, ctx, pool, queries, "donor-panic-a@example.com")

	require.NotPanics(t, func() {
		_ = w.ProcessOnceSafe(ctx)
	}, "a panic while processing one job must be recovered by ProcessOnceSafe, never propagate")

	// The panicking job (donationA) may be left mid-processing — CR-01's
	// reclaim is what eventually recovers it, not this test's concern. What
	// CR-02 guarantees is that the WORKER (and therefore the whole process,
	// since Run shares this goroutine) is still alive and functional —
	// proven by processing a brand-new, healthy job right after.
	donationB := seedIssuedDonation(t, ctx, pool, queries, "donor-panic-b@example.com")

	err = w.ProcessOnceSafe(ctx)
	require.NoError(t, err, "the worker must keep processing subsequent jobs normally after recovering from a panic")

	updatedB, err := queries.GetDonationByID(ctx, donationB.ID)
	require.NoError(t, err)
	require.NotNil(t, updatedB.ReceiptPdfObjectKey, "a healthy job right after a panic-recovery must still render/freeze normally")
}
