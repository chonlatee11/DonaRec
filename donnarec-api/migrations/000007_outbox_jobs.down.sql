-- migrations/000007_outbox_jobs.down.sql
-- Reversal of 000007_outbox_jobs.up.sql
--
-- DANGER: This destroys all outbox job records.
-- Run ONLY on local development / disposable test databases.
--
-- outbox_jobs has no FKs to other tables, so a simple DROP suffices.

DROP TABLE IF EXISTS outbox_jobs;
