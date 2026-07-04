-- migrations/000009_outbox_next_attempt_at.down.sql
-- Reversal of 000009_outbox_next_attempt_at.up.sql
--
-- Drops next_attempt_at. Run ONLY on local development / disposable test databases.

ALTER TABLE outbox_jobs
    DROP COLUMN IF EXISTS next_attempt_at;
