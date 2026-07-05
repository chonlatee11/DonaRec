-- migrations/000008_donor_language.up.sql
-- Phase 4: Donor document language capture (FR-23, D-55)
--
-- Design decisions realized here:
--   D-55: donor_language captured at donation-create time (Maker choice, Flow A),
--         frozen as part of the immutable snapshot (same principle as D-43 donor
--         snapshot / D-42 frozen receipt number) — PDF + email always use this
--         value, never re-derived or toggled later.
--   Existing rows backfill to 'th' via the column DEFAULT (no data migration needed).
--
-- The application connects as role 'donnarec_app'.
-- This migration:
--   1. Adds donor_language column to donations (NOT NULL, default 'th', CHECK th|en)

-- ============================================================
-- 1. donations.donor_language (D-55, FR-23)
-- ============================================================

ALTER TABLE donations
    ADD COLUMN donor_language TEXT NOT NULL DEFAULT 'th'
        CHECK (donor_language IN ('th', 'en'));
