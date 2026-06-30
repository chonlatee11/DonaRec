-- migrations/000005_donations.down.sql
-- Reversal of 000005_donations.up.sql
--
-- DANGER: This destroys all donation records.
-- Run ONLY on local development / disposable test databases.
--
-- Drop order: donations table first (holds self-FKs + FK to receipt_numbers),
--             then donation_status enum (referenced by the table)
--
-- Note: 000006_slip_attachments (FK → donations) must be rolled back first
--       if it exists; golang-migrate handles ordering automatically.

DROP TABLE IF EXISTS donations;
DROP TYPE IF EXISTS donation_status;
