-- migrations/000008_donor_language.down.sql
-- Reversal of 000008_donor_language.up.sql
--
-- Drops the donor_language column. Language selection is lost on rollback —
-- acceptable for local development / disposable test databases only.

ALTER TABLE donations
    DROP COLUMN IF EXISTS donor_language;
