-- migrations/000011_receipt_template_config.up.sql
-- Phase 4: Receipt template / branding config store (FR-33, NFR-09, D-58/D-59)
--
-- Design decisions realized here:
--   D-58: Full config store (DB) + Admin UI — template HTML, watermark/signature/
--         seal/letterhead images, section 6 text, deduction multiplier can all be
--         edited without a deploy. Single-row table, same shape as Phase 2's
--         receipt_number_config (000004) — deliberately a SIBLING table, not an
--         ALTER of receipt_number_config (RESEARCH Open Question 3).
--   D-59: deduction_multiplier is a single hospital-wide value (1x/2x) — no
--         per-donation field in MVP.
--   Images are stored as MinIO object keys (letterhead/seal/signature/watermark),
--         never as BLOBs (CLAUDE.md "What NOT to Use").
--   Seed: the row is pre-populated with a minimal-but-complete bilingual HTML
--         skeleton (placeholders: donor_name, receipt_no, amount, issue_date,
--         section6_text, letterhead/signature/watermark img slots) so the worker
--         can render a meaningful receipt (04-05) before any admin ever opens the
--         settings UI (04-07). section6_text_th/en are left '' pending the
--         accounting/legal §6-wording sign-off (STATE.md Blockers/Concerns —
--         Phase 4 stakeholder gate); this is intentionally NOT blocking build.
--
-- The application connects as role 'donnarec_app'.
-- This migration:
--   1. Creates receipt_template_config (single-row enforced) and seeds it
--   2. Grants SELECT, INSERT, UPDATE to donnarec_app

-- ============================================================
-- 1. receipt_template_config — single-row table (D-58), mirrors 000004's
--    id BOOLEAN PRIMARY KEY DEFAULT true + CHECK(id = true) pattern.
-- ============================================================

CREATE TABLE receipt_template_config (
    id                      BOOLEAN     PRIMARY KEY DEFAULT true,
    CONSTRAINT              single_row CHECK (id = true),

    -- HTML templates (D-58) — one per language (FR-23); admin-editable, html/template
    -- contextual autoescaping applied at render time (04-RESEARCH Pattern 3), never
    -- raw-eval'd (D-58 security flag).
    template_html           TEXT        NOT NULL DEFAULT '',
    template_html_en        TEXT        NOT NULL DEFAULT '',

    -- Tax-deduction wording (FR-24) — text per language; multiplier is global (D-59)
    section6_text_th        TEXT        NOT NULL DEFAULT '',
    section6_text_en        TEXT        NOT NULL DEFAULT '',
    deduction_multiplier    TEXT        NOT NULL DEFAULT '1x'
                                CHECK (deduction_multiplier IN ('1x', '2x')),

    -- Branding assets (FR-20/21/22) — MinIO object keys, never BLOBs
    letterhead_object_key   TEXT,
    seal_object_key         TEXT,
    signature_object_key    TEXT,
    watermark_object_key    TEXT,

    -- Audit fields
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by              UUID
);

-- Seed the single config row, then populate it with a minimal-but-complete
-- bilingual receipt skeleton so the worker (04-05) has something meaningful
-- to render before any admin edits the template via the settings UI (04-07).
-- ON CONFLICT makes this idempotent on re-run (mirrors 000004's seed pattern).
INSERT INTO receipt_template_config DEFAULT VALUES
ON CONFLICT (id) DO NOTHING;

UPDATE receipt_template_config
SET template_html = $tmpl_th$
<!DOCTYPE html>
<html lang="th">
<head>
<meta charset="UTF-8">
<style>
  body { font-family: 'TH Sarabun New', sans-serif; font-size: 16pt; position: relative; }
  .watermark { position: absolute; top: 35%; left: 15%; width: 60%; opacity: 0.12; z-index: -1; }
  .letterhead { width: 100%; margin-bottom: 12px; }
  .signature { height: 70px; display: block; margin-top: 8px; }
  .section6 { margin-top: 24px; font-size: 14pt; white-space: pre-wrap; }
  .field { margin: 4px 0; }
</style>
</head>
<body>
  <img class="watermark" src="{{.WatermarkData}}" alt="">
  <img class="letterhead" src="{{.LetterheadData}}" alt="">
  <h1>ใบเสร็จรับเงินบริจาค</h1>
  <p class="field">เลขที่ใบเสร็จ: {{.ReceiptNo}}</p>
  <p class="field">วันที่ออกใบเสร็จ: {{.IssueDate}}</p>
  <p class="field">ได้รับเงินบริจาคจาก: {{.DonorName}}</p>
  <p class="field">จำนวนเงิน: {{.Amount}} บาท</p>
  <div class="section6">{{.Section6Text}}</div>
  <img class="signature" src="{{.SignatureData}}" alt="">
</body>
</html>
$tmpl_th$,
    template_html_en = $tmpl_en$
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<style>
  body { font-family: 'TH Sarabun New', sans-serif; font-size: 16pt; position: relative; }
  .watermark { position: absolute; top: 35%; left: 15%; width: 60%; opacity: 0.12; z-index: -1; }
  .letterhead { width: 100%; margin-bottom: 12px; }
  .signature { height: 70px; display: block; margin-top: 8px; }
  .section6 { margin-top: 24px; font-size: 14pt; white-space: pre-wrap; }
  .field { margin: 4px 0; }
</style>
</head>
<body>
  <img class="watermark" src="{{.WatermarkData}}" alt="">
  <img class="letterhead" src="{{.LetterheadData}}" alt="">
  <h1>Donation Receipt</h1>
  <p class="field">Receipt No.: {{.ReceiptNo}}</p>
  <p class="field">Issue Date: {{.IssueDate}}</p>
  <p class="field">Received from: {{.DonorName}}</p>
  <p class="field">Amount: {{.Amount}} THB</p>
  <div class="section6">{{.Section6Text}}</div>
  <img class="signature" src="{{.SignatureData}}" alt="">
</body>
</html>
$tmpl_en$
WHERE id = true;

-- ============================================================
-- 2. Permissions for donnarec_app role
-- ============================================================

GRANT SELECT, INSERT, UPDATE ON receipt_template_config TO donnarec_app;
