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

	letterhead, err := w.fetchTemplateImage(ctx, tplCfg.LetterheadObjectKey)
	if err != nil {
		return nil, err
	}
	seal, err := w.fetchTemplateImage(ctx, tplCfg.SealObjectKey)
	if err != nil {
		return nil, err
	}
	signature, err := w.fetchTemplateImage(ctx, tplCfg.SignatureObjectKey)
	if err != nil {
		return nil, err
	}
	watermark, err := w.fetchTemplateImage(ctx, tplCfg.WatermarkObjectKey)
	if err != nil {
		return nil, err
	}

	receiptNo := ""
	if donation.ReceiptFormatted != nil {
		receiptNo = *donation.ReceiptFormatted
	}

	data := pdf.ReceiptData{
		DonorName:           donation.DonorName,
		ReceiptNo:           receiptNo,
		Amount:              formatAmount(donation.Amount),
		IssueDate:           formatIssueDate(donation.ApprovedAt),
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

// formatIssueDate formats the donation's approved_at timestamp (the receipt
// issue date) as YYYY-MM-DD. Returns "" if unset (should not happen for a
// job the worker is asked to process, since only issued donations enqueue
// this job type — defensive rather than assumed).
func formatIssueDate(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.Format("2006-01-02")
}
