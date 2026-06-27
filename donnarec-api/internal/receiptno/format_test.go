// Package receiptno provides white-box unit tests for formatReceiptNo().
//
// This file tests formatReceiptNo() rendering (D-28, D-29, D-30, D-42):
//   - Default format: fiscalYear(BE4) + separator + zero-padded running_no (e.g. "2569/000123").
//   - Padding is minimum width — values wider than padding expand naturally, never truncate (D-29).
//   - yearFormat "CE4" renders the Christian Era year (fiscalYear - 543).
//   - Prefix is prepended before the year (D-30).
//
// White-box test (package receiptno) is used because formatReceiptNo() is intentionally
// unexported — it is an internal pure helper called only by allocator.go (D-35).
package receiptno

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatReceiptNo(t *testing.T) {
	cases := []struct {
		name       string
		fiscalYear int
		runningNo  int
		separator  string
		padding    int
		yearFormat string
		prefix     string
		expected   string
	}{
		{
			// Default format: BE4 year, "/" separator, 6-digit padding, no prefix
			name:       "default BE4 format",
			fiscalYear: 2569,
			runningNo:  123,
			separator:  "/",
			padding:    6,
			yearFormat: "BE4",
			prefix:     "",
			expected:   "2569/000123",
		},
		{
			// D-29: running_no exceeds padding width — must expand naturally, NOT truncate or error
			name:       "running_no > 6 digits expands naturally (D-29 no-truncation)",
			fiscalYear: 2569,
			runningNo:  1000000,
			separator:  "/",
			padding:    6,
			yearFormat: "BE4",
			prefix:     "",
			expected:   "2569/1000000",
		},
		{
			// Prefix + non-default separator + 4-digit padding
			name:       "prefix HOSP with dash separator and 4-digit padding",
			fiscalYear: 2569,
			runningNo:  5,
			separator:  "-",
			padding:    4,
			yearFormat: "BE4",
			prefix:     "HOSP",
			expected:   "HOSP2569-0005",
		},
		{
			// CE4 year format: render Christian Era year (fiscalYear - 543)
			// 2569 - 543 = 2026
			name:       "CE4 year format renders CE year",
			fiscalYear: 2569,
			runningNo:  7,
			separator:  "/",
			padding:    6,
			yearFormat: "CE4",
			prefix:     "",
			expected:   "2026/000007",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := formatReceiptNo(tc.fiscalYear, tc.runningNo, tc.separator, tc.padding, tc.yearFormat, tc.prefix)
			require.NoError(t, err)
			require.Equal(t, tc.expected, got,
				"formatReceiptNo(%d, %d, %q, %d, %q, %q) = %q; want %q",
				tc.fiscalYear, tc.runningNo, tc.separator, tc.padding, tc.yearFormat, tc.prefix, got, tc.expected)
		})
	}
}

// TestFormatReceiptNo_RejectsDangerousConfigChars verifies the defense-in-depth
// allowlist (WR-02): a prefix or separator carrying HTML/script-injection characters
// must be rejected with an error so the tainted value never reaches the immutable ledger.
func TestFormatReceiptNo_RejectsDangerousConfigChars(t *testing.T) {
	cases := []struct {
		name      string
		separator string
		prefix    string
	}{
		{name: "script tag in prefix", separator: "/", prefix: "<img src=x onerror=alert(1)>"},
		{name: "angle brackets in separator", separator: "</td><td>", prefix: ""},
		{name: "quote in prefix", separator: "/", prefix: `HOSP"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := formatReceiptNo(2569, 1, tc.separator, 6, "BE4", tc.prefix)
			require.Error(t, err, "dangerous config chars must be rejected")
		})
	}
}
