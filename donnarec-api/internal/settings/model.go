// Package settings provides the Admin-only receipt template/compliance config store
// (D-58 full config store + Admin UI, D-59 global 1x/2x deduction, NFR-09 no-deploy config).
//
// Design decisions realized here:
//
//	D-58: template_html/template_html_en are Admin-editable HTML executed later by a
//	      sandboxed headless-Chromium instance (internal/pdf, 04-03) — this package
//	      validates only that the template PARSES (html/template.Parse), never sanitizes
//	      or rewrites it; the render sandbox is the real control (04-RESEARCH.md V5).
//	D-59: deduction_multiplier is a single hospital-wide value (1x/2x) — no per-donation
//	      field in MVP.
//	NFR-09: number-format fields (separator/padding/year-format/prefix, Phase 2
//	      receipt_number_config) are consolidated into this same settings surface per
//	      CONTEXT.md's canonical_refs note, rather than a separate page.
//
// Mirrors internal/donation/model.go's DTO shape (request/response structs with
// go-playground/validator tags).
package settings

import "time"

// ReceiptSettings is the merged Admin-editable receipt configuration: the full
// template/branding config (receipt_template_config) plus the receipt-number format
// config (receipt_number_config, Phase 2), returned by GET and accepted by PUT in one
// request (Screen 6 "save all tabs at once" — no partial/inconsistent config states).
//
// On PUT, UpdatedAt/UpdatedBy in the request body are ignored — the service always sets
// updated_at=now() and updated_by=the resolved acting admin's users.id (never a
// client-supplied value).
type ReceiptSettings struct {
	// Template HTML (D-58) — one per language (FR-23). Validated for parse-ability only
	// (html/template.Parse) before save; the sandboxed renderer is the real control.
	TemplateHTML   string `json:"template_html"   validate:"required"`
	TemplateHTMLEn string `json:"template_html_en" validate:"required"`

	// Tax-deduction wording (FR-24) — text per language; multiplier is global (D-59).
	Section6TextTh      string `json:"section6_text_th"`
	Section6TextEn      string `json:"section6_text_en"`
	DeductionMultiplier string `json:"deduction_multiplier" validate:"required,oneof=1x 2x"`

	// Branding assets (FR-20/21/22) — MinIO object keys, populated via the dedicated
	// image-upload endpoint (never a raw file upload on this struct).
	LetterheadObjectKey *string `json:"letterhead_object_key"`
	SealObjectKey       *string `json:"seal_object_key"`
	SignatureObjectKey  *string `json:"signature_object_key"`
	WatermarkObjectKey  *string `json:"watermark_object_key"`

	// Audit fields (read-only on GET; ignored on PUT — see doc comment above).
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"`

	// Number-format fields (Phase 2 receipt_number_config; consolidated into this same
	// screen/API per CONTEXT.md canonical_refs). Frozen ledger entries (D-42) are never
	// affected by a format change — only the NEXT allocation uses the new format.
	Separator        string `json:"separator" validate:"required"`
	RunningNoPadding int    `json:"running_no_padding" validate:"min=1"`
	YearFormat       string `json:"year_format" validate:"required,oneof=BE4 CE4"`
	Prefix           string `json:"prefix"`
}

// PreviewRequest carries the CURRENT, UNSAVED editor state for the HTML/real-PDF preview
// endpoints (D-61 — preview reflects in-progress edits, not the last-saved config).
// Preview ALWAYS renders against sample/mock data (never live donor PII, D-61 mandate) —
// no donation id or donor field is ever accepted here.
type PreviewRequest struct {
	TemplateHTML        string  `json:"template_html"   validate:"required"`
	TemplateHTMLEn      string  `json:"template_html_en"`
	Section6TextTh      string  `json:"section6_text_th"`
	Section6TextEn      string  `json:"section6_text_en"`
	DeductionMultiplier string  `json:"deduction_multiplier"`
	LetterheadObjectKey *string `json:"letterhead_object_key"`
	SealObjectKey       *string `json:"seal_object_key"`
	SignatureObjectKey  *string `json:"signature_object_key"`
	WatermarkObjectKey  *string `json:"watermark_object_key"`
	// Language selects which template/section6 field and sample fixture (Thai-name vs
	// English-name, 04-UI-SPEC.md "Sample preview data" note) to render. Defaults to "th".
	Language string `json:"language" validate:"omitempty,oneof=th en"`
}
