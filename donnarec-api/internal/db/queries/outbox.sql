-- internal/db/queries/outbox.sql
-- sqlc queries for the transactional outbox_jobs table (Phase 3 enqueue only).
-- Phase 4 adds worker queries (poll, update status, mark done/failed).
--
-- Design: INSERT happens inside the issuance transaction — if the receipt issuance
-- rolls back, the outbox row is also rolled back (atomically linked, CLAUDE.md §"Email Delivery").

-- name: EnqueueOutboxJob :exec
-- Enqueue an outbox job inside the issuance transaction (Step 7 of Pattern 1).
-- Caller MUST call this within the same pgx.Tx as IssueDonation so the job exists
-- IFF the receipt was issued — all-or-nothing guarantee.
-- job_type: e.g. 'issue_receipt'
-- payload:  e.g. {"donation_id": "uuid-string"} as JSON bytes
INSERT INTO outbox_jobs (job_type, payload, status)
VALUES (@job_type, @payload, 'pending');

-- name: ClaimNextOutboxJob :one
-- Atomically claim exactly one pending job that is due for (re)processing,
-- race-free across N worker instances/goroutines (04-RESEARCH Pattern 1, verified).
-- Single round-trip UPDATE...WHERE id=(SELECT...FOR UPDATE SKIP LOCKED) — no
-- separate SELECT-then-UPDATE step that could race between two workers.
-- next_attempt_at <= now() excludes jobs still in their backoff window (D-57).
--
-- WR-05 fix (04-REVIEW.md): status = 'pending' ONLY — 'failed' is NEVER
-- claimable here. MarkOutboxJobFailed's own CASE logic already treats
-- 'failed' as terminal (a retriable failure with attempts remaining stays
-- 'pending'; only a truly exhausted job becomes 'failed'), so a job that
-- reaches 'failed' must stay dead-lettered forever — staff-triggered resend
-- (which enqueues a brand-new outbox_jobs row, internal/donation/service.go
-- Resend) is the only way to retry it. Previously this WHERE also matched
-- 'failed' (relying solely on attempts < @max_attempts to keep it
-- unclaimable), which silently "resurrected" a dead-lettered job the moment
-- an operator raised WORKER_MAX_ATTEMPTS after the fact.
--
-- Returns pgx.ErrNoRows when there is no eligible job — caller treats this as
-- "nothing to do this tick", not an error.
UPDATE outbox_jobs AS o
SET status = 'processing',
    updated_at = now()
WHERE o.id = (
    SELECT j.id FROM outbox_jobs AS j
    WHERE j.status = 'pending'
      AND j.next_attempt_at <= now()
      AND j.attempts < @max_attempts
    ORDER BY j.created_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING o.id, o.job_type, o.payload, o.attempts;

-- name: ReclaimStuckOutboxJobs :execrows
-- CR-01 fix (04-REVIEW.md): a job that was claimed (status='processing') but
-- never reached MarkOutboxJobDone/MarkOutboxJobFailed — because the worker
-- process was killed, panicked (see CR-02), OOM-killed, or evicted between
-- the claim and the completion write — would otherwise stay 'processing'
-- forever: ClaimNextOutboxJob's own filter (status = 'pending') deliberately
-- excludes 'processing' rows so two workers never double-process the same
-- job, but that same exclusion means a truly abandoned job had no way back
-- into the claimable set.
--
-- Any row whose updated_at is older than @cutoff (now() - StuckJobTimeout,
-- computed in Go so the threshold is configurable via WORKER_STUCK_JOB_TIMEOUT)
-- is reset to 'pending' — safe because updated_at is bumped by BOTH the claim
-- (ClaimNextOutboxJob) and the completion writes (MarkOutboxJobDone/Failed), so
-- a row this old can only mean the job is genuinely abandoned, never a
-- healthy in-flight render/email still within its normal ~2-3s budget
-- (NFR-07) — the default timeout is minutes, several orders of magnitude
-- above that budget.
UPDATE outbox_jobs
SET status = 'pending',
    updated_at = now()
WHERE status = 'processing'
  AND updated_at < @cutoff;

-- name: MarkOutboxJobDone :exec
-- Mark a claimed job as successfully processed (render + store + email all
-- completed). Terminal state — a done job is never reclaimed.
UPDATE outbox_jobs
SET status = 'done',
    updated_at = now()
WHERE id = @id;

-- name: MarkOutboxJobFailed :exec
-- Record a failed processing attempt and either re-arm the job for retry
-- (status stays 'pending', next_attempt_at pushed out per the caller's backoff
-- schedule — D-57 Pitfall 5: 1m/5m/15m/1h/4h) or, once attempts reaches
-- @max_attempts, transition to the terminal 'failed' state (dead-letter — no
-- further auto-retry; staff see the failure and can resend manually, FR-27).
UPDATE outbox_jobs
SET status = CASE WHEN attempts + 1 >= @max_attempts THEN 'failed' ELSE 'pending' END,
    attempts = attempts + 1,
    last_error = @last_error,
    next_attempt_at = @next_attempt_at,
    updated_at = now()
WHERE id = @id;
