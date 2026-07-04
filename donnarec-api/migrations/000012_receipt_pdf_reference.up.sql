-- migrations/000012_receipt_pdf_reference.up.sql
-- Phase 4: Frozen receipt PDF reference (FR-24, D-56, immutability)
--
-- Design decisions realized here:
--   D-56: Render once — the worker (04-05) renders the receipt PDF exactly once
--         when it processes the issue_receipt outbox job, stores the file in
--         MinIO (bucket separate from slips, e.g. donnarec-receipts), and writes
--         the resulting object key here. resend (04-06) and download (04-06)
--         always reuse this same file — never re-render, even if the template
--         config changes later (same immutability principle as D-42 frozen
--         receipt number / D-43 frozen donor snapshot).
--   Column, not a separate table (RESEARCH Open Question 2) — a donation has at
--         most one frozen PDF, so a nullable column on donations is sufficient;
--         nullable because the column is unpopulated until the worker completes.
--
-- The application connects as role 'donnarec_app'.
-- This migration:
--   1. Adds receipt_pdf_object_key column to donations (nullable)

-- ============================================================
-- 1. donations.receipt_pdf_object_key (D-56)
-- ============================================================

ALTER TABLE donations
    ADD COLUMN receipt_pdf_object_key TEXT;
