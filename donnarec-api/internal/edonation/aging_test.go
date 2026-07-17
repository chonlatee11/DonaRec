// internal/edonation/aging_test.go — TDD RED→GREEN tests for the pure,
// Bangkok-aware aging deadline/bucket computation (Task 1, plan 05-04, FR-31/D-68).
//
// White-box test (package edonation, not edonation_test) is used because
// computeDeadline/computeBucket are intentionally unexported — pure internal
// helpers called only by Service.Aging (mirrors receiptno/fiscalyear_test.go's
// white-box convention for fiscalYear()).
package edonation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestComputeDeadline_DecemberRollover proves computeDeadline correctly rolls
// December approvals into January of the FOLLOWING year (D-68, Pitfall 1) —
// trusting time.Date's stdlib month-overflow normalization rather than a
// hand-written December special case.
func TestComputeDeadline_DecemberRollover(t *testing.T) {
	bkk, err := time.LoadLocation("Asia/Bangkok")
	require.NoError(t, err, "Asia/Bangkok timezone must be available")

	cases := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			name:     "December approval → January 5th of the NEXT year",
			input:    time.Date(2025, time.December, 15, 10, 30, 0, 0, bkk),
			expected: time.Date(2026, time.January, 5, 0, 0, 0, 0, bkk),
		},
		{
			name:     "January approval → February 5th of the SAME year",
			input:    time.Date(2026, time.January, 20, 9, 0, 0, 0, bkk),
			expected: time.Date(2026, time.February, 5, 0, 0, 0, 0, bkk),
		},
		{
			name:     "November approval → December 5th of the SAME year",
			input:    time.Date(2025, time.November, 1, 0, 0, 0, 0, bkk),
			expected: time.Date(2025, time.December, 5, 0, 0, 0, 0, bkk),
		},
		{
			name: "UTC input is normalised to Bangkok before month math (approval near midnight UTC)",
			// 2025-12-31 20:00 UTC = 2026-01-01 03:00 BKK (UTC+7) → January approval → Feb 5.
			input:    time.Date(2025, time.December, 31, 20, 0, 0, 0, time.UTC),
			expected: time.Date(2026, time.February, 5, 0, 0, 0, 0, bkk),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeDeadline(tc.input)
			require.True(t, got.Equal(tc.expected),
				"computeDeadline(%v) = %v; want %v", tc.input, got, tc.expected)
			assert2SameLocation(t, got, bkk)
		})
	}
}

// assert2SameLocation is a tiny local helper asserting the returned deadline is
// normalized to Asia/Bangkok (never left in the caller's original location).
func assert2SameLocation(t *testing.T, got time.Time, bkk *time.Location) {
	t.Helper()
	require.Equal(t, bkk.String(), got.Location().String(),
		"computeDeadline must return a time normalized to Asia/Bangkok")
}

// TestComputeBucket proves the 3-bucket classification (not_due/near_due/overdue)
// against a fixed nearDueDays threshold, including exact-threshold boundary
// instants (mirrors receiptno/fiscalyear_test.go's boundary style, D-68).
func TestComputeBucket(t *testing.T) {
	bkk, err := time.LoadLocation("Asia/Bangkok")
	require.NoError(t, err, "Asia/Bangkok timezone must be available")

	// approvedAt = Jan 15 → deadline = Feb 5 00:00:00 BKK.
	approvedAt := time.Date(2026, time.January, 15, 12, 0, 0, 0, bkk)
	deadline := time.Date(2026, time.February, 5, 0, 0, 0, 0, bkk)
	const nearDueDays = 3

	cases := []struct {
		name     string
		now      time.Time
		expected AgingBucket
	}{
		{
			name:     "well before deadline (10 days out) → not_due",
			now:      deadline.AddDate(0, 0, -10),
			expected: BucketNotDue,
		},
		{
			name:     "exactly nearDueDays+1 before deadline → not_due (one day more than threshold)",
			now:      deadline.AddDate(0, 0, -(nearDueDays + 1)),
			expected: BucketNotDue,
		},
		{
			name:     "exactly nearDueDays before deadline → near_due (boundary, inclusive)",
			now:      deadline.AddDate(0, 0, -nearDueDays),
			expected: BucketNearDue,
		},
		{
			name:     "1 day before deadline → near_due",
			now:      deadline.AddDate(0, 0, -1),
			expected: BucketNearDue,
		},
		{
			name:     "exactly at the deadline instant → overdue",
			now:      deadline,
			expected: BucketOverdue,
		},
		{
			name:     "1 day after deadline → overdue",
			now:      deadline.AddDate(0, 0, 1),
			expected: BucketOverdue,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeBucket(approvedAt, tc.now, nearDueDays)
			require.Equal(t, tc.expected, got,
				"computeBucket(approvedAt=%v, now=%v, nearDueDays=%d) = %v; want %v",
				approvedAt, tc.now, nearDueDays, got, tc.expected)
		})
	}
}

// TestComputeBucket_DecemberRolloverIntegration proves the bucket classification
// still works correctly when the deadline itself crosses a year boundary.
func TestComputeBucket_DecemberRolloverIntegration(t *testing.T) {
	bkk, err := time.LoadLocation("Asia/Bangkok")
	require.NoError(t, err)

	// approvedAt = Dec 20 2025 → deadline = Jan 5 2026 00:00:00 BKK.
	approvedAt := time.Date(2025, time.December, 20, 8, 0, 0, 0, bkk)
	now := time.Date(2026, time.January, 3, 0, 0, 0, 0, bkk) // 2 days before deadline
	got := computeBucket(approvedAt, now, 3)
	require.Equal(t, BucketNearDue, got,
		"December-approved donation 2 days before its January 5th deadline must be near_due")
}
