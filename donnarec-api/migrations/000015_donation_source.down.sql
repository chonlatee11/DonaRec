-- migrations/000015_donation_source.down.sql
-- Reverse 000015: drop the source column (and its CHECK constraint along with it).

ALTER TABLE donations
    DROP COLUMN source;
