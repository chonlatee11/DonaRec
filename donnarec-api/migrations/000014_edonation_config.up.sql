-- migrations/000014_edonation_config.up.sql
-- Phase 5: e-Donation export config store (FR-30/FR-31, D-65, D-68, D-75, NFR-09)
--
-- Design decisions realized here:
--   D-75: field_mapping (JSONB) — column order/names for the e-Donation export are
--         config-driven, never hardcoded in Go, so the real RD e-Donation column
--         spec (stakeholder gate, still pending confirmation) can be edited without
--         a deploy once confirmed.
--   D-65: cash_type_label — constant "เงินสด/โอน" value, still admin-editable.
--   D-68: near_due_days — aging-bucket "near due" threshold (days before deadline).
--   Pattern 6 (05-RESEARCH.md): sibling single-row table, same shape as
--         receipt_number_config (000004) and receipt_template_config (000011) —
--         id BOOLEAN PRIMARY KEY DEFAULT true + CHECK(id = true), NOT an ALTER of
--         either existing config table.
--
-- The application connects as role 'donnarec_app'.
-- This migration:
--   1. Creates edonation_config (single-row enforced) and seeds it with a sane
--      default field_mapping so a fresh install produces a usable export before
--      any stakeholder edit.
--   2. Grants SELECT, INSERT, UPDATE to donnarec_app.

-- ============================================================
-- 1. edonation_config — single-row table (Pattern 6), mirrors 000004/000011's
--    id BOOLEAN PRIMARY KEY DEFAULT true + CHECK(id = true) pattern.
-- ============================================================

CREATE TABLE edonation_config (
    id                  BOOLEAN     PRIMARY KEY DEFAULT true,
    CONSTRAINT          single_row CHECK (id = true),

    -- Field mapping (D-75) — ordered JSONB array of
    -- {column_key, header_th, header_en} objects; column order/names editable
    -- without a deploy once the real RD e-Donation spec is confirmed.
    field_mapping       JSONB       NOT NULL DEFAULT '[]'::jsonb,

    -- Cash-type label (D-65) — constant value, still admin-editable.
    cash_type_label     TEXT        NOT NULL DEFAULT 'เงินสด/โอน',

    -- Aging threshold (D-68) — "near due" window, days before deadline.
    near_due_days       INTEGER     NOT NULL DEFAULT 3
                            CHECK (near_due_days >= 0),

    -- Audit fields
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by          UUID        REFERENCES users(id)
);

-- Seed the single config row, then populate it with a sane default field
-- mapping (national ID / donation date / cash type / receipt number / donor
-- name) so a fresh install produces a usable export before any stakeholder
-- edit. ON CONFLICT makes this idempotent on re-run (mirrors 000004/000011's
-- seed pattern).
INSERT INTO edonation_config DEFAULT VALUES
ON CONFLICT (id) DO NOTHING;

UPDATE edonation_config
SET field_mapping = '[
    {"column_key": "national_id", "header_th": "เลขบัตรประชาชน/เลขผู้เสียภาษี", "header_en": "National ID"},
    {"column_key": "donated_at",  "header_th": "วันที่บริจาค",                    "header_en": "Donation Date"},
    {"column_key": "cash_type",   "header_th": "ประเภทการชำระเงิน",               "header_en": "Cash Type"},
    {"column_key": "receipt_no",  "header_th": "เลขที่ใบเสร็จ",                    "header_en": "Receipt No."},
    {"column_key": "donor_name",  "header_th": "ชื่อผู้บริจาค",                     "header_en": "Donor Name"}
]'::jsonb
WHERE id = true;

-- ============================================================
-- 2. Permissions for donnarec_app role
-- ============================================================

GRANT SELECT, INSERT, UPDATE ON edonation_config TO donnarec_app;
