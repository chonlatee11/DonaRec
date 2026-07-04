// Package worker implements the Phase 4 outbox worker: a background goroutine
// that polls outbox_jobs (enqueued atomically inside the Phase 3 issuance
// transaction, internal/donation/service.go Approve Step 7) and processes each
// job entirely OFF the receipt-numbering lock path (NFR-07).
//
// Design decisions realized here:
//
//	Atomic claim (T-04-11): ProcessOnce claims exactly one job via
//	      db.ClaimNextOutboxJob, a single UPDATE...WHERE id=(SELECT...FOR UPDATE
//	      SKIP LOCKED) — no separate SELECT-then-UPDATE race window across
//	      multiple worker goroutines/instances (04-RESEARCH Pattern 1).
//	Bounded retry + dead-letter (T-04-12, D-57): a job that fails is re-armed
//	      with next_attempt_at pushed out per Config.ComputeBackoff, until
//	      attempts reaches Config.MaxAttempts, at which point
//	      db.MarkOutboxJobFailed transitions it to the terminal 'failed' state
//	      (dead-letter — the worker stops auto-retrying; staff see the failure
//	      and can resend manually, FR-27/28, plan 04-06).
//	No re-render on a frozen receipt (D-56): enforced in issue_receipt.go, not
//	      here — processOnce/dispatch never inspects receipt_pdf_object_key
//	      itself, but the job_type="issue_receipt" handler always checks it
//	      first (T-04-14).
//	No email/number allocation inside the issuance tx: already true by
//	      construction — this package only ever runs against jobs that were
//	      already committed by Phase 3's Approve transaction; it never imports
//	      internal/receiptno.
//	Stuck-job reclaim (CR-01, 04-REVIEW.md): ReclaimStuckJobs resets a job
//	      that has sat in 'processing' longer than Config.StuckJobTimeout back
//	      to 'pending' — recovers a job orphaned by a worker process that was
//	      killed/panicked/OOM-killed between claim and completion, which
//	      otherwise had NO path back to claimable (dead forever). Called once
//	      per Run tick, before ProcessOnce.
//	Panic recovery (CR-02, 04-REVIEW.md): Run calls ProcessOnce via a
//	      recover()-guarded wrapper so a panic while processing one job (e.g.
//	      a third-party CDP/template edge case) is logged and swallowed rather
//	      than crashing the entire donnarec-api process (including the HTTP
//	      API, which has its own independent gin.Recovery()).
//
// Anti-patterns explicitly absent (mirrors internal/receiptno/allocator.go's
// doc-comment convention):
//   - NO separate SELECT-then-UPDATE claim (would race across workers)
//   - NO synchronous render/email call inside any DB transaction
//   - NO unbounded auto-retry (Config.MaxAttempts always terminates retries)
//   - NO unrecovered panic path from Run's ticker loop (CR-02)
package worker

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/mailer"
	gogoi18n "github.com/nicksnyder/go-i18n/v2/i18n"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// ErrNoJob is returned by ProcessOnce when no eligible outbox job was
// available to claim this tick — the caller (Run's poll loop, or a test)
// treats this as "nothing to do", not an error condition.
var ErrNoJob = errors.New("worker: no eligible job to claim")

// ReceiptsStore is the minimal object-storage seam issue_receipt needs —
// satisfied by *storage.StorageClient (PutObject/GetObject) in production.
// Defining a narrow interface here (rather than depending on the concrete
// storage type) keeps this package testable without a live MinIO in every test.
type ReceiptsStore interface {
	PutObject(ctx context.Context, objectKey string, data []byte, contentType string) error
	GetObject(ctx context.Context, objectKey string) ([]byte, error)
}

// PDFRenderer is the minimal PDF-rendering seam issue_receipt needs —
// satisfied by *pdf.Renderer (internal/pdf/chromium.go) in production. A
// narrow interface (rather than depending on *pdf.Renderer directly) lets
// tests wrap the real renderer in a call-counting decorator to prove the
// freeze-idempotency invariant (D-56: no re-render on a resend) without
// reimplementing the render pipeline.
type PDFRenderer interface {
	RenderPDF(ctx context.Context, selfContainedHTML string) ([]byte, error)
}

// Config holds the outbox worker's poll/retry knobs. Defined in this package
// (rather than depending on internal/config directly) so tests can inject a
// near-zero ComputeBackoff without needing to mutate config's package-level
// backoff schedule.
type Config struct {
	// PollInterval is how often Run's ticker checks outbox_jobs for claimable work.
	PollInterval time.Duration
	// MaxAttempts is the number of send attempts before a job becomes
	// terminally 'failed' (dead-letter, D-57) — passed straight through to
	// ClaimNextOutboxJob's exclusion filter and MarkOutboxJobFailed's
	// terminal-transition guard.
	MaxAttempts int32
	// ComputeBackoff returns the delay before the next retry, given the
	// attempts count already recorded on the job (pre-increment — see
	// config.WorkerConfig.ComputeBackoff's doc comment, 04-01-SUMMARY).
	ComputeBackoff func(attempts int32) time.Duration
	// StuckJobTimeout is how long a job may sit in 'processing' before
	// ReclaimStuckJobs resets it back to 'pending' (CR-01, 04-REVIEW.md) —
	// recovers from a worker process killed/panicked/OOM-killed between
	// claiming a job and marking it done/failed. Must be well above the
	// ~2-3s NFR-07 render+email budget so a healthy in-flight job is never
	// reclaimed out from under the worker actually processing it.
	StuckJobTimeout time.Duration
}

// Worker polls outbox_jobs and dispatches each claimed job by job_type.
// Use New to construct.
type Worker struct {
	pool          *pgxpool.Pool
	queries       *db.Queries
	receiptsStore ReceiptsStore
	renderer      PDFRenderer
	sender        mailer.EmailSender
	bundle        *gogoi18n.Bundle
	logger        *zap.Logger
	cfg           Config
}

// New constructs a Worker. pool is currently unused by any query path (all
// worker queries run directly against queries, which is bound to the shared
// pgxpool.Pool) but is accepted for parity with other service constructors
// in this codebase (e.g. donation.NewDonationService) and to leave room for
// a future multi-statement worker transaction without a constructor signature
// change.
func New(pool *pgxpool.Pool, queries *db.Queries, receiptsStore ReceiptsStore, renderer PDFRenderer, sender mailer.EmailSender, bundle *gogoi18n.Bundle, logger *zap.Logger, cfg Config) *Worker {
	return &Worker{
		pool:          pool,
		queries:       queries,
		receiptsStore: receiptsStore,
		renderer:      renderer,
		sender:        sender,
		bundle:        bundle,
		logger:        logger,
		cfg:           cfg,
	}
}

// Run polls outbox_jobs every cfg.PollInterval until ctx is cancelled (the
// shared signal.NotifyContext from cmd/server/main.go — same graceful
// shutdown pattern as the HTTP server goroutine). Errors from a single
// ProcessOnce call are logged (Pattern C: operation + job id only, never
// donor PII) and never terminate the loop — a single bad job must not stop
// the entire worker from processing subsequent jobs.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("worker: shutting down")
			return
		case <-ticker.C:
			if err := w.ReclaimStuckJobs(ctx); err != nil {
				w.logger.Error("worker: reclaim stuck jobs failed", zap.String("operation", "ReclaimStuckJobs"), zap.Error(err))
			}
			if err := w.ProcessOnceSafe(ctx); err != nil && !errors.Is(err, ErrNoJob) {
				w.logger.Error("worker: process tick failed", zap.String("operation", "ProcessOnce"), zap.Error(err))
			}
		}
	}
}

// ReclaimStuckJobs resets outbox jobs that have been stuck in 'processing'
// for longer than cfg.StuckJobTimeout back to 'pending' (CR-01, 04-REVIEW.md)
// — recovers a job left claimed-but-unfinished by a worker process that was
// killed, panicked, OOM-killed, or evicted before it could call
// MarkOutboxJobDone/MarkOutboxJobFailed. Called once per Run tick, before
// ProcessOnce, so a reclaimed job becomes claimable again on the very same
// tick's ClaimNextOutboxJob call. Safe to call with zero eligible rows (a
// no-op, not an error).
func (w *Worker) ReclaimStuckJobs(ctx context.Context) error {
	cutoff := time.Now().Add(-w.cfg.StuckJobTimeout)
	n, err := w.queries.ReclaimStuckOutboxJobs(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true})
	if err != nil {
		return fmt.Errorf("worker: reclaim stuck jobs: %w", err)
	}
	if n > 0 {
		w.logger.Warn("worker: reclaimed stuck outbox jobs", zap.Int64("count", n))
	}
	return nil
}

// ProcessOnce claims exactly one due outbox job (ClaimNextOutboxJob — atomic
// FOR UPDATE SKIP LOCKED, race-free across worker instances) and dispatches
// it by job_type. Returns ErrNoJob when there was nothing eligible to claim
// this call — callers (Run, or a test driving the worker synchronously)
// should treat that as "nothing to do", not a failure.
//
// A job-level processing failure (render/store/email error) is handled
// internally: it is recorded via MarkOutboxJobFailed (bounded retry +
// backoff, D-57) and ProcessOnce still returns nil — only an
// infrastructure-level error (claim failed for a reason other than "no
// rows", or marking done/failed itself failed) is returned to the caller.
func (w *Worker) ProcessOnce(ctx context.Context) error {
	job, err := w.queries.ClaimNextOutboxJob(ctx, w.cfg.MaxAttempts)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoJob
		}
		return fmt.Errorf("worker: claim next outbox job: %w", err)
	}

	var procErr error
	switch job.JobType {
	case "issue_receipt":
		procErr = w.handleIssueReceipt(ctx, job)
	default:
		procErr = fmt.Errorf("worker: unknown job_type %q", job.JobType)
	}

	if procErr != nil {
		w.logger.Error("worker: job processing failed",
			zap.String("operation", job.JobType),
			zap.Int64("job_id", job.ID),
			zap.Error(procErr),
		)

		errMsg := procErr.Error()
		nextAttempt := time.Now().Add(w.cfg.ComputeBackoff(job.Attempts))
		if markErr := w.queries.MarkOutboxJobFailed(ctx, db.MarkOutboxJobFailedParams{
			MaxAttempts:   w.cfg.MaxAttempts,
			LastError:     &errMsg,
			NextAttemptAt: pgtype.Timestamptz{Time: nextAttempt, Valid: true},
			ID:            job.ID,
		}); markErr != nil {
			return fmt.Errorf("worker: mark job %d failed: %w", job.ID, markErr)
		}
		return nil
	}

	if err := w.queries.MarkOutboxJobDone(ctx, job.ID); err != nil {
		return fmt.Errorf("worker: mark job %d done: %w", job.ID, err)
	}
	return nil
}

// ProcessOnceSafe wraps ProcessOnce with a deferred recover() (CR-02,
// 04-REVIEW.md): a panic anywhere inside a single job's processing (e.g. a
// third-party CDP/template.Execute edge case — nil deref, index out of range)
// is logged and swallowed here instead of propagating up through Run's
// goroutine, which would otherwise terminate the ENTIRE donnarec-api process
// (Go panics that escape every goroutine kill the whole process, not just the
// goroutine it started in) — taking the JSON HTTP API down with it over a
// single bad receipt render.
//
// A job whose processing panicked is left claimed ('processing') — it is not
// marked done or failed, since the panic could have interrupted the job at
// any point. ReclaimStuckJobs (CR-01) is what eventually recovers it back to
// 'pending' once StuckJobTimeout elapses, giving it a normal retry via
// ProcessOnce's usual MarkOutboxJobFailed/backoff path on the next claim.
//
// Run calls this instead of ProcessOnce directly. Tests may call it directly
// to prove the recovery behavior without waiting on the ticker loop.
func (w *Worker) ProcessOnceSafe(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("worker: recovered from panic during job processing",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())),
			)
			err = nil
		}
	}()
	return w.ProcessOnce(ctx)
}
