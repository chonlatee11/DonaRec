-- migrations/000005_donations.up.sql
-- Phase 3: Donation entity — lifecycle, PII snapshot, SoD backstop, receipt FK
--
-- Design decisions realized here:
--   D-43: Snapshot-only — no donor master table, no blind index in Phase 3
--   D-44: donor_tax_id_enc/dek NOT NULL — ciphertext mandatory at API boundary
--   D-45: Two distinct review actions: return (non-terminal) vs reject (terminal)
--   D-47: Cancellation retains receipt_number_id — no gap in sequence
--   D-49: Consent capture — consent_given/at/text_version/purpose per snapshot
--   D-50: Void & Reissue self-FKs: replaces / replaced_by on donations(id)
--   D-51: edonation_keyed flag for RD reconciliation guard on cancel
--   D-52: LockDonationForUpdate (SELECT FOR UPDATE) serializes concurrent approvals
--   D-38: receipt_number_id BIGINT FK to receipt_numbers ledger (Phase 2)
--   D-42: receipt_formatted frozen snapshot (never recomputed from config)
--
-- The application connects as role 'donnarec_app'.
-- This migration:
--   1. Creates donation_status enum (5 values, FR-11)
--   2. Creates donations table with full donor snapshot + consent + reissue links
--   3. Adds SoD CHECK (chk_sod_approver) and receipt-on-issued CHECK (T-03-01, T-03-02)
--   4. Creates indexes for FR-10 search (D-53)
--   5. Grants minimal privileges to donnarec_app; REVOKEs DELETE (FR-19, T-03-03)

-- ============================================================
-- 1. Enum: donation lifecycle statuses (FR-11)
-- ============================================================

CREATE TYPE donation_status AS ENUM (
    'draft',
    'pending_review',
    'issued',
    'rejected',
    'cancelled'
);

-- ============================================================
-- 2. Donations table (core entity)
-- ============================================================

CREATE TABLE donations (
    id                      UUID            PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Maker & lifecycle
    created_by              UUID            NOT NULL REFERENCES users(id),
    created_at              TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ     NOT NULL DEFAULT now(),
    status                  donation_status NOT NULL DEFAULT 'draft',

    -- Donor snapshot (immutable after issue, D-43)
    donor_name              TEXT            NOT NULL,
    donor_address           TEXT            NOT NULL DEFAULT '',
    donor_email             TEXT,                               -- optional

    -- PII: national/tax ID encrypted at rest (PDPA, NFR-02, D-44)
    -- Columns store AES-256-GCM ciphertext only — plaintext NEVER persisted (T-03-04)
    donor_tax_id_enc        BYTEA           NOT NULL,           -- ciphertext
    donor_tax_id_dek        BYTEA           NOT NULL,           -- wrapped DEK

    -- Donation detail
    amount                  NUMERIC(15,2)   NOT NULL CHECK (amount > 0),
    donated_at              DATE            NOT NULL,
    notes                   TEXT,

    -- Consent capture (D-49, NFR-03)
    consent_given           BOOLEAN         NOT NULL DEFAULT false,
    consent_at              TIMESTAMPTZ,
    consent_text_version    TEXT,
    consent_purpose         TEXT,

    -- Retention (Phase 1 model)
    retain_until            DATE,
    legal_basis             TEXT            NOT NULL DEFAULT 'tax_obligation',

    -- Submit
    submitted_at            TIMESTAMPTZ,

    -- Return / Reject (Checker review loop or terminal rejection, D-45)
    reviewed_by             UUID            REFERENCES users(id),
    reviewed_at             TIMESTAMPTZ,
    review_reason           TEXT,               -- mandatory on return/reject

    -- Approval (issuance)
    approved_by             UUID            REFERENCES users(id),
    approved_at             TIMESTAMPTZ,

    -- Receipt number — FK to Phase 2 ledger (D-38)
    receipt_number_id       BIGINT          REFERENCES receipt_numbers(id),
    receipt_formatted       TEXT,               -- frozen snapshot (D-42)

    -- Cancellation (D-47)
    cancelled_by            UUID            REFERENCES users(id),
    cancelled_at            TIMESTAMPTZ,
    cancel_reason           TEXT,
    edonation_keyed         BOOLEAN         NOT NULL DEFAULT false,  -- D-51

    -- Void & Reissue self-FKs (D-50)
    replaces                UUID            REFERENCES donations(id),  -- this record replaces (old)
    replaced_by             UUID            REFERENCES donations(id),  -- replaced by this new record

    -- SoD DB backstop (CLAUDE.md defense-in-depth — approver cannot be creator, T-03-01)
    CONSTRAINT chk_sod_approver
        CHECK (approved_by IS NULL OR approved_by != created_by),

    -- Receipt number must be set IFF status is issued or cancelled (D-38, D-47, FR-19, T-03-02)
    CONSTRAINT chk_receipt_only_on_issued_or_cancelled
        CHECK (
            (status IN ('issued','cancelled') AND receipt_number_id IS NOT NULL AND receipt_formatted IS NOT NULL)
            OR (status NOT IN ('issued','cancelled') AND receipt_number_id IS NULL AND receipt_formatted IS NULL)
        )
);

-- ============================================================
-- 3. Indexes for FR-10 search (D-53: name, date, status, receipt_no)
-- ============================================================

CREATE INDEX idx_donations_donor_name        ON donations (donor_name);
CREATE INDEX idx_donations_donated_at        ON donations (donated_at);
CREATE INDEX idx_donations_status            ON donations (status);
-- Partial index: most donations won't have a receipt number (draft/pending/rejected)
CREATE INDEX idx_donations_receipt_number_id ON donations (receipt_number_id)
    WHERE receipt_number_id IS NOT NULL;
CREATE INDEX idx_donations_created_by        ON donations (created_by);
-- Partial index: only approved records have approved_by
CREATE INDEX idx_donations_approved_by       ON donations (approved_by)
    WHERE approved_by IS NOT NULL;

-- ============================================================
-- 4. Permissions for donnarec_app role
-- ============================================================

-- Donations: app may read, insert, and update (state transitions, review, approval)
GRANT SELECT, INSERT, UPDATE ON donations TO donnarec_app;

-- No DELETE — donation records are immutable; status='cancelled' is the only removal path
-- (FR-19 immutability requirement, T-03-03 Tampering mitigation)
REVOKE DELETE ON donations FROM donnarec_app;
