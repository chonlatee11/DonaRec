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
	"math/big"

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

	// totalAmountRat accumulates the top-line total EXACTLY across breakdown
	// rows (WR-03): each row's DB NUMERIC(15,2) is added as a big.Rat, so no
	// base-10 currency fraction is lost and no IEEE-754 rounding drift can
	// accumulate the way summing many float64 amounts would. The rational total
	// (and the average derived from it) is converted to float64 only once, at
	// the JSON/display boundary below — mirroring CLAUDE.md's money-precision
	// convention (numeric source of truth; float only at the edge).
	totalAmountRat := new(big.Rat)
	var totalCount int

	// accumulate converts one aggregate row's amount to an exact rational, adds
	// it to the running total, and returns the per-row PeriodRow (the per-row
	// display value is a single DB SUM, converted once — the drift risk is only
	// in the cross-row accumulation, which stays rational).
	accumulate := func(period pgtype.Date, count int64, amount pgtype.Numeric) (PeriodRow, error) {
		rat, err := numericToRat(amount)
		if err != nil {
			return PeriodRow{}, err
		}
		totalAmountRat.Add(totalAmountRat, rat)
		totalCount += int(count)
		f, _ := rat.Float64()
		return PeriodRow{
			Period:       dateStr(period),
			ReceiptCount: int(count),
			TotalAmount:  f,
		}, nil
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
			row, convErr := accumulate(r.Period, r.ReceiptCount, r.TotalAmount)
			if convErr != nil {
				return SummaryResult{}, fmt.Errorf("report: convert day total amount: %w", convErr)
			}
			breakdown = append(breakdown, row)
		}
	case "month":
		rows, err := s.queries.SummaryByMonth(ctx, db.SummaryByMonthParams{FromDate: fromDate, ToDate: toDate})
		if err != nil {
			return SummaryResult{}, fmt.Errorf("report: summary by month: %w", err)
		}
		breakdown = make([]PeriodRow, 0, len(rows))
		for _, r := range rows {
			row, convErr := accumulate(r.Period, r.ReceiptCount, r.TotalAmount)
			if convErr != nil {
				return SummaryResult{}, fmt.Errorf("report: convert month total amount: %w", convErr)
			}
			breakdown = append(breakdown, row)
		}
	default:
		return SummaryResult{}, ErrInvalidGroupBy
	}

	totalAmount, _ := totalAmountRat.Float64()

	var average float64
	if totalCount > 0 {
		avgRat := new(big.Rat).Quo(totalAmountRat, new(big.Rat).SetInt64(int64(totalCount)))
		average, _ = avgRat.Float64()
	}

	return SummaryResult{
		TotalAmount:       totalAmount,
		ReceiptCount:      totalCount,
		AveragePerReceipt: average,
		Breakdown:         breakdown,
	}, nil
}

// numericToRat converts a pgtype.Numeric aggregate (SUM(amount)::numeric — see
// internal/db/queries/reports.sql's cast comment) to an EXACT math/big.Rat,
// preserving every base-10 fractional digit (WR-03). The numeric's value is
// Int × 10^Exp; a positive Exp scales up, a negative Exp scales down by an
// exact power of ten — no float64 ever participates, so no currency fraction is
// lost. An invalid/unset numeric (should not occur for an existing GROUP BY
// bucket, since every returned row has at least one matching donation) converts
// to 0 rather than erroring; a NaN/±Infinity aggregate (impossible for
// SUM(amount) over finite money rows) is rejected explicitly.
func numericToRat(n pgtype.Numeric) (*big.Rat, error) {
	if !n.Valid || n.Int == nil {
		return new(big.Rat), nil
	}
	if n.NaN || n.InfinityModifier != pgtype.Finite {
		return nil, fmt.Errorf("report: non-finite numeric aggregate")
	}
	rat := new(big.Rat).SetInt(n.Int)
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs32(n.Exp))), nil)
	if n.Exp >= 0 {
		rat.Mul(rat, new(big.Rat).SetInt(scale))
	} else {
		rat.Quo(rat, new(big.Rat).SetInt(scale))
	}
	return rat, nil
}

// abs32 returns the absolute value of a signed 32-bit integer as an int32.
func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
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
