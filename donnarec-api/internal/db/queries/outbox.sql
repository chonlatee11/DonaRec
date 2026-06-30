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
