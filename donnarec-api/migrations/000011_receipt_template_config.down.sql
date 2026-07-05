-- migrations/000011_receipt_template_config.down.sql
-- Reversal of 000011_receipt_template_config.up.sql
--
-- DANGER: This destroys the admin-configured receipt template/branding config.
-- Run ONLY on local development / disposable test databases.

DROP TABLE IF EXISTS receipt_template_config;
