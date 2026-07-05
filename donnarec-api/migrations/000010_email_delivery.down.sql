-- migrations/000010_email_delivery.down.sql
-- Reversal of 000010_email_delivery.up.sql
--
-- DANGER: This destroys all email delivery history records.
-- Run ONLY on local development / disposable test databases.
--
-- email_delivery has an FK to donations but no table depends on email_delivery,
-- so a simple DROP suffices.

DROP TABLE IF EXISTS email_delivery;
