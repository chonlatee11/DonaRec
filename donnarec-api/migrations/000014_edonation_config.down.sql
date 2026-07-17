-- migrations/000014_edonation_config.down.sql
-- Reversal of 000014_edonation_config.up.sql
--
-- DANGER: This destroys the admin-configured e-Donation field mapping / cash
-- type label / near-due-days config. Run ONLY on local development / disposable
-- test databases.

DROP TABLE IF EXISTS edonation_config;
