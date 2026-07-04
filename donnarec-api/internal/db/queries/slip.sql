-- internal/db/queries/slip.sql
-- sqlc queries for Plan 03-04: slip_attachments (D-48, D-54)
--
-- Rules (Pattern F, PATTERNS.md):
--   1. Always use @param_name (not $1 positional) for sqlc named params
--   2. Named queries: -- name: QueryName :one/:many/:exec
--   3. No bare SELECT * — explicit column list
--   4. uploaded_at omitted from INSERT VALUES (DEFAULT now() in schema)
--   5. No string concatenation — all parameterized (T-02-03 mitigation)
--   6. SoftDeleteSlip uses UPDATE (not DELETE) — REVOKE DELETE enforced at DB level (D-54)

-- name: InsertSlip :one
-- Inserts a new slip reference after successful MinIO PutObject.
-- Called within a WithTx in slip_service.UploadSlip.
INSERT INTO slip_attachments (donation_id, object_key, mime_type, size_bytes, uploaded_by)
VALUES (@donation_id, @object_key, @mime_type, @size_bytes, @uploaded_by)
RETURNING id, donation_id, object_key, mime_type, size_bytes, uploaded_by, uploaded_at, deleted_at, deleted_by;

-- name: GetActiveSlipByDonation :one
-- Returns the active (non-deleted) slip reference for a donation.
-- Used by ViewSlip to obtain the object_key for PresignedGet.
-- Returns pgx.ErrNoRows if no active slip exists (normal for cash/no-slip donations — D-48).
SELECT id, donation_id, object_key, mime_type, size_bytes, uploaded_by, uploaded_at, deleted_at, deleted_by
FROM slip_attachments
WHERE donation_id = @donation_id
  AND deleted_at IS NULL;

-- name: SoftDeleteSlip :exec
-- Soft-deletes a slip reference (D-54): sets deleted_at + deleted_by.
-- File is NOT removed from MinIO — retained for audit/evidence.
-- Called within a WithTx in slip_service.RemoveSlip alongside AppendAuditEntryTx.
UPDATE slip_attachments
SET deleted_at = now(),
    deleted_by  = @deleted_by
WHERE id = @id;
