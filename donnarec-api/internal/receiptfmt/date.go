// Package receiptfmt holds the shared, pure display-formatting helpers for the
// receipt artifact (issue date, amount) used by BOTH the outbox worker
// (internal/worker, which renders the real frozen receipt) and the settings
// admin preview (internal/settings, which renders the sample-data preview) — so
// preview output is formatted by the EXACT same code as production output
// (BL-01, 04-REVIEW-PRESHIP.md: "preview == real").
package receiptfmt

import (
	"github.com/jackc/pgx/v5/pgtype"
)

// FormatIssueDate renders a receipt issue date (approved_at) for display.
//
// NOTE (BL-01 RED stub): this is intentionally the OLD, defective behavior —
// UTC calendar date in ISO-Gregorian, ignoring lang — which the accompanying
// test asserts against and therefore fails. The GREEN commit replaces this body
// with Asia/Bangkok + Thai-BE/English-Gregorian rendering.
func FormatIssueDate(t pgtype.Timestamptz, lang string) string {
	if !t.Valid {
		return ""
	}
	return t.Time.Format("2006-01-02")
}
