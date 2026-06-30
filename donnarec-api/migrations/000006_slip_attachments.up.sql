-- migrations/000006_slip_attachments.up.sql
-- Phase 3, Plan 04: slip_attachments reference table (D-48, D-54, FR-09)
--
-- Design decisions realized here:
--   D-48: slip attachment is OPTIONAL in Flow A — cash/no-slip donations fully supported
--   D-54: soft-delete only — deleted_at set on remove; file retained in MinIO; audited
--   T-03-17: Repudiation guard — REVOKE DELETE prevents hard-delete of reference rows
--
-- The application connects as role 'donnarec_app'.
-- This migration:
--   1. Creates slip_attachments (FK → donations + users; soft-delete via deleted_at)
--   2. Creates partial index on (donation_id) WHERE deleted_at IS NULL for fast lookup
--   3. Grants SELECT, INSERT, UPDATE to donnarec_app; REVOKEs DELETE (soft-delete enforced)

-- ============================================================
-- 1. slip_attachments — object storage reference table (D-54)
--    Files are stored in MinIO; DB holds the object_key reference only.
--    Never store binary content here (CLAUDE.md §"What NOT to Use").
-- ============================================================

CREATE TABLE slip_attachments (
    id           UUID        NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Donation this slip belongs to (required; one active slip per donation at a time)
    donation_id  UUID        NOT NULL REFERENCES donations(id),

    -- MinIO/S3 object reference (format: slips/{donationID}/{uuid}{ext})
    object_key   TEXT        NOT NULL,
    mime_type    TEXT        NOT NULL,  -- detected from magic bytes (T-03-14)
    size_bytes   BIGINT      NOT NULL,

    -- Uploader identity (Maker or admin staff)
    uploaded_by  UUID        NOT NULL REFERENCES users(id),
    uploaded_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Soft-delete fields (D-54: reference retained even after removal; file not deleted)
    deleted_at   TIMESTAMPTZ NULL,
    deleted_by   UUID        NULL REFERENCES users(id)
);

-- ============================================================
-- 2. Index for active slip lookup (most common query path)
--    Partial index: only active (non-deleted) rows — keeps the index small.
-- ============================================================

CREATE INDEX idx_slip_attachments_active_by_donation
    ON slip_attachments (donation_id)
    WHERE deleted_at IS NULL;

-- ============================================================
-- 3. Permissions for donnarec_app role
-- ============================================================

-- App may read, insert, and update (soft-delete sets deleted_at via UPDATE).
GRANT SELECT, INSERT, UPDATE ON slip_attachments TO donnarec_app;

-- No hard-delete — soft-delete only (D-54, T-03-17 Repudiation guard).
-- Defense-in-depth: even if application logic has a bug, DB-level REVOKE prevents data loss.
REVOKE DELETE ON slip_attachments FROM donnarec_app;
