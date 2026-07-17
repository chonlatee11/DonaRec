// Package worker — ack_email.go implements the job_type="ack_email" handler:
// send the donor a bilingual acknowledgement that their public submission was
// received and is NOT yet a receipt, carrying their donor-facing reference
// number (Phase 6, plan 06-04; FR-05/FR-06, D-84/D-85).
//
// The job is enqueued atomically inside plan 03's CreatePublicSubmission
// transaction (internal/donation/service.go CreatePublicSubmission →
// EnqueueOutboxJob{JobType: "ack_email", Payload: {"donation_id": "..."}}), so
// it runs entirely OFF the submit request path (NFR-07): a send failure
// retries/backs off via ProcessOnce's uniform MarkOutboxJobFailed path (D-57)
// and NEVER rolls back the already-committed submission (T-06-16).
//
// Design decisions realized here:
//
//	D-84 (T-06-15): the ack email carries the donor-facing REFERENCE number
//	      (donation.PublicReferenceNumber — derived from the donation id), NEVER
//	      a receipt number. No receipt number exists pre-approval; issuing one
//	      here would misrepresent the submission's status.
//	D-85/FR-05: the copy explicitly states the submission was received and is
//	      NOT yet a receipt (the non-negotiable ackEmail.body wording), matching
//	      06-UI-SPEC's Ack Email Template Copy.
//	FR-06/D-55: donor_language (frozen at submission time) selects the go-i18n
//	      locale for subject/body — 'en' → English catalog, otherwise Thai.
//	FR-28-parity: a donor with no email on file is an expected TERMINAL state
//	      (the job completes 'done'), not an error and not a retry loop —
//	      mirrors issue_receipt's no-email handling. Unlike issue_receipt this
//	      handler records NO email_delivery row: that table's status CHECK and
//	      the staff resend UI (04-06) are scoped to RECEIPT deliveries, and an
//	      ack is not a receipt delivery (keeping the two flows uncoupled).
//	Pattern C (no PII in logs): only donation_id / job_id / operation name are
//	      ever logged from this file — never donor name/email/tax id or body.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/donation"
	"github.com/donnarec/donnarec-api/internal/mailer"
	gogoi18n "github.com/nicksnyder/go-i18n/v2/i18n"

	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

// monoFontStack renders the reference number in the ack email HTML using a
// system-monospace fallback stack. Email-client web-font support is unreliable,
// so — matching the Phase 4 receipt email and 06-UI-SPEC's guidance — we do NOT
// depend on IBM Plex Mono loading; a system mono stack approximates the
// on-screen confirmation's monospaced reference-number convention.
const monoFontStack = "ui-monospace, 'SFMono-Regular', 'Menlo', 'Consolas', 'Liberation Mono', monospace"

// ackEmailPayload is the JSON shape enqueued by plan 03's CreatePublicSubmission
// (EnqueueOutboxJobParams{JobType: "ack_email", Payload: {"donation_id": "..."}}).
type ackEmailPayload struct {
	DonationID string `json:"donation_id"`
}

// handleAckEmail sends the donor a bilingual "received, not yet a receipt"
// acknowledgement for one public (flow_b) submission, or — when the donor left
// no email — completes as a terminal no-op. It returns an error IFF the job
// should be retried/dead-lettered (ProcessOnce records this via
// MarkOutboxJobFailed with backoff, D-57). A donor-has-no-email outcome is NOT
// an error: it is a valid terminal state (the job is marked done), mirroring
// issue_receipt's no-email contract.
func (w *Worker) handleAckEmail(ctx context.Context, job db.ClaimNextOutboxJobRow) error {
	var payload ackEmailPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal ack_email payload: %w", err)
	}

	var donationID pgtype.UUID
	if err := donationID.Scan(payload.DonationID); err != nil {
		return fmt.Errorf("parse donation_id: %w", err)
	}

	don, err := w.queries.GetDonationByID(ctx, donationID)
	if err != nil {
		return fmt.Errorf("get donation: %w", err)
	}

	if don.DonorEmail == nil || strings.TrimSpace(*don.DonorEmail) == "" {
		// Terminal no-op (FR-28-parity): a public donor who supplied no email
		// simply gets no ack — this is expected, not a failure to retry. Logged
		// with donation_id + job_id only (Pattern C).
		w.logger.Info("worker: ack_email skipped — donor has no email on file",
			zap.String("operation", "ack_email"),
			zap.Int64("job_id", job.ID),
			zap.String("donation_id", payload.DonationID),
		)
		return nil
	}

	msg, err := w.composeAckEmail(don, payload.DonationID)
	if err != nil {
		return fmt.Errorf("compose ack email: %w", err)
	}

	if _, err := w.sender.Send(ctx, msg); err != nil {
		return fmt.Errorf("send ack email: %w", err)
	}

	return nil
}

// composeAckEmail builds the bilingual (donor_language-selected) ack message.
// The reference number is derived via donation.PublicReferenceNumber (D-84 —
// NEVER a receipt number) and rendered in a monospace fallback stack in the
// HTML body. The message carries no attachment (an ack is not a receipt).
func (w *Worker) composeAckEmail(don db.Donation, donationID string) (mailer.Message, error) {
	localizer := gogoi18n.NewLocalizer(w.bundle, don.DonorLanguage)

	localize := func(id string) (string, error) {
		s, err := localizer.Localize(&gogoi18n.LocalizeConfig{MessageID: id})
		if err != nil {
			return "", fmt.Errorf("localize %s: %w", id, err)
		}
		return s, nil
	}

	subject, err := localize("ackEmail.subject")
	if err != nil {
		return mailer.Message{}, err
	}
	greeting, err := localize("ackEmail.greeting")
	if err != nil {
		return mailer.Message{}, err
	}
	body, err := localize("ackEmail.body")
	if err != nil {
		return mailer.Message{}, err
	}
	refLabel, err := localize("ackEmail.reference_label")
	if err != nil {
		return mailer.Message{}, err
	}
	footer, err := localize("ackEmail.footer")
	if err != nil {
		return mailer.Message{}, err
	}

	referenceNumber := donation.PublicReferenceNumber(donationID)

	bodyHTML := fmt.Sprintf(
		"<p>%s</p><p>%s</p><p>%s: <span style=\"font-family:%s\">%s</span></p><p>%s</p>",
		template.HTMLEscapeString(greeting),
		template.HTMLEscapeString(body),
		template.HTMLEscapeString(refLabel),
		monoFontStack,
		template.HTMLEscapeString(referenceNumber),
		template.HTMLEscapeString(footer),
	)
	bodyText := fmt.Sprintf("%s\n\n%s\n\n%s: %s\n\n%s", greeting, body, refLabel, referenceNumber, footer)

	return mailer.Message{
		To:       *don.DonorEmail,
		Subject:  subject,
		BodyHTML: bodyHTML,
		BodyText: bodyText,
	}, nil
}
