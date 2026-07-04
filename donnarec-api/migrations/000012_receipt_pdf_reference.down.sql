-- migrations/000012_receipt_pdf_reference.down.sql
-- Reversal of 000012_receipt_pdf_reference.up.sql
--
-- Drops receipt_pdf_object_key. Run ONLY on local development / disposable
-- test databases — this loses the reference to frozen receipt PDFs.

ALTER TABLE donations
    DROP COLUMN IF EXISTS receipt_pdf_object_key;
