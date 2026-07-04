package settings

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"regexp"

	dbhelpers "github.com/donnarec/donnarec-api/internal/db"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/pdf"
	"github.com/gabriel-vasile/mimetype"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// ErrInvalidTemplate is returned by SaveSettings/BuildPreviewHTML when template_html or
// template_html_en fails to parse via html/template.Parse (04-RESEARCH.md V5 input
// validation: parse-ability only, never sanitized/rewritten — the sandboxed renderer
// (internal/pdf, 04-03) is the real control against admin-supplied-HTML injection).
var ErrInvalidTemplate = errors.New("settings: template failed to parse")

// ErrInvalidNumberFormat is returned when separator/prefix contain characters outside the
// safe allowlist internal/receiptno/format.go's formatReceiptNo enforces at allocation
// time. Rejecting this at SAVE time (rather than only at the next allocation, deep inside
// the issuance transaction) gives the admin an immediate 422 instead of silently breaking
// the next Approve — the exact risk format.go's own doc comment describes.
var ErrInvalidNumberFormat = errors.New("settings: separator/prefix contains disallowed characters")

// ErrInvalidImageSlot is returned when SaveTemplateImage is called with a slot name other
// than the four recognised brand-asset slots.
var ErrInvalidImageSlot = errors.New("settings: unrecognised template image slot")

// numberFormatCharAllowlist mirrors internal/receiptno/format.go's unexported
// configCharAllowlist. That regex is intentionally NOT exported from internal/receiptno
// (Allocate is the single code path that may write a receipt number, and format.go keeps
// its validation private to that path) — this package duplicates the SAME safe-character
// set (alphanumerics, space, "_./-") so a bad separator/prefix is rejected here, at
// save-time, rather than only discovered later at allocation-time.
var numberFormatCharAllowlist = regexp.MustCompile(`^[A-Za-z0-9 _./-]*$`)

// validImageSlots are the four brand-asset slots a receipt template references
// (FR-20/21/22, D-58).
var validImageSlots = map[string]bool{
	"letterhead": true,
	"seal":       true,
	"signature":  true,
	"watermark":  true,
}

// ReceiptsStore is the subset of *storage.StorageClient SettingsService depends on —
// declared as an interface (not the concrete type) so tests can supply a hermetic fake,
// mirroring internal/donation's ReceiptsStore seam (plan 04-06) for DownloadReceipt.
//
// In production this is the SAME receipts-bucket-bound *storage.StorageClient the outbox
// worker (04-05) reads frozen PDFs and branding images from (04-05-SUMMARY.md decision:
// "template branding assets fetched via the same receipts bucket/ReceiptsStore as frozen
// PDFs") — an object key written by PutTemplateImage here is immediately readable by the
// worker's GetObject calls.
type ReceiptsStore interface {
	// GetObject reads the full bytes of a previously-uploaded brand-image object.
	GetObject(ctx context.Context, objectKey string) ([]byte, error)
	// PutTemplateImage validates (magic-byte + size cap) and uploads a brand-image file,
	// returning the generated object key.
	PutTemplateImage(ctx context.Context, r io.Reader, size int64, slot string) (string, string, error)
}

// SettingsService reads/writes the merged receipt template + number-format config
// (D-58/D-59, Phase 2 receipt_number_config) and builds sample-data preview HTML (D-61).
// Mirrors internal/receiptno/allocator.go's config-row read/format-service shape.
type SettingsService struct {
	pool          *pgxpool.Pool
	queries       *db.Queries
	receiptsStore ReceiptsStore
	logger        *zap.Logger
}

// NewSettingsService constructs a SettingsService with the given dependencies.
// Panics if queries is nil — a programming-error guard, mirroring
// receiptno.NewAllocator's constructor style. receiptsStore may be nil for callers that
// only need GetSettings/SaveSettings (which never touch it) — SaveTemplateImage and
// BuildPreviewHTML (when an image object key is set) require a non-nil receiptsStore.
// pool may be nil for tests that only exercise the pre-DB-write validation paths of
// SaveSettings/SaveTemplateImage (mirrors donation.NewDonationService's constructor
// style) — SaveSettings's transactional write (WR-07, 04-REVIEW.md) requires a non-nil
// pool.
func NewSettingsService(pool *pgxpool.Pool, queries *db.Queries, receiptsStore ReceiptsStore, logger *zap.Logger) *SettingsService {
	if queries == nil {
		panic("settings.NewSettingsService: queries must not be nil")
	}
	return &SettingsService{pool: pool, queries: queries, receiptsStore: receiptsStore, logger: logger}
}

// GetSettings returns the merged config: the template/branding config joined with the
// receipt-number format config (Phase 2), as one ReceiptSettings DTO (CONTEXT.md
// canonical_refs: consolidate number-format editing into this same settings screen).
func (s *SettingsService) GetSettings(ctx context.Context) (ReceiptSettings, error) {
	tmpl, err := s.queries.GetReceiptTemplateConfig(ctx)
	if err != nil {
		return ReceiptSettings{}, fmt.Errorf("settings: get template config: %w", err)
	}
	num, err := s.queries.GetReceiptNumberConfig(ctx)
	if err != nil {
		return ReceiptSettings{}, fmt.Errorf("settings: get number format config: %w", err)
	}

	return ReceiptSettings{
		TemplateHTML:        tmpl.TemplateHtml,
		TemplateHTMLEn:      tmpl.TemplateHtmlEn,
		Section6TextTh:      tmpl.Section6TextTh,
		Section6TextEn:      tmpl.Section6TextEn,
		DeductionMultiplier: tmpl.DeductionMultiplier,
		LetterheadObjectKey: tmpl.LetterheadObjectKey,
		SealObjectKey:       tmpl.SealObjectKey,
		SignatureObjectKey:  tmpl.SignatureObjectKey,
		WatermarkObjectKey:  tmpl.WatermarkObjectKey,
		UpdatedAt:           tmpl.UpdatedAt.Time,
		UpdatedBy:           tmpl.UpdatedBy.String(),

		Separator:        num.Separator,
		RunningNoPadding: int(num.RunningNoPadding),
		YearFormat:       num.YearFormat,
		Prefix:           num.Prefix,
	}, nil
}

// SaveSettings validates then persists ALL settings fields in one call — both
// receipt_template_config and receipt_number_config — Admin-gated at the handler/route
// level (D-58) and audited by the caller (Pattern D, AuditMiddleware). updatedBy MUST be
// the acting admin's resolved users.id (auth.ResolveAppUser), never the raw Keycloak
// subject.
//
// Validation order (BEFORE any DB write, so a rejected save leaves both config rows
// untouched — no partial save):
//  1. Template parse-ability (both languages) — ErrInvalidTemplate
//  2. Number-format character allowlist (separator/prefix) — ErrInvalidNumberFormat
func (s *SettingsService) SaveSettings(ctx context.Context, input ReceiptSettings, updatedBy pgtype.UUID) error {
	if _, err := template.New("receipt_th").Parse(input.TemplateHTML); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTemplate, err)
	}
	if _, err := template.New("receipt_en").Parse(input.TemplateHTMLEn); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTemplate, err)
	}

	if !numberFormatCharAllowlist.MatchString(input.Separator) || !numberFormatCharAllowlist.MatchString(input.Prefix) {
		return ErrInvalidNumberFormat
	}

	// WR-07 fix (04-REVIEW.md): both writes share ONE transaction (Pattern B,
	// dbhelpers.WithTx — the same helper every other service's atomic mutation
	// uses, e.g. internal/donation/service.go) so a failure on either write
	// rolls back BOTH — no partial save, matching what this method's own doc
	// comment already promised.
	return dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.queries.WithTx(tx)

		if err := qtx.UpdateReceiptTemplateConfig(ctx, db.UpdateReceiptTemplateConfigParams{
			TemplateHtml:        input.TemplateHTML,
			TemplateHtmlEn:      input.TemplateHTMLEn,
			Section6TextTh:      input.Section6TextTh,
			Section6TextEn:      input.Section6TextEn,
			DeductionMultiplier: input.DeductionMultiplier,
			LetterheadObjectKey: input.LetterheadObjectKey,
			SealObjectKey:       input.SealObjectKey,
			SignatureObjectKey:  input.SignatureObjectKey,
			WatermarkObjectKey:  input.WatermarkObjectKey,
			UpdatedBy:           updatedBy,
		}); err != nil {
			return fmt.Errorf("settings: update template config: %w", err)
		}

		if err := qtx.UpdateReceiptNumberConfig(ctx, db.UpdateReceiptNumberConfigParams{
			Separator:        input.Separator,
			RunningNoPadding: int32(input.RunningNoPadding),
			YearFormat:       input.YearFormat,
			Prefix:           input.Prefix,
			UpdatedBy:        updatedBy,
		}); err != nil {
			return fmt.Errorf("settings: update number format config: %w", err)
		}

		return nil
	})
}

// SaveTemplateImage validates (magic-byte + 2 MB cap, via receiptsStore.PutTemplateImage)
// and uploads a brand-image file for the given slot, then persists the resulting object
// key onto receipt_template_config immediately — Screen 6's ImageUploadSlot uploads and
// reflects the new thumbnail right away, independent of the "save all tabs" button for the
// other fields (04-UI-SPEC.md Screen 6). Returns the new object key.
//
// slot is validated BEFORE any upload/DB work — an unrecognised slot returns
// ErrInvalidImageSlot without touching receiptsStore or the DB.
func (s *SettingsService) SaveTemplateImage(ctx context.Context, slot string, r io.Reader, size int64, updatedBy pgtype.UUID) (string, error) {
	if !validImageSlots[slot] {
		return "", ErrInvalidImageSlot
	}

	objectKey, _, err := s.receiptsStore.PutTemplateImage(ctx, r, size, slot)
	if err != nil {
		return "", err
	}

	current, err := s.queries.GetReceiptTemplateConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("settings: get template config for image update: %w", err)
	}

	params := db.UpdateReceiptTemplateConfigParams{
		TemplateHtml:        current.TemplateHtml,
		TemplateHtmlEn:      current.TemplateHtmlEn,
		Section6TextTh:      current.Section6TextTh,
		Section6TextEn:      current.Section6TextEn,
		DeductionMultiplier: current.DeductionMultiplier,
		LetterheadObjectKey: current.LetterheadObjectKey,
		SealObjectKey:       current.SealObjectKey,
		SignatureObjectKey:  current.SignatureObjectKey,
		WatermarkObjectKey:  current.WatermarkObjectKey,
		UpdatedBy:           updatedBy,
	}
	switch slot {
	case "letterhead":
		params.LetterheadObjectKey = &objectKey
	case "seal":
		params.SealObjectKey = &objectKey
	case "signature":
		params.SignatureObjectKey = &objectKey
	case "watermark":
		params.WatermarkObjectKey = &objectKey
	}

	if err := s.queries.UpdateReceiptTemplateConfig(ctx, params); err != nil {
		return "", fmt.Errorf("settings: persist template image key: %w", err)
	}

	return objectKey, nil
}

// BuildPreviewHTML assembles the admin's CURRENT, UNSAVED template + sample/mock donation
// data (D-61 mandate: never live donor PII) into a complete HTML document via
// pdf.Render — the SAME contextual-autoescaping code path (internal/pdf, 04-03) production
// rendering uses, so a preview that renders safely proves the saved template will too
// (T-04-20).
//
// language selects the template/section6 field and sample fixture; defaults to "th".
func (s *SettingsService) BuildPreviewHTML(ctx context.Context, req PreviewRequest) (string, error) {
	language := req.Language
	if language == "" {
		language = "th"
	}

	templateHTML := req.TemplateHTML
	section6 := req.Section6TextTh
	if language == "en" {
		templateHTML = req.TemplateHTMLEn
		section6 = req.Section6TextEn
	}

	data := sampleReceiptData(language)
	data.Section6Text = section6
	data.DeductionMultiplier = req.DeductionMultiplier

	var err error
	if data.LetterheadData, err = s.fetchTemplateImage(ctx, req.LetterheadObjectKey); err != nil {
		return "", err
	}
	if data.SealData, err = s.fetchTemplateImage(ctx, req.SealObjectKey); err != nil {
		return "", err
	}
	if data.SignatureData, err = s.fetchTemplateImage(ctx, req.SignatureObjectKey); err != nil {
		return "", err
	}
	if data.WatermarkData, err = s.fetchTemplateImage(ctx, req.WatermarkObjectKey); err != nil {
		return "", err
	}

	html, err := pdf.Render(templateHTML, data)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidTemplate, err)
	}
	return html, nil
}

// fetchTemplateImage returns a base64 data: URI for the given object key, or an empty
// template.URL if key is nil/empty (no admin-uploaded asset yet). Mirrors
// internal/worker/issue_receipt.go's fetchTemplateImage exactly — same Go-fetches-bytes,
// Chromium-never-fetches rationale (04-RESEARCH.md Pitfall 3).
func (s *SettingsService) fetchTemplateImage(ctx context.Context, objectKey *string) (template.URL, error) {
	if objectKey == nil || *objectKey == "" {
		return "", nil
	}
	data, err := s.receiptsStore.GetObject(ctx, *objectKey)
	if err != nil {
		return "", fmt.Errorf("settings: fetch template image %q: %w", *objectKey, err)
	}
	mimeType := mimetype.Detect(data).String()
	return pdf.DataURI(mimeType, data), nil
}

// sampleReceiptData returns the fixture ReceiptData used by BuildPreviewHTML (D-61
// mandate: sample/mock data only, never live donor PII). Two fixtures — Thai-name and
// English-name — so BOTH donor_language branches can be sanity-checked
// (04-UI-SPEC.md "Sample preview data" note).
func sampleReceiptData(language string) pdf.ReceiptData {
	if language == "en" {
		return pdf.ReceiptData{
			DonorName: "Jane Sample Donor",
			ReceiptNo: "2569/000001",
			Amount:    "1,500.00",
			IssueDate: "15 Mar 2026",
			Language:  "en",
		}
	}
	return pdf.ReceiptData{
		DonorName: "นาย ตัวอย่าง ใจบุญ",
		ReceiptNo: "2569/000001",
		Amount:    "1,500.00",
		IssueDate: "15 มี.ค. 2569",
		Language:  "th",
	}
}
