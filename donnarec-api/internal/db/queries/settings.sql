-- internal/db/queries/settings.sql
-- sqlc queries for Phase 4: receipt template / branding config store
-- (D-58 full config store + admin UI, D-59 global 1x/2x deduction).
-- Single-row table (receipt_template_config, id BOOLEAN PRIMARY KEY DEFAULT
-- true) — same single-row CRUD shape as internal/db/queries/receiptno.sql's
-- GetReceiptNumberConfig (Phase 2).

-- name: GetReceiptTemplateConfig :one
-- Read the single template/branding config row. Called by the worker (04-05,
-- to render receipts) and the settings API (04-07, to populate the editor).
SELECT
    template_html,
    template_html_en,
    section6_text_th,
    section6_text_en,
    deduction_multiplier,
    letterhead_object_key,
    seal_object_key,
    signature_object_key,
    watermark_object_key,
    updated_at,
    updated_by
FROM receipt_template_config
LIMIT 1;

-- name: UpdateReceiptTemplateConfig :exec
-- Update the single config row (Admin-only, D-58). updated_by is set to the
-- acting admin's app-user id for the audit trail (Pattern D, FR-13).
-- Callers MUST validate template_html/template_html_en parse successfully via
-- html/template.Parse BEFORE calling this (surfaces as "Template save failed"
-- per UI-SPEC) — this query does not validate template syntax itself.
UPDATE receipt_template_config
SET
    template_html         = @template_html,
    template_html_en      = @template_html_en,
    section6_text_th      = @section6_text_th,
    section6_text_en      = @section6_text_en,
    deduction_multiplier  = @deduction_multiplier,
    letterhead_object_key = @letterhead_object_key,
    seal_object_key       = @seal_object_key,
    signature_object_key  = @signature_object_key,
    watermark_object_key  = @watermark_object_key,
    updated_at            = now(),
    updated_by            = @updated_by
WHERE id = true;

-- name: UpdateReceiptTemplateContent :exec
-- BW-04 fix (04-REVIEW-PRESHIP.md): the "save all tabs" write path
-- (SaveSettings) updates ONLY the admin-editable text/compliance fields and
-- NEVER the image object keys. Those keys are owned solely by the upload
-- endpoint (UpdateTemplateImageKey) — writing them from the settings PUT body
-- would let a stale/omitted key silently null or revert a freshly-uploaded
-- asset. Deliberately omits letterhead/seal/signature/watermark_object_key so
-- their current DB values are left untouched.
UPDATE receipt_template_config
SET
    template_html         = @template_html,
    template_html_en      = @template_html_en,
    section6_text_th      = @section6_text_th,
    section6_text_en      = @section6_text_en,
    deduction_multiplier  = @deduction_multiplier,
    updated_at            = now(),
    updated_by            = @updated_by
WHERE id = true;

-- name: UpdateTemplateImageKey :exec
-- BW-03 fix (04-REVIEW-PRESHIP.md): persist ONE brand-image slot key with a
-- single atomic per-column write, replacing SaveTemplateImage's former
-- read-whole-row / mutate-one-slot / write-whole-row path (which had no
-- tx/lock and lost a sibling slot's fresh key under concurrent uploads). Each
-- non-target slot column is set to its OWN current value (read in the same
-- statement), so a concurrent write to a different slot is never clobbered.
UPDATE receipt_template_config
SET
    letterhead_object_key = CASE WHEN @slot::text = 'letterhead' THEN @object_key::text ELSE letterhead_object_key END,
    seal_object_key       = CASE WHEN @slot::text = 'seal'       THEN @object_key::text ELSE seal_object_key END,
    signature_object_key  = CASE WHEN @slot::text = 'signature'  THEN @object_key::text ELSE signature_object_key END,
    watermark_object_key  = CASE WHEN @slot::text = 'watermark'  THEN @object_key::text ELSE watermark_object_key END,
    updated_at            = now(),
    updated_by            = @updated_by
WHERE id = true;
