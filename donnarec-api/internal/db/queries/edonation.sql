-- internal/db/queries/edonation.sql
-- sqlc queries for Phase 5: e-Donation export source, keyed/aging status, and
-- the edonation_config accessor (FR-30/FR-31, D-51, D-66, D-67, D-68, D-75).
-- All filter params are optional via the sqlc.narg('...') pattern, matching
-- donations.sql's discipline (D-53) — nullable params compile to nullable Go
-- fields, so a nil filter is skip-this-filter, never an accidental empty match.

-- name: SearchIssuedForExport :many
-- Export source (FR-30/D-66): issued donations only, optional donated_at range
-- and optional keyed-status filter. Ciphertext columns are decrypted at the
-- SERVICE layer (05-RESEARCH.md Pattern 3) — never in SQL. No FOR UPDATE — this
-- is a plain read, not part of the approval-locking path (D-52 is unrelated).
SELECT
    id,
    donor_name,
    donor_tax_id_enc,
    donor_tax_id_dek,
    donated_at,
    receipt_formatted,
    edonation_keyed
FROM donations
WHERE status = 'issued'
  AND (sqlc.narg('from_date')::DATE      IS NULL OR donated_at >= sqlc.narg('from_date'))
  AND (sqlc.narg('to_date')::DATE        IS NULL OR donated_at <= sqlc.narg('to_date'))
  AND (sqlc.narg('keyed_status')::BOOLEAN IS NULL OR edonation_keyed = sqlc.narg('keyed_status'))
ORDER BY donated_at ASC;

-- name: SearchUnkeyedIssued :many
-- Aging view source (FR-31/D-68): all issued, not-yet-keyed donations with
-- their approval timestamp — approved_at is the D-68 aging base date (donations
-- has NO issued_at column); the aging bucket itself is computed in Go
-- (internal/edonation/aging.go, 05-RESEARCH.md Pattern 5), not in SQL.
SELECT id, donor_name, receipt_formatted, approved_at, edonation_keyed
FROM donations
WHERE status = 'issued' AND edonation_keyed = false
ORDER BY approved_at ASC;

-- name: SetKeyedBulk :exec
-- Bulk mark/unmark keyed status (D-67) — one statement covers the whole
-- selection. This stays a plain boolean UPDATE — no sequence/allocator
-- machinery (05-RESEARCH.md Anti-Patterns). The caller writes one audit row
-- PER donation_id afterward (Pattern 4 — distinct from export's single
-- summary-row audit rationale). status='issued' guard mirrors the other
-- lifecycle UPDATEs' defense-in-depth precondition style (donations.sql).
UPDATE donations
SET
    edonation_keyed    = @keyed,
    edonation_keyed_at = @keyed_at,
    edonation_keyed_by = @keyed_by
WHERE id = ANY(@donation_ids::uuid[])
  AND status = 'issued';

-- name: GetEdonationConfig :one
-- Read the single e-Donation config row (field mapping + cash-type label +
-- aging threshold). Called by the export/aging services and by the admin
-- settings 5th tab (D-75, Pattern 6).
SELECT
    field_mapping,
    cash_type_label,
    near_due_days,
    updated_at,
    updated_by
FROM edonation_config
LIMIT 1;

-- name: UpdateEdonationConfig :exec
-- Update the single config row (Admin-only, D-75). updated_by is set to the
-- acting admin's app-user id for the audit trail (Pattern D, FR-13), mirroring
-- settings.sql's UpdateReceiptTemplateConfig shape.
UPDATE edonation_config
SET
    field_mapping    = @field_mapping,
    cash_type_label  = @cash_type_label,
    near_due_days    = @near_due_days,
    updated_at       = now(),
    updated_by       = @updated_by
WHERE id = true;
