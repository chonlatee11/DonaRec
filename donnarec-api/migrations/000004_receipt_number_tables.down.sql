-- migrations/000004_receipt_number_tables.down.sql
-- Reversal of 000004_receipt_number_tables.up.sql
--
-- DANGER: This migration destroys all receipt number data (counters, ledger, config).
-- The receipt_numbers ledger is legally significant (FR-16, NFR-04 — audit/tax records).
-- Run ONLY on local development / disposable test databases.
--
-- Drop order: child → parent (foreign key safe):
--   receipt_numbers (may be FK target of Phase 3 receipts) → receipt_number_counters → receipt_number_config

DROP TABLE IF EXISTS receipt_numbers;
DROP TABLE IF EXISTS receipt_number_counters;
DROP TABLE IF EXISTS receipt_number_config;
