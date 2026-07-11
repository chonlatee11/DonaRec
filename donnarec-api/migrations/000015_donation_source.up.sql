-- migrations/000015_donation_source.up.sql
-- Phase 6: Flow B (public donation web form) — explicit source column (FR-08, D-77)
--
-- Design decisions realized here:
--   D-77: donations.source separates staff-entered records (Flow A) from public
--         web submissions (Flow B) EXPLICITLY — never inferred from created_by,
--         since Flow B still needs a created_by FK (the seeded public-web system
--         user from 000016) and Flow A staff could theoretically be reused later.
--   TEXT + CHECK chosen over an enum type (Claude's discretion, plan 06-01):
--         simpler migration, matches the sqlc.narg('source')::TEXT filter shape
--         already used for donor_name/status/date-range nargs (D-53 precedent) —
--         no CREATE TYPE / ALTER TYPE ceremony for a two-value domain.
--   DEFAULT 'flow_a' backfills every existing row automatically on ADD COLUMN —
--         all pre-Phase-6 donations were staff-entered, so this is correct by
--         construction, not merely a safe default.
--
-- The application connects as role 'donnarec_app'.
-- This migration:
--   1. Adds donations.source (TEXT NOT NULL DEFAULT 'flow_a') with a CHECK
--      constraint limiting values to 'flow_a'/'flow_b'.
--   2. No new GRANT — UPDATE/SELECT on donations is already granted to
--      donnarec_app (000005).

-- ============================================================
-- 1. donations.source (D-77)
-- ============================================================

ALTER TABLE donations
    ADD COLUMN source TEXT NOT NULL DEFAULT 'flow_a'
        CONSTRAINT chk_donations_source CHECK (source IN ('flow_a', 'flow_b'));
