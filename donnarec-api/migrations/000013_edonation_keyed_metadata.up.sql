-- migrations/000013_edonation_keyed_metadata.up.sql
-- Phase 5: e-Donation keyed-status actor/timestamp metadata (FR-31, D-51, D-75)
--
-- Design decisions realized here:
--   D-51: donations.edonation_keyed (BOOLEAN) already exists since 000005 — this
--         migration adds ONLY the "who/when marked" metadata columns so the aging
--         page can answer that question without joining audit_log on every read.
--   Pattern 4 (05-RESEARCH.md): extend the EXISTING column set — no new table,
--         no allocator/locking machinery. The keyed flag stays a plain boolean
--         UPDATE (see internal/db/queries/edonation.sql SetKeyedBulk).
--
-- The application connects as role 'donnarec_app'.
-- This migration:
--   1. Adds edonation_keyed_at (TIMESTAMPTZ, nullable) and edonation_keyed_by
--      (UUID FK users(id), nullable) to donations.
--   2. No new GRANT — UPDATE on donations is already granted to donnarec_app (000005).

-- ============================================================
-- 1. donations.edonation_keyed_at / edonation_keyed_by (D-51, Pattern 4)
-- ============================================================

ALTER TABLE donations
    ADD COLUMN edonation_keyed_at TIMESTAMPTZ,
    ADD COLUMN edonation_keyed_by UUID REFERENCES users(id);
