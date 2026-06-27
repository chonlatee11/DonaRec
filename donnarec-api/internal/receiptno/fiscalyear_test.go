// Package receiptno provides white-box unit tests for fiscalYear() boundary behaviour.
//
// This file tests fiscalYear() boundary behaviour (SC#2, D-40, D-41):
//   - Thai fiscal year runs Oct 1 – Sep 30 (Buddhist Era).
//   - Oct–Dec of CE year Y belong to fiscal year Y+544 (BE).
//   - Jan–Sep of CE year Y belong to fiscal year Y+543 (BE).
//   - Any input timezone is normalised to Asia/Bangkok before the boundary check.
//
// White-box test (package receiptno) is used because fiscalYear() is intentionally
// unexported — it is an internal pure helper called only by allocator.go (D-35).
package receiptno

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFiscalYear(t *testing.T) {
	bkk, err := time.LoadLocation("Asia/Bangkok")
	require.NoError(t, err, "Asia/Bangkok timezone must be available")

	cases := []struct {
		name     string
		input    time.Time
		expected int
	}{
		{
			// Last second of fiscal year 2568 — still in Jan-Sep window of CE 2025
			name:     "Sep 30 23:59:59 BKK → 2568",
			input:    time.Date(2025, time.September, 30, 23, 59, 59, 0, bkk),
			expected: 2568,
		},
		{
			// First second of fiscal year 2569 — Oct 1 triggers next-year rule
			name:     "Oct 1 00:00:00 BKK → 2569",
			input:    time.Date(2025, time.October, 1, 0, 0, 0, 0, bkk),
			expected: 2569,
		},
		{
			// UTC 17:00 on Sep 30 = BKK 00:00 on Oct 1 → must normalise to BKK first.
			// This is the critical timezone-normalisation boundary test (D-40).
			// Asia/Bangkok is UTC+7; Sep 30 17:00 UTC + 7h = Oct 1 00:00 BKK → 2569.
			name:     "Sep 30 17:00:00 UTC (= Oct 1 00:00 BKK) → 2569",
			input:    time.Date(2025, time.September, 30, 17, 0, 0, 0, time.UTC),
			expected: 2569,
		},
		{
			// Mid-fiscal-year: Jan 1 2026 is within the 2569 fiscal year (Oct 2025 – Sep 2026)
			name:     "Jan 1 2026 00:00:00 BKK → 2569",
			input:    time.Date(2026, time.January, 1, 0, 0, 0, 0, bkk),
			expected: 2569,
		},
		{
			// Last second of fiscal year 2569
			name:     "Sep 30 2026 23:59:59 BKK → 2569",
			input:    time.Date(2026, time.September, 30, 23, 59, 59, 0, bkk),
			expected: 2569,
		},
		{
			// First second of fiscal year 2570
			name:     "Oct 1 2026 00:00:00 BKK → 2570",
			input:    time.Date(2026, time.October, 1, 0, 0, 0, 0, bkk),
			expected: 2570,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fiscalYear(tc.input)
			require.Equal(t, tc.expected, got,
				"fiscalYear(%v) = %d; want %d", tc.input, got, tc.expected)
		})
	}
}
