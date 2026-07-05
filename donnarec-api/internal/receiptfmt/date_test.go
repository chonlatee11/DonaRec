// Package receiptfmt_test — BL-01 (04-REVIEW-PRESHIP.md) regression coverage:
// the receipt issue date must be rendered in Asia/Bangkok (not UTC), and in the
// Thai Buddhist-Era + Thai-month form for the "th" locale, so a donation
// approved during Bangkok 00:00–07:00 (UTC+7) never prints the previous UTC day
// on a legally-binding tax receipt — and never shifts a year boundary into the
// wrong tax year.
package receiptfmt_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/donnarec/donnarec-api/internal/receiptfmt"
)

func mustTs(t *testing.T, rfc3339 string) pgtype.Timestamptz {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, rfc3339)
	require.NoError(t, err)
	return pgtype.Timestamptz{Time: parsed, Valid: true}
}

// TestFormatIssueDate_ThaiBangkokBoundary is the canonical BL-01 case: an
// instant that is one UTC day but the NEXT Bangkok day (18:30Z → 01:30 BKK the
// following day). The receipt must show the Bangkok calendar date, the Thai
// month abbreviation, and the Buddhist-Era year (+543).
func TestFormatIssueDate_ThaiBangkokBoundary(t *testing.T) {
	t.Parallel()
	// 2026-06-01T18:30:00Z → Asia/Bangkok 2026-06-02 01:30 → 2 มิ.ย. 2569.
	got := receiptfmt.FormatIssueDate(mustTs(t, "2026-06-01T18:30:00Z"), "th")
	require.Equal(t, "2 มิ.ย. 2569", got)
}

// TestFormatIssueDate_EnglishBangkokBoundary proves the same Bangkok-date shift
// for the English locale, rendered Gregorian.
func TestFormatIssueDate_EnglishBangkokBoundary(t *testing.T) {
	t.Parallel()
	got := receiptfmt.FormatIssueDate(mustTs(t, "2026-06-01T18:30:00Z"), "en")
	require.Equal(t, "2 Jun 2026", got)
}

// TestFormatIssueDate_MatchesPreviewFixture pins preview == real: the same
// helper, given the settings sample instant (15 March 2026, Bangkok), must
// reproduce the exact strings the admin preview fixture has always shown
// ("15 มี.ค. 2569" / "15 Mar 2026").
func TestFormatIssueDate_MatchesPreviewFixture(t *testing.T) {
	t.Parallel()
	ts := mustTs(t, "2026-03-15T12:00:00Z") // Bangkok 2026-03-15 19:00
	require.Equal(t, "15 มี.ค. 2569", receiptfmt.FormatIssueDate(ts, "th"))
	require.Equal(t, "15 Mar 2026", receiptfmt.FormatIssueDate(ts, "en"))
}

// TestFormatIssueDate_YearBoundary proves the tax-year-shift risk is closed: an
// instant that is 30 Sep in UTC but 1 Oct in Bangkok must render the Bangkok
// date (Thai fiscal-year rollover day).
func TestFormatIssueDate_YearBoundary(t *testing.T) {
	t.Parallel()
	// 2026-09-30T18:00:00Z → Asia/Bangkok 2026-10-01 01:00.
	require.Equal(t, "1 ต.ค. 2569", receiptfmt.FormatIssueDate(mustTs(t, "2026-09-30T18:00:00Z"), "th"))
	require.Equal(t, "1 Oct 2026", receiptfmt.FormatIssueDate(mustTs(t, "2026-09-30T18:00:00Z"), "en"))
}

// TestFormatIssueDate_InvalidReturnsEmpty keeps the defensive empty-string
// behavior for an unset timestamp.
func TestFormatIssueDate_InvalidReturnsEmpty(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", receiptfmt.FormatIssueDate(pgtype.Timestamptz{}, "th"))
}
