-- audit.sql — sqlc queries for the audit_log table
-- All queries use explicit column lists (no SELECT * in writes per Foundational Rule 4).
-- Parameterized queries only — no string concatenation (T-1-tamper-01).

-- NOTE (WR-05): The previous InsertAuditLog :one query was removed. The audit
-- service writes rows via a raw tx.Exec INSERT in AppendAuditEntryTx that also
-- sets the reserved `id` and captured `created_at` explicitly (both feed the
-- hash-chain). That sqlc query omitted id/created_at and was never used, so
-- keeping two divergent insert definitions for the immutable audit table was a
-- maintenance trap. There is now exactly one insert path (the raw exec).

-- name: GetLastAuditRowForUpdate :one
-- Fetches the most recent audit row's id and row_hash, locking it with FOR UPDATE.
-- This serializes concurrent hash-chain appends: the next INSERT cannot proceed
-- until the current transaction releases this lock (Pitfall 2 mitigation, D-17).
-- Returns pgx.ErrNoRows if audit_log is empty (caller sets prevHash = "GENESIS").
SELECT id, row_hash
FROM audit_log
ORDER BY id DESC
LIMIT 1
FOR UPDATE;

-- name: ListAuditLogs :many
-- Admin-only paginated listing of audit entries, newest first.
-- actor_id filter is optional: pass NULL to list all actors.
-- Caller must enforce Admin role before invoking (D-16).
SELECT
    id,
    actor_id,
    actor_email,
    action,
    resource,
    before_json,
    after_json,
    ip_address,
    created_at,
    prev_hash,
    row_hash
FROM audit_log
WHERE (@actor_id::uuid IS NULL OR actor_id = @actor_id)
ORDER BY created_at DESC
LIMIT @limit_n
OFFSET @offset_n;

-- name: ListAllAuditForVerify :many
-- Returns all audit rows in ascending id order for chain verification.
-- Used by VerifyChain to recompute each row_hash and detect tampering.
-- Admin / internal tool only — no pagination (verification reads entire chain).
SELECT
    id,
    actor_id,
    actor_email,
    action,
    resource,
    created_at,
    prev_hash,
    row_hash
FROM audit_log
ORDER BY id ASC;
