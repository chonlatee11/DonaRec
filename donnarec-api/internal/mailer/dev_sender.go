package mailer

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// DevSender is the dev/local capture EmailSender (D-60). It performs NO network
// call and imports no SMTP/provider SDK — it writes the message body and
// attachment to a per-message directory under OutDir so a developer/QA can open
// the files and visually confirm content. This is explicitly not a production
// path (CLAUDE.md forbids self-hosted SMTP in production); a real provider
// (SES/Postmark) plugs in later as a separate implementation of EmailSender.
type DevSender struct {
	// OutDir is the base directory under which each captured message gets its
	// own uuid-named subdirectory.
	OutDir string
}

// Send writes msg.BodyHTML to body.html and msg.Attachment.Data to
// msg.Attachment.Filename, both under a new per-message directory inside
// d.OutDir. Returns a SendResult with SentAt set and ProviderMessageID empty
// (DevSender has no provider to assign one).
func (d *DevSender) Send(ctx context.Context, msg Message) (SendResult, error) {
	dir := filepath.Join(d.OutDir, uuid.NewString())
	// BI-01 fix (04-REVIEW-PRESHIP.md): the captured message body + PDF
	// attachment contain donor PII (name, receipt, amount) written unencrypted
	// to local disk. Restrict them to the owner only — 0700 dir, 0600 files —
	// instead of the previous world-readable 0755/0644.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return SendResult{}, err
	}

	if err := os.WriteFile(filepath.Join(dir, "body.html"), []byte(msg.BodyHTML), 0o600); err != nil {
		return SendResult{}, err
	}

	if msg.Attachment.Filename != "" {
		if err := os.WriteFile(filepath.Join(dir, msg.Attachment.Filename), msg.Attachment.Data, 0o600); err != nil {
			return SendResult{}, err
		}
	}

	return SendResult{SentAt: time.Now()}, nil
}
