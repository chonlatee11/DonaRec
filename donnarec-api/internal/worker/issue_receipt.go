// Package worker — issue_receipt.go implements the job_type="issue_receipt"
// handler: render the donation's receipt PDF exactly once, freeze it to
// object storage, email it to the donor bilingually, and record the delivery
// attempt.
//
// Design decisions realized here:
//
//	D-56 (freeze, T-04-14): if donations.receipt_pdf_object_key is already
//	      set, the stored PDF bytes are reused verbatim — the renderer is
//	      NEVER invoked again for an already-frozen receipt, even on a
//	      staff-triggered resend (04-06) or an auto-retried email send.
//	D-55: donor_language (frozen at donation-create time) selects both the
//	      HTML template (template_html vs template_html_en) and the go-i18n
//	      locale used for the email subject/body.
//	D-59: deduction_multiplier is a single hospital-wide config value, read
//	      fresh from receipt_template_config at render time (only relevant
//	      the FIRST time a receipt renders — frozen thereafter, same as the
//	      template HTML itself).
//	FR-28: a donor with no email on file gets an 'no_email' email_delivery
//	      record (not an error) — staff can still download the PDF manually.
//	Pattern C (no PII in logs): only donation_id / job_id / operation name are
//	      ever logged from this file — never donor name/email/tax id, the
//	      rendered HTML, or the PDF bytes.
//
// KNOWN LIMITATION (WR-03, 04-REVIEW.md — deliberately NOT fixed this pass):
//
//	deduction_multiplier/section6_text/template_html are read from
//	receipt_template_config at FIRST-render time (renderReceiptPDF, below),
//	not snapshotted at Approve time (internal/donation/service.go). If an
//	admin edits these values in the narrow window between Approve
//	committing (which enqueues the outbox job) and the worker actually
//	processing it (normally one poll interval, ~5s — but longer if the job
//	sits behind a backlog or a stuck/reclaimed job, CR-01), the rendered
//	receipt will reflect the NEW config, not what was in effect when the
//	checker approved — unlike every other receipt field (receipt number,
//	donor snapshot), which IS frozen at approval.
//
//	The safe fix (snapshotting deduction_multiplier — at minimum — onto the
//	donations row or the outbox job payload) requires touching
//	internal/donation/service.go's Approve method: the single most
//	load-bearing, security/compliance-critical transaction in the codebase
//	(D-52 — gap-less receipt numbering + SoD + audit, all inside one
//	pg_advisory-locked tx). Adding a new column + sqlc params + a
//	config-table read to that path is exactly the kind of change CLAUDE.md
//	warns must be reviewed with extreme care, and doing it as a drive-by
//	code-review fix (vs. a properly planned/tested phase) risks a
//	regression in the one invariant this whole system cannot get wrong.
//	Deferred to a future phase with its own plan + tests against Approve,
//	rather than scope-creeping this fix pass into that transaction.
//
//	Mitigating factor: this is an ADMIN-configuration-change race, not a
//	donor/staff-triggered one — an admin editing the deduction multiplier
//	or receipt template seconds after a checker approves a donation is a
//	narrow, low-frequency operational scenario, not a per-request path.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/mailer"
	"github.com/donnarec/donnarec-api/internal/pdf"
	"github.com/donnarec/donnarec-api/internal/receiptfmt"
	"github.com/gabriel-vasile/mimetype"
	gogoi18n "github.com/nicksnyder/go-i18n/v2/i18n"

	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

// issueReceiptPayload is the JSON shape enqueued by the Phase 3 issuance
// transaction (internal/donation/service.go Approve Step 7,
// EnqueueOutboxJobParams{JobType: "issue_receipt", Payload: {"donation_id": "..."}})
type issueReceiptPayload struct {
	DonationID string `json:"donation_id"`
}

// handleIssueReceipt renders (or reuses the frozen) receipt PDF for one
// donation, emails it to the donor (or records 'no_email'), and returns an
// error IFF the job should be retried/dead-lettered (ProcessOnce records
// this via MarkOutboxJobFailed). A donor-has-no-email outcome is NOT an
// error — it is a valid terminal state recorded as email_delivery
// status='no_email' (FR-28).
func (w *Worker) handleIssueReceipt(ctx context.Context, job db.ClaimNextOutboxJobRow) error {
	var payload issueReceiptPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal issue_receipt payload: %w", err)
	}

	var donationID pgtype.UUID
	if err := donationID.Scan(payload.DonationID); err != nil {
		return fmt.Errorf("parse donation_id: %w", err)
	}

	donation, err := w.queries.GetDonationByID(ctx, donationID)
	if err != nil {
		return fmt.Errorf("get donation: %w", err)
	}

	pdfBytes, err := w.getOrRenderReceiptPDF(ctx, donation)
	if err != nil {
		return fmt.Errorf("get or render receipt pdf: %w", err)
	}

	if donation.DonorEmail == nil || strings.TrimSpace(*donation.DonorEmail) == "" {
		if _, err := w.queries.InsertEmailDelivery(ctx, db.InsertEmailDeliveryParams{
			DonationID:        donationID,
			SentTo:            nil,
			Status:            "no_email",
			ProviderMessageID: nil,
			Attempts:          job.Attempts + 1,
			LastError:         nil,
		}); err != nil {
			return fmt.Errorf("record no_email delivery: %w", err)
		}
		return nil
	}

	msg, err := w.composeReceiptEmail(donation, pdfBytes)
	if err != nil {
		return fmt.Errorf("compose receipt email: %w", err)
	}

	result, sendErr := w.sender.Send(ctx, msg)
	if sendErr != nil {
		errMsg := sendErr.Error()
		if _, insErr := w.queries.InsertEmailDelivery(ctx, db.InsertEmailDeliveryParams{
			DonationID:        donationID,
			SentTo:            donation.DonorEmail,
			Status:            "failed",
			ProviderMessageID: nil,
			Attempts:          job.Attempts + 1,
			LastError:         &errMsg,
		}); insErr != nil {
			w.logger.Error("worker: record failed email_delivery",
				zap.String("operation", "InsertEmailDelivery"),
				zap.Int64("job_id", job.ID),
				zap.Error(insErr),
			)
		}
		return fmt.Errorf("send email: %w", sendErr)
	}

	var providerMsgID *string
	if result.ProviderMessageID != "" {
		providerMsgID = &result.ProviderMessageID
	}
	if _, err := w.queries.InsertEmailDelivery(ctx, db.InsertEmailDeliveryParams{
		DonationID:        donationID,
		SentTo:            donation.DonorEmail,
		Status:            "sent",
		ProviderMessageID: providerMsgID,
		Attempts:          job.Attempts + 1,
		LastError:         nil,
	}); err != nil {
		return fmt.Errorf("record sent delivery: %w", err)
	}

	return nil
}

// getOrRenderReceiptPDF returns the donation's frozen receipt PDF bytes,
// rendering (and freezing) them for the first time if
// donation.ReceiptPdfObjectKey is not yet set (D-56, T-04-14).
func (w *Worker) getOrRenderReceiptPDF(ctx context.Context, donation db.Donation) ([]byte, error) {
	if donation.ReceiptPdfObjectKey != nil {
		data, err := w.receiptsStore.GetObject(ctx, *donation.ReceiptPdfObjectKey)
		if err != nil {
			return nil, fmt.Errorf("load frozen receipt pdf %q: %w", *donation.ReceiptPdfObjectKey, err)
		}
		return data, nil
	}

	pdfBytes, err := w.renderReceiptPDF(ctx, donation)
	if err != nil {
		return nil, fmt.Errorf("render receipt pdf: %w", err)
	}

	objectKey := fmt.Sprintf("receipts/%s.pdf", donation.ID.String())
	if err := w.receiptsStore.PutObject(ctx, objectKey, pdfBytes, "application/pdf"); err != nil {
		return nil, fmt.Errorf("store receipt pdf: %w", err)
	}

	if err := w.queries.SetReceiptPDFObjectKey(ctx, db.SetReceiptPDFObjectKeyParams{
		ReceiptPdfObjectKey: &objectKey,
		ID:                  donation.ID,
	}); err != nil {
		return nil, fmt.Errorf("set receipt_pdf_object_key: %w", err)
	}

	return pdfBytes, nil
}

// renderReceiptPDF assembles ReceiptData from receipt_template_config +
// the donation snapshot and renders it through the sandboxed Chromium
// pipeline (internal/pdf, 04-03). Called exactly once per donation — the
// caller (getOrRenderReceiptPDF) only invokes this when
// receipt_pdf_object_key is still nil.
//
// KNOWN LIMITATION (WR-03 — see package doc comment above for full
// reasoning): tplCfg (including deduction_multiplier/section6 text/template
// HTML) is read fresh here, NOT snapshotted at Approve time — an admin
// config edit in the window between approval and first-render will affect
// the rendered receipt.
func (w *Worker) renderReceiptPDF(ctx context.Context, donation db.Donation) ([]byte, error) {
	tplCfg, err := w.queries.GetReceiptTemplateConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get receipt template config: %w", err)
	}

	templateHTML := tplCfg.TemplateHtml
	section6 := tplCfg.Section6TextTh
	if donation.DonorLanguage == "en" {
		templateHTML = tplCfg.TemplateHtmlEn
		section6 = tplCfg.Section6TextEn
	}

	// WR-06 fix (04-REVIEW.md): fetch branding images via the SOFT variant —
	// letterhead/seal/signature/watermark are decorative, not legally-required
	// receipt content (donor name/amount/receipt number/section6 text always
	// come from the donation row/config text fields, never from these image
	// fetches), so a transient object-storage blip on any ONE of them must not
	// fail the whole render (and burn a D-57 retry/backoff attempt) when the
	// legally-required content was ready to go.
	letterhead := w.fetchTemplateImageSoft(ctx, tplCfg.LetterheadObjectKey, "letterhead")
	seal := w.fetchTemplateImageSoft(ctx, tplCfg.SealObjectKey, "seal")
	signature := w.fetchTemplateImageSoft(ctx, tplCfg.SignatureObjectKey, "signature")
	watermark := w.fetchTemplateImageSoft(ctx, tplCfg.WatermarkObjectKey, "watermark")

	receiptNo := ""
	if donation.ReceiptFormatted != nil {
		receiptNo = *donation.ReceiptFormatted
	}

	data := pdf.ReceiptData{
		DonorName:           donation.DonorName,
		ReceiptNo:           receiptNo,
		Amount:              formatAmount(donation.Amount),
		IssueDate:           receiptfmt.FormatIssueDate(donation.ApprovedAt, donation.DonorLanguage),
		Section6Text:        section6,
		DeductionMultiplier: tplCfg.DeductionMultiplier,
		Language:            donation.DonorLanguage,
		LetterheadData:      letterhead,
		SealData:            seal,
		SignatureData:       signature,
		WatermarkData:       watermark,
		// FontFaceCSS intentionally left empty: TH Sarabun New is not yet
		// sourced (assets/fonts/README.md, 04-03 Assumption A3) — the render
		// pipeline falls back to fonts-thai-tlwg (Waree) already baked into
		// docker/chrome.Dockerfile, same as every other current render path.
		FontFaceCSS: "",
	}

	html, err := pdf.Render(templateHTML, data)
	if err != nil {
		return nil, fmt.Errorf("render template html: %w", err)
	}

	pdfBytes, err := w.renderer.RenderPDF(ctx, html)
	if err != nil {
		return nil, fmt.Errorf("render pdf via chromium: %w", err)
	}

	return pdfBytes, nil
}

// fetchTemplateImage returns a base64 data: URI for the given
// receipt_template_config object key, or an empty template.URL if the key is
// unset (no admin-uploaded asset yet — the seeded 04-01 template tolerates an
// empty img src). The Go app fetches the bytes itself (network access it
// legitimately has); Chromium never fetches them (04-RESEARCH Pitfall 3).
func (w *Worker) fetchTemplateImage(ctx context.Context, objectKey *string) (template.URL, error) {
	if objectKey == nil || *objectKey == "" {
		return "", nil
	}
	data, err := w.receiptsStore.GetObject(ctx, *objectKey)
	if err != nil {
		return "", fmt.Errorf("fetch template image %q: %w", *objectKey, err)
	}
	mimeType := mimetype.Detect(data).String()
	return pdf.DataURI(mimeType, data), nil
}

// fetchTemplateImageSoft is fetchTemplateImage's fail-OPEN variant for the
// four decorative branding slots (letterhead/seal/signature/watermark) —
// WR-06, 04-REVIEW.md. None of them are legally-required receipt content, so
// a transient object-storage error fetching one is logged (Pattern C:
// operation + image slot name only, never PII) and treated exactly like an
// unset object key ("render without this image") rather than failing the
// entire render and burning a D-57 retry/backoff attempt. An unset
// (nil/empty) objectKey is the normal steady-state case, not a failure — it
// is never logged.
func (w *Worker) fetchTemplateImageSoft(ctx context.Context, objectKey *string, imageName string) template.URL {
	uri, err := w.fetchTemplateImage(ctx, objectKey)
	if err != nil {
		w.logger.Warn("worker: non-critical branding image fetch failed — rendering without it",
			zap.String("operation", "fetchTemplateImage"),
			zap.String("image", imageName),
			zap.Error(err),
		)
		return ""
	}
	return uri
}

// composeReceiptEmail builds the bilingual (donor_language-selected) email
// message with the frozen receipt PDF attached (FR-25/26).
func (w *Worker) composeReceiptEmail(donation db.Donation, pdfBytes []byte) (mailer.Message, error) {
	localizer := gogoi18n.NewLocalizer(w.bundle, donation.DonorLanguage)

	subject, err := localizer.Localize(&gogoi18n.LocalizeConfig{MessageID: "email.subject"})
	if err != nil {
		return mailer.Message{}, fmt.Errorf("localize email.subject: %w", err)
	}
	greeting, err := localizer.Localize(&gogoi18n.LocalizeConfig{MessageID: "email.greeting"})
	if err != nil {
		return mailer.Message{}, fmt.Errorf("localize email.greeting: %w", err)
	}
	body, err := localizer.Localize(&gogoi18n.LocalizeConfig{MessageID: "email.body"})
	if err != nil {
		return mailer.Message{}, fmt.Errorf("localize email.body: %w", err)
	}
	footer, err := localizer.Localize(&gogoi18n.LocalizeConfig{MessageID: "email.footer"})
	if err != nil {
		return mailer.Message{}, fmt.Errorf("localize email.footer: %w", err)
	}

	bodyHTML := fmt.Sprintf(
		"<p>%s</p><p>%s</p><p>%s</p>",
		template.HTMLEscapeString(greeting),
		template.HTMLEscapeString(body),
		template.HTMLEscapeString(footer),
	)
	bodyText := fmt.Sprintf("%s\n\n%s\n\n%s", greeting, body, footer)

	receiptNo := "receipt"
	if donation.ReceiptFormatted != nil && *donation.ReceiptFormatted != "" {
		receiptNo = *donation.ReceiptFormatted
	}

	return mailer.Message{
		To:       *donation.DonorEmail,
		Subject:  subject,
		BodyHTML: bodyHTML,
		BodyText: bodyText,
		Attachment: mailer.Attachment{
			Filename:    sanitizeFilename(receiptNo) + ".pdf",
			ContentType: "application/pdf",
			Data:        pdfBytes,
		},
	}, nil
}

// sanitizeFilename replaces path-separator-like characters in a receipt
// number (e.g. "2569/000001") so it is safe to use as a single filename
// component across filesystems (DevSender writes attachments to disk).
func sanitizeFilename(s string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "_")
	return replacer.Replace(s)
}

// formatAmount converts a pgtype.Numeric (big.Int mantissa + Exp) to a plain
// decimal string, mirroring internal/donation/service.go's unexported
// numericStr helper (duplicated here rather than exported cross-package,
// since it is a small, self-contained pure function with no other shared
// state).
func formatAmount(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil {
		return "0"
	}
	intStr := n.Int.Text(10)
	negative := strings.HasPrefix(intStr, "-")
	if negative {
		intStr = intStr[1:]
	}

	var result string
	if n.Exp >= 0 {
		result = intStr + strings.Repeat("0", int(n.Exp))
	} else {
		decPlaces := int(-n.Exp)
		for len(intStr) <= decPlaces {
			intStr = "0" + intStr
		}
		pos := len(intStr) - decPlaces
		result = intStr[:pos] + "." + intStr[pos:]
	}

	if negative {
		result = "-" + result
	}
	return result
}
