// Package report — model.go
//
// Request/response Go structs for the donation summary report slice (FR-32,
// plan 05-05, D-70/D-71). This report is deliberately PII-free (SUM/COUNT
// aggregates over `donations` only, WHERE status='issued') and open to every
// authenticated staff role (Maker/Checker/Admin, D-71 — no route guard) —
// unlike the export/aging/keyed slices in internal/edonation, there is no
// donor-identifying column anywhere on this path and therefore no
// audited-reveal discipline to mirror.
package report

import "time"

// SummaryFilter narrows Summary() to an optional donated_at date range (A1
// default assumption — 05-RESEARCH.md Assumptions Log) plus a required
// breakdown granularity. GroupBy is validated against an allowlist
// ("month"|"day") at the HANDLER boundary (mirrors edonation.Handler.Export's
// format-allowlist discipline) — Service.Summary re-validates as
// defense-in-depth and returns ErrInvalidGroupBy for anything else.
type SummaryFilter struct {
	From    *time.Time // inclusive lower bound on donated_at
	To      *time.Time // inclusive upper bound on donated_at
	GroupBy string     // "month" | "day"
}

// PeriodRow is one row of the month/day breakdown — a single aggregation
// bucket's receipt count and summed amount. Period is a "YYYY-MM-DD" string:
// the first day of the month for a monthly breakdown, or the exact day for a
// daily breakdown.
type PeriodRow struct {
	Period       string
	ReceiptCount int
	TotalAmount  float64
}

// SummaryResult is the full aggregate report: top-line totals (computed in Go
// as the sum of Breakdown's rows, per plan 05-05's <action>) plus the
// per-period breakdown itself. AveragePerReceipt is TotalAmount /
// ReceiptCount, guarded against divide-by-zero (an empty-range query returns
// an all-zero SummaryResult with an empty Breakdown, never a panic).
type SummaryResult struct {
	TotalAmount       float64
	ReceiptCount      int
	AveragePerReceipt float64
	Breakdown         []PeriodRow
}
