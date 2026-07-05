// Package worker_test — BW-01 (04-REVIEW-PRESHIP.md) regression coverage: the
// panic+reclaim path must be BOUNDED. A job that panics on every tick is left
// 'processing' by ProcessOnceSafe (no attempts increment) and then reset by
// ReclaimStuckOutboxJobs; if reclaim also fails to increment attempts, that job
// is retried forever and NEVER dead-letters — contradicting the bounded-retry
// design. Reclaim must increment attempts and apply the same terminal CASE as
// MarkOutboxJobFailed, so a deterministically-panicking job reaches 'failed'
// after MaxAttempts reclaims.
package worker_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/testutil"
	"github.com/donnarec/donnarec-api/internal/worker"
)

// TestReclaimStuckJobs_DeadLettersPerpetuallyPanickingJob proves BW-01's fix: a
// job repeatedly left stuck in 'processing' (as a perpetually-panicking job
// would be — ProcessOnceSafe recovers the panic without incrementing attempts)
// must have its attempts incremented on each reclaim and transition to the
// terminal 'failed' state once attempts reaches MaxAttempts, rather than looping
// forever.
func TestReclaimStuckJobs_DeadLettersPerpetuallyPanickingJob(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	ctx := context.Background()
	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)

	const maxAttempts = int32(3)
	w := worker.New(pool, queries, nil, nil, nil, nil, zap.NewNop(), worker.Config{
		PollInterval:    time.Second,
		MaxAttempts:     maxAttempts,
		StuckJobTimeout: 10 * time.Minute,
		ComputeBackoff:  func(attempts int32) time.Duration { return time.Millisecond },
	})

	staleUpdatedAt := time.Now().Add(-1 * time.Hour)
	var jobID int64
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO outbox_jobs (job_type, payload, status, attempts, updated_at)
		 VALUES ('issue_receipt', '{}', 'processing', 0, $1) RETURNING id`,
		staleUpdatedAt,
	).Scan(&jobID))

	// Simulate maxAttempts claim→panic→reclaim cycles: before each reclaim,
	// force the job back into a stale 'processing' state (what a worker that
	// claimed the job and then panicked leaves behind).
	for i := int32(1); i <= maxAttempts; i++ {
		_, err := pool.Exec(ctx,
			`UPDATE outbox_jobs SET status = 'processing', updated_at = $1 WHERE id = $2`,
			staleUpdatedAt, jobID,
		)
		require.NoError(t, err)

		require.NoError(t, w.ReclaimStuckJobs(ctx))

		var status string
		var attempts int32
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT status, attempts FROM outbox_jobs WHERE id = $1`, jobID,
		).Scan(&status, &attempts))
		require.Equal(t, i, attempts, "each reclaim of a stuck job must increment attempts")

		if i < maxAttempts {
			require.Equal(t, "pending", status, "a stuck job with attempts remaining must be re-armed as pending")
		} else {
			require.Equal(t, "failed", status, "a stuck job that reaches MaxAttempts reclaims must dead-letter to failed (BW-01)")
		}
	}

	// The dead-lettered job must not be resurrected even by a raised MaxAttempts,
	// consistent with the WR-05 invariant — ClaimNextOutboxJob never claims 'failed'.
	_, err := queries.ClaimNextOutboxJob(ctx, 50)
	require.Error(t, err, "a dead-lettered job must not be claimable")
}
