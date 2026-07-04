// Package worker_test — CR-01 regression coverage: a job stuck in
// status='processing' (worker killed/panicked between claim and
// mark-done/failed) must eventually become claimable again, rather than being
// lost forever (04-REVIEW.md CR-01).
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

// TestReclaimStuckJobs_ResetsProcessingPastTimeout proves CR-01's fix: a job
// stuck in 'processing' with updated_at older than cfg.StuckJobTimeout is
// reset back to 'pending' so ClaimNextOutboxJob can pick it up again —
// recovering from a worker crash/panic that left the row claimed but never
// finished.
func TestReclaimStuckJobs_ResetsProcessingPastTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	ctx := context.Background()
	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)

	staleUpdatedAt := time.Now().Add(-1 * time.Hour)
	var jobID int64
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO outbox_jobs (job_type, payload, status, updated_at)
		 VALUES ('issue_receipt', '{}', 'processing', $1) RETURNING id`,
		staleUpdatedAt,
	).Scan(&jobID))

	w := worker.New(pool, queries, nil, nil, nil, nil, zap.NewNop(), worker.Config{
		PollInterval:    time.Second,
		MaxAttempts:     5,
		StuckJobTimeout: 10 * time.Minute,
		ComputeBackoff:  func(attempts int32) time.Duration { return time.Millisecond },
	})

	require.NoError(t, w.ReclaimStuckJobs(ctx))

	var status string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status FROM outbox_jobs WHERE id = $1`, jobID,
	).Scan(&status))
	require.Equal(t, "pending", status, "a job stuck in 'processing' past the stuck-job timeout must be reset to 'pending'")

	// The reclaimed job must now actually be claimable (not just have the right
	// status label) — prove it via a real ClaimNextOutboxJob call.
	claimed, err := queries.ClaimNextOutboxJob(ctx, 5)
	require.NoError(t, err)
	require.Equal(t, jobID, claimed.ID)
}

// TestReclaimStuckJobs_LeavesRecentProcessingAlone proves the reclaim is
// timeout-gated: a job claimed moments ago (still legitimately in-flight,
// e.g. a slow-but-healthy render) must NOT be yanked back to 'pending' out
// from under the worker actually processing it.
func TestReclaimStuckJobs_LeavesRecentProcessingAlone(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	ctx := context.Background()
	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)

	var jobID int64
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO outbox_jobs (job_type, payload, status, updated_at)
		 VALUES ('issue_receipt', '{}', 'processing', now()) RETURNING id`,
	).Scan(&jobID))

	w := worker.New(pool, queries, nil, nil, nil, nil, zap.NewNop(), worker.Config{
		PollInterval:    time.Second,
		MaxAttempts:     5,
		StuckJobTimeout: 10 * time.Minute,
		ComputeBackoff:  func(attempts int32) time.Duration { return time.Millisecond },
	})

	require.NoError(t, w.ReclaimStuckJobs(ctx))

	var status string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status FROM outbox_jobs WHERE id = $1`, jobID,
	).Scan(&status))
	require.Equal(t, "processing", status, "a recently-claimed job still within the stuck-job timeout must be left alone")
}
