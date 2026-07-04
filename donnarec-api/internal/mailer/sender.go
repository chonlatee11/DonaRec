// Package mailer defines the swappable email-sending seam for receipt delivery.
//
// Design decisions realized here:
//
//	D-60: EmailSender is an interface, not a concrete provider — this phase ships only
//	      a dev/local capture implementation (DevSender); the real provider (SES vs
//	      Postmark) is a stakeholder gate deferred to a later phase. Swapping providers
//	      later means adding a new implementation of this interface, never touching the
//	      worker that calls Send.
//	CLAUDE.md "What NOT to Use": no self-hosted SMTP as a production path. This package
//	      must never import net/smtp or dial an SMTP server directly.
package mailer

import (
	"context"
	"time"
)

// Attachment is a single file attached to an outgoing email (the frozen receipt PDF).
type Attachment struct {
	Filename    string
	ContentType string // e.g. "application/pdf"
	Data        []byte
}

// Message is a single outgoing email, addressed to one donor.
type Message struct {
	To         string
	Subject    string
	BodyHTML   string
	BodyText   string
	Attachment Attachment
}

// SendResult is returned by a successful Send call.
//
// ProviderMessageID is empty for implementations that have no provider-assigned
// message id (e.g. DevSender). SentAt is always set to a non-zero time on success.
type SendResult struct {
	ProviderMessageID string
	SentAt            time.Time
}

// EmailSender is the swappable seam (D-60): the worker depends only on this
// interface, never on a concrete provider. Send must not perform any action that
// mutates outbox/receipt state — the caller is responsible for recording the
// SendResult (or error) in the email_delivery table.
type EmailSender interface {
	Send(ctx context.Context, msg Message) (SendResult, error)
}
