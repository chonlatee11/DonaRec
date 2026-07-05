-- migrations/000010_email_delivery.up.sql
-- Phase 4: Email delivery status tracking (FR-27, D-57)
--
-- Design decisions realized here:
--   D-57: one row per send attempt (auto-retry AND staff manual resend both
--         insert a new row here — never overwrite a prior attempt's record),
--         recording status/provider_message_id/attempts/error to support
--         staff-visible delivery status + resend (FR-27).
--   D-60: EmailSender is an interface; provider_message_id is "" for the
--         dev/local sender and populated once a real provider (SES/Postmark)
--         is wired in a later phase.
--
-- The application connects as role 'donnarec_app'.
-- This migration:
--   1. Creates email_delivery table with status CHECK constraint
--   2. Creates index for latest-delivery-per-donation lookups (Screen 3b panel)
--   3. Grants SELECT, INSERT, UPDATE
--   4. Grants USAGE, SELECT on SEQUENCE for BIGSERIAL id

-- ============================================================
-- 1. Email delivery table (one row per send attempt, CLAUDE.md §"Email Delivery")
-- ============================================================

CREATE TABLE email_delivery (
    id                  BIGSERIAL   PRIMARY KEY,
    donation_id         UUID        NOT NULL REFERENCES donations(id),
    sent_to             TEXT,
    status              TEXT        NOT NULL
                            CHECK (status IN ('sent', 'failed', 'no_email')),
    provider_message_id TEXT,
    attempts            INT         NOT NULL DEFAULT 0,
    last_error          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- 2. Index for latest-delivery-per-donation lookups (Screen 3b status panel)
-- ============================================================

CREATE INDEX idx_email_delivery_donation_created
    ON email_delivery (donation_id, created_at DESC);

-- ============================================================
-- 3. Permissions for donnarec_app role
-- ============================================================

GRANT SELECT, INSERT, UPDATE ON email_delivery TO donnarec_app;

-- Sequence for BIGSERIAL id — app needs USAGE + SELECT for nextval/currval
GRANT USAGE, SELECT ON SEQUENCE email_delivery_id_seq TO donnarec_app;
