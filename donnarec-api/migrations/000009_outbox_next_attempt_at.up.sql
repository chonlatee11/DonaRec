-- migrations/000009_outbox_next_attempt_at.up.sql
-- Phase 4: Outbox job backoff scheduling (D-57, 04-RESEARCH Pattern 1)
--
-- Design decisions realized here:
--   D-57: auto-retry with backoff, terminal 'failed' after max attempts.
--   next_attempt_at lets the worker's atomic claim query
--   (ClaimNextOutboxJob, internal/db/queries/outbox.sql) filter out jobs that
--   are not yet due for a retry, without a separate scheduler/cron process.
--   DEFAULT now() means existing/new pending jobs are immediately claimable.
--
-- The application connects as role 'donnarec_app'.
-- This migration:
--   1. Adds next_attempt_at column to outbox_jobs (NOT NULL, default now())
--
-- No index changes: the existing partial index idx_outbox_jobs_pending
-- (status, created_at) WHERE status IN ('pending','failed') still supports
-- the claim query's WHERE status IN (...) AND next_attempt_at <= now()
-- predicate — next_attempt_at is a residual filter over an already-narrow
-- partial-index scan, not a leading index column (RESEARCH.md Pattern 1 note).

-- ============================================================
-- 1. outbox_jobs.next_attempt_at (D-57)
-- ============================================================

ALTER TABLE outbox_jobs
    ADD COLUMN next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();
