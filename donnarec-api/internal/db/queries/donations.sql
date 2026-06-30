-- internal/db/queries/donations.sql
-- sqlc queries for Phase 3: donation lifecycle (create/submit/review/approve/cancel/search)
-- All queries use named @params and explicit column lists (no SELECT * in writes, Pattern F).
-- Parameterized only — no string concatenation (T-03 threat model mitigation).
--
-- Key design constraints:
--   - LockDonationForUpdate uses SELECT … FOR UPDATE (D-52, NFR-04) to serialize approvals
--   - IssueDonation has a WHERE status='pending_review' precondition (extra DB-side safety)
--   - CancelDonation retains receipt_number_id/receipt_formatted (FR-19, D-47)
--   - SearchDonations uses nullable @param::TYPE IS NULL pattern for optional filters (D-53)

-- name: LockDonationForUpdate :one
-- Lock the donation row FOR UPDATE to serialize concurrent approval attempts (D-52).
-- Returns the columns needed for precondition checks (status, created_by) and
-- downstream use (receipt_number_id for idempotency, edonation_keyed for cancel guard).
-- Caller MUST hold this lock for the full issuance transaction (see 03-PATTERNS §Issuance).
--
-- Returns pgx.ErrNoRows if the donation does not exist (caller maps to 404).
SELECT id, status, created_by, receipt_number_id, edonation_keyed
FROM donations
WHERE id = @id
FOR UPDATE;

-- name: CreateDonation :one
-- Insert a new donation record in 'draft' status with donor snapshot + PII ciphertext.
-- created_at/updated_at omitted from VALUES — rely on DEFAULT now() (IN-01).
-- donor_tax_id_enc/dek accept ciphertext only — plaintext is encrypted at service layer (D-44).
INSERT INTO donations (
    created_by,
    donor_name,
    donor_address,
    donor_email,
    donor_tax_id_enc,
    donor_tax_id_dek,
    amount,
    donated_at,
    notes,
    consent_given,
    consent_at,
    consent_text_version,
    consent_purpose,
    retain_until,
    legal_basis
) VALUES (
    @created_by,
    @donor_name,
    @donor_address,
    @donor_email,
    @donor_tax_id_enc,
    @donor_tax_id_dek,
    @amount,
    @donated_at,
    @notes,
    @consent_given,
    @consent_at,
    @consent_text_version,
    @consent_purpose,
    @retain_until,
    @legal_basis
)
RETURNING
    id, created_by, status, created_at, updated_at;

-- name: GetDonationByID :one
-- Full read of a donation row including PII ciphertext columns.
-- PII decrypt + masking is done at service layer (never in SQL).
SELECT
    id,
    created_by,
    created_at,
    updated_at,
    status,
    donor_name,
    donor_address,
    donor_email,
    donor_tax_id_enc,
    donor_tax_id_dek,
    amount,
    donated_at,
    notes,
    consent_given,
    consent_at,
    consent_text_version,
    consent_purpose,
    retain_until,
    legal_basis,
    submitted_at,
    reviewed_by,
    reviewed_at,
    review_reason,
    approved_by,
    approved_at,
    receipt_number_id,
    receipt_formatted,
    cancelled_by,
    cancelled_at,
    cancel_reason,
    edonation_keyed,
    replaces,
    replaced_by
FROM donations
WHERE id = @id;

-- name: UpdateDraftDonation :exec
-- Update Maker-editable donor fields. Only allowed while status = 'draft' (FR-09).
-- Checker-set fields (reviewed_by/at/reason) are NOT included here.
-- PII columns accept ciphertext — re-encrypt at service layer before calling.
UPDATE donations
SET
    donor_name           = @donor_name,
    donor_address        = @donor_address,
    donor_email          = @donor_email,
    donor_tax_id_enc     = @donor_tax_id_enc,
    donor_tax_id_dek     = @donor_tax_id_dek,
    amount               = @amount,
    donated_at           = @donated_at,
    notes                = @notes,
    consent_given        = @consent_given,
    consent_at           = @consent_at,
    consent_text_version = @consent_text_version,
    consent_purpose      = @consent_purpose,
    retain_until         = @retain_until,
    legal_basis          = @legal_basis,
    updated_at           = now()
WHERE id     = @id
  AND status = 'draft';

-- name: SubmitDonation :exec
-- Transition draft → pending_review (FR-11 state machine).
-- submitted_at records when the Maker sent it for review.
UPDATE donations
SET
    status       = 'pending_review',
    submitted_at = now(),
    updated_at   = now()
WHERE id     = @id
  AND status = 'draft';

-- name: ReturnDonation :exec
-- Checker returns pending_review → draft with a mandatory reason (D-45, FR-12).
-- review_reason is enforced as non-empty at service layer before this call.
UPDATE donations
SET
    status        = 'draft',
    reviewed_by   = @reviewed_by,
    reviewed_at   = @reviewed_at,
    review_reason = @review_reason,
    updated_at    = now()
WHERE id     = @id
  AND status = 'pending_review';

-- name: RejectDonation :exec
-- Checker permanently rejects pending_review → rejected with a mandatory reason (D-45, FR-12).
-- 'rejected' is a terminal state — no further transitions are allowed.
-- review_reason is enforced as non-empty at service layer before this call.
UPDATE donations
SET
    status        = 'rejected',
    reviewed_by   = @reviewed_by,
    reviewed_at   = @reviewed_at,
    review_reason = @review_reason,
    updated_at    = now()
WHERE id     = @id
  AND status = 'pending_review';

-- name: IssueDonation :exec
-- Stamp receipt fields on approval: pending_review → issued (FR-14, D-38, D-42).
-- WHERE status='pending_review' is an extra DB-side precondition — the primary guard is
-- LockDonationForUpdate + code check in service; this is defense-in-depth (D-52).
-- receipt_number_id must reference an allocated receipt_numbers row (D-38 FK constraint).
-- receipt_formatted is the frozen snapshot from the allocator (D-42 — never recomputed).
UPDATE donations
SET
    status            = 'issued',
    approved_by       = @approved_by,
    approved_at       = @approved_at,
    receipt_number_id = @receipt_number_id,
    receipt_formatted = @receipt_formatted,
    updated_at        = now()
WHERE id     = @id
  AND status = 'pending_review';

-- name: CancelDonation :exec
-- Cancel an issued receipt: issued → cancelled (FR-19, D-47).
-- IMPORTANT: receipt_number_id and receipt_formatted are intentionally NOT modified.
-- The CHECK constraint chk_receipt_only_on_issued_or_cancelled enforces they remain set
-- for 'cancelled' status — preserving the sequence without a gap (D-47).
-- cancel_reason is mandatory at service layer before this call.
UPDATE donations
SET
    status        = 'cancelled',
    cancelled_by  = @cancelled_by,
    cancelled_at  = @cancelled_at,
    cancel_reason = @cancel_reason,
    updated_at    = now()
WHERE id     = @id
  AND status = 'issued';

-- name: SetReplacedBy :exec
-- Link a cancelled record to its reissued successor (D-50 Void & Reissue).
-- Called after the replacement donation has been created and committed.
-- Only updates the replaced_by pointer — no status change here.
UPDATE donations
SET
    replaced_by = @replaced_by,
    updated_at  = now()
WHERE id = @id;

-- name: SearchDonations :many
-- Search donations by name / date range / status / receipt number (FR-10, D-53).
-- All filter params are optional (nullable @param::TYPE IS NULL pattern):
--   pass NULL  → filter is skipped (no restriction applied)
--   pass value → filter is applied
-- Results exclude PII ciphertext columns for performance and least-privilege.
-- Pagination: caller passes @limit_n rows starting at @offset_n.
SELECT
    id,
    status,
    donor_name,
    donated_at,
    amount,
    receipt_formatted,
    created_at,
    approved_at,
    created_by,
    edonation_keyed
FROM donations
WHERE
    (@donor_name::TEXT         IS NULL OR donor_name       ILIKE '%' || @donor_name || '%')
    AND (@status::donation_status IS NULL OR status        = @status)
    AND (@from_date::DATE         IS NULL OR donated_at   >= @from_date)
    AND (@to_date::DATE           IS NULL OR donated_at   <= @to_date)
    AND (@receipt_no::TEXT        IS NULL OR receipt_formatted = @receipt_no)
ORDER BY created_at DESC
LIMIT  @limit_n
OFFSET @offset_n;
