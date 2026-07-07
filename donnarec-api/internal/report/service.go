// Package report — service.go
//
// Service.Summary implements the PII-free donation summary report (FR-32,
// D-70/D-71). Unlike internal/edonation.Service (which decrypts donor
// national/tax IDs behind an audited-reveal transaction), this service takes
// ONLY a *db.Queries dependency — no key provider, no audit service — because
// the underlying queries (SummaryByMonth/SummaryByDay) select nothing but
// date_trunc(donated_at), COUNT(*), and SUM(amount): no per-donor identifying
// column is ever read, so there is nothing to decrypt and no PII-reveal event
// to audit. D-71 accordingly gives this report NO route-level role gate —
// every authenticated staff member (Maker/Checker/Admin) may view it.
package report

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/donnarec/donnarec-api/internal/db/generated"
)

// Service implements the donation summary report (FR-32, plan 05-05).
type Service struct {
	queries *db.Queries
}

// NewService constructs a Service with the given query dependency. Panics if
// queries is nil — a programming-error guard, mirroring
// edonation.NewConfig's constructor style.
func NewService(queries *db.Queries) *Service {
	if queries == nil {
		panic("report.NewService: queries must not be nil")
	}
	return &Service{queries: queries}
}

// Summary dispatches to SummaryByMonth or SummaryByDay per filter.GroupBy,
// converts each row's aggregate amount from its DB numeric representation to
// a float64 (Pattern: no fractional precision is lost for currency values at
// report-display scale), then computes the top-line TotalAmount/ReceiptCount
// as the sum of the returned Breakdown rows — NOT via a second DB query.
// AveragePerReceipt guards divide-by-zero: an empty result set (no issued
// donations in range) returns an all-zero SummaryResult with an empty
// Breakdown, never a panic.
//
// Both underlying queries already scope WHERE status='issued' (cancelled/
// draft/rejected donations are excluded — Assumption A2, 05-RESEARCH.md) and
// select no column beyond donated_at/COUNT(*)/SUM(amount) — no donor name,
// no encrypted or plaintext identifying field of any kind is ever read on
// this path.
func (s *Service) Summary(ctx context.Context, filter SummaryFilter) (SummaryResult, error) {
	var fromDate, toDate pgtype.Date
	if filter.From != nil {
		fromDate = pgtype.Date{Time: *filter.From, Valid: true}
	}
	if filter.To != nil {
		toDate = pgtype.Date{Time: *filter.To, Valid: true}
	}

	var breakdown []PeriodRow
	switch filter.GroupBy {
	case "day":
		rows, err := s.queries.SummaryByDay(ctx, db.SummaryByDayParams{FromDate: fromDate, ToDate: toDate})
		if err != nil {
			return SummaryResult{}, fmt.Errorf("report: summary by day: %w", err)
		}
		breakdown = make([]PeriodRow, 0, len(rows))
		for _, r := range rows {
			amount, convErr := numericToFloat64(r.TotalAmount)
			if convErr != nil {
				return SummaryResult{}, fmt.Errorf("report: convert day total amount: %w", convErr)
			}
			breakdown = append(breakdown, PeriodRow{
				Period:       dateStr(r.Period),
				ReceiptCount: int(r.ReceiptCount),
				TotalAmount:  amount,
			})
		}
	case "month":
		rows, err := s.queries.SummaryByMonth(ctx, db.SummaryByMonthParams{FromDate: fromDate, ToDate: toDate})
		if err != nil {
			return SummaryResult{}, fmt.Errorf("report: summary by month: %w", err)
		}
		breakdown = make([]PeriodRow, 0, len(rows))
		for _, r := range rows {
			amount, convErr := numericToFloat64(r.TotalAmount)
			if convErr != nil {
				return SummaryResult{}, fmt.Errorf("report: convert month total amount: %w", convErr)
			}
			breakdown = append(breakdown, PeriodRow{
				Period:       dateStr(r.Period),
				ReceiptCount: int(r.ReceiptCount),
				TotalAmount:  amount,
			})
		}
	default:
		return SummaryResult{}, ErrInvalidGroupBy
	}

	var totalAmount float64
	var totalCount int
	for _, row := range breakdown {
		totalAmount += row.TotalAmount
		totalCount += row.ReceiptCount
	}

	var average float64
	if totalCount > 0 {
		average = totalAmount / float64(totalCount)
	}

	return SummaryResult{
		TotalAmount:       totalAmount,
		ReceiptCount:      totalCount,
		AveragePerReceipt: average,
		Breakdown:         breakdown,
	}, nil
}

// numericToFloat64 converts a pgtype.Numeric aggregate (SUM(amount)::numeric —
// see internal/db/queries/reports.sql's cast comment) to a float64. An
// invalid/unset numeric (should not occur for an existing GROUP BY bucket,
// since every returned row has at least one matching donation) converts to 0
// rather than erroring.
func numericToFloat64(n pgtype.Numeric) (float64, error) {
	if !n.Valid {
		return 0, nil
	}
	f, err := n.Float64Value()
	if err != nil {
		return 0, fmt.Errorf("report: numeric to float64: %w", err)
	}
	if !f.Valid {
		return 0, nil
	}
	return f.Float64, nil
}

// dateStr converts a pgtype.Date to a "YYYY-MM-DD" string, or "" if invalid.
// Duplicated (rather than imported) from internal/edonation's private helper
// of the same name/behavior — that helper is unexported in a different
// package (same rationale edonation/service.go documents for its own copy).
func dateStr(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}
