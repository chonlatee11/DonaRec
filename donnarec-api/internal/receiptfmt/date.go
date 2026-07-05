// Package receiptfmt holds the shared, pure display-formatting helpers for the
// receipt artifact (issue date, amount) used by BOTH the outbox worker
// (internal/worker, which renders the real frozen receipt) and the settings
// admin preview (internal/settings, which renders the sample-data preview) — so
// preview output is formatted by the EXACT same code as production output
// (BL-01, 04-REVIEW-PRESHIP.md: "preview == real").
package receiptfmt

import (
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// bangkokLoc caches the loaded Asia/Bangkok *time.Location, mirroring
// internal/receiptno/fiscal_year.go's loader: populated once, then immutable and
// safe for concurrent reads. Asia/Bangkok is UTC+7 all year (no DST), so no
// spring-forward/fall-back edge cases. Panics if tzdata is unavailable — a
// deployment-configuration bug (the receiptno package already relies on the same
// timezone in the Approve tx, so this assumption is not new).
var (
	bangkokLoc  *time.Location
	bangkokOnce sync.Once
)

func loadBangkok() *time.Location {
	bangkokOnce.Do(func() {
		loc, err := time.LoadLocation("Asia/Bangkok")
		if err != nil {
			panic("Asia/Bangkok timezone not available: " + err.Error())
		}
		bangkokLoc = loc
	})
	return bangkokLoc
}

// thaiMonthAbbrev indexes the Thai month abbreviation by calendar month (1–12);
// index 0 is unused. These match the settings preview fixture (มี.ค. = March)
// and standard Thai document usage.
var thaiMonthAbbrev = [...]string{
	"",
	"ม.ค.", "ก.พ.", "มี.ค.", "เม.ย.", "พ.ค.", "มิ.ย.",
	"ก.ค.", "ส.ค.", "ก.ย.", "ต.ค.", "พ.ย.", "ธ.ค.",
}

// FormatIssueDate renders a receipt issue date (approved_at) for display,
// normalising the instant to Asia/Bangkok FIRST (BL-01) so a donation approved
// during Bangkok 00:00–07:00 never shows the previous UTC day on a
// legally-binding tax receipt.
//
//   - lang == "th": "<day> <Thai month abbrev> <Buddhist-Era year>", e.g.
//     "15 มี.ค. 2569". BE year = Gregorian year + 543.
//   - any other lang (e.g. "en"): Gregorian "2 Jan 2006", e.g. "15 Mar 2026".
//
// Returns "" for an unset timestamp (defensive — an issued donation always has
// approved_at set).
//
// COMPLIANCE NOTE: the exact era wording (BE vs CE) and Thai month spelling on
// the final legal receipt are pending accounting/legal sign-off (CLAUDE.md
// stakeholder gate). This aligns to the codebase's existing preview fixture
// ("15 มี.ค. 2569") as the current intended format so preview == real output.
func FormatIssueDate(t pgtype.Timestamptz, lang string) string {
	if !t.Valid {
		return ""
	}
	bkk := t.Time.In(loadBangkok())
	if lang == "th" {
		return fmt.Sprintf("%d %s %d", bkk.Day(), thaiMonthAbbrev[int(bkk.Month())], bkk.Year()+543)
	}
	return bkk.Format("2 Jan 2006")
}
