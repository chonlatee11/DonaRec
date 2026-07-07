// internal/report/service_test.go — TDD RED→GREEN tests for Service.Summary
// (Task 1, plan 05-05, FR-32/D-70/D-71).
//
// Fixtures reuse the REAL donation lifecycle (Create->Submit->Approve[->Cancel])
// via internal/donation.DonationService — mirrors internal/edonation's
// keyedFixture/exportFixture pattern (the only way to produce a genuinely
// 'issued' or 'cancelled' donation for these tests).
package report_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/donnarec/donnarec-api/internal/audit"
	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/crypto"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/donation"
	"github.com/donnarec/donnarec-api/internal/receiptno"
	"github.com/donnarec/donnarec-api/internal/report"
	"github.com/donnarec/donnarec-api/internal/testutil"
	"github.com/jackc/pgx/v5/pgtype"
)

// reportTestKEK is a 32-byte hex key for this file's test use only (same
// convention as edonation/keyed_test.go's keyedTestKEK — test-only, never a
// real secret). donation.DonationService still requires a key provider even
// though report.Service itself does not.
const reportTestKEK = "3102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f31"

// reportFixture bundles the donation lifecycle service (to seed issued/
// cancelled fixtures) plus the report.Service under test.
type reportFixture struct {
	donationSvc   *donation.DonationService
	reportSvc     *report.Service
	makerID       pgtype.UUID
	checkerID     pgtype.UUID
	makerClaims   auth.KeycloakClaims
	checkerClaims auth.KeycloakClaims
}

func setupReportFixture(t *testing.T) *reportFixture {
	t.Helper()
	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", reportTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	auditSvc := audit.NewAuditService(pool, queries, zap.NewNop())
	alloc := receiptno.NewAllocator(queries)
	donationSvc := donation.NewDonationService(pool, queries, alloc, auditSvc, kp, zap.NewNop())
	reportSvc := report.NewService(queries)

	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "maker-report-test@example.com", DisplayName: "Maker Report Test",
		KeycloakSubject: "55555555-5555-5555-5555-555555555555",
	})
	require.NoError(t, err)
	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-report-test@example.com", DisplayName: "Checker Report Test",
		KeycloakSubject: "66666666-6666-6666-6666-666666666666",
	})
	require.NoError(t, err)

	return &reportFixture{
		donationSvc: donationSvc,
		reportSvc:   reportSvc,
		makerID:     makerRow.ID,
		checkerID:   checkerRow.ID,
		makerClaims: auth.KeycloakClaims{
			Subject:     "55555555-5555-5555-5555-555555555555",
			RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
		},
		checkerClaims: auth.KeycloakClaims{
			Subject:     "66666666-6666-6666-6666-666666666666",
			RealmAccess: auth.RealmRoles{Roles: []string{"checker"}},
		},
	}
}

func (f *reportFixture) seedIssued(t *testing.T, ctx context.Context, name, taxID, date string, amount float64) *donation.DonationDetailResponse {
	t.Helper()
	d, err := f.donationSvc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  name,
		DonorTaxID: taxID,
		Amount:     amount,
		DonatedAt:  date,
	}, f.makerID, f.makerClaims)
	require.NoError(t, err)

	_, err = f.donationSvc.Submit(ctx, d.ID, f.makerClaims)
	require.NoError(t, err)

	issued, err := f.donationSvc.Approve(ctx, d.ID, f.checkerID, f.checkerClaims)
	require.NoError(t, err)
	require.Equal(t, "issued", issued.Status)
	return issued
}

func (f *reportFixture) seedCancelled(t *testing.T, ctx context.Context, name, taxID, date string, amount float64) *donation.DonationDetailResponse {
	t.Helper()
	issued := f.seedIssued(t, ctx, name, taxID, date, amount)
	cancelled, err := f.donationSvc.Cancel(ctx, issued.ID, donation.CancelDonationRequest{
		Reason: "test cancellation for report fixture",
	}, f.checkerID, f.checkerClaims)
	require.NoError(t, err)
	require.Equal(t, "cancelled", cancelled.Status)
	return cancelled
}

func findPeriodRow(t *testing.T, rows []report.PeriodRow, period string) report.PeriodRow {
	t.Helper()
	for _, r := range rows {
		if r.Period == period {
			return r
		}
	}
	t.Fatalf("no breakdown row found for period %q (rows: %+v)", period, rows)
	return report.PeriodRow{}
}

func ptrTime(tm time.Time) *time.Time { return &tm }

// TestReportSummary_MonthlyBreakdown_ExcludesCancelled proves: (a) issued
// donations across two calendar months sum correctly into a per-month
// breakdown; (b) a cancelled donation is excluded from BOTH the top-line
// totals AND the breakdown entirely (Assumption A2); (c) the top-line
// TotalAmount/ReceiptCount/AveragePerReceipt match the sum of the breakdown
// rows exactly.
func TestReportSummary_MonthlyBreakdown_ExcludesCancelled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}
	f := setupReportFixture(t)
	ctx := context.Background()

	// January: two issued donations (one with a satang fraction).
	f.seedIssued(t, ctx, "นาย รายงาน หนึ่ง", "5050505050501", "2026-01-05", 1000.00)
	f.seedIssued(t, ctx, "นาย รายงาน สอง", "5050505050502", "2026-01-20", 500.50)
	// February: one issued donation.
	f.seedIssued(t, ctx, "นาย รายงาน สาม", "5050505050503", "2026-02-10", 2000.25)
	// A cancelled donation in January — must be excluded entirely (A2).
	f.seedCancelled(t, ctx, "นาย รายงาน ยกเลิก", "5050505050504", "2026-01-15", 9999.00)

	result, err := f.reportSvc.Summary(ctx, report.SummaryFilter{GroupBy: "month"})
	require.NoError(t, err)

	require.Len(t, result.Breakdown, 2, "exactly two monthly buckets: Jan + Feb (cancelled excluded, no third bucket)")

	jan := findPeriodRow(t, result.Breakdown, "2026-01-01")
	assert.Equal(t, 2, jan.ReceiptCount)
	assert.InDelta(t, 1500.50, jan.TotalAmount, 0.001)

	feb := findPeriodRow(t, result.Breakdown, "2026-02-01")
	assert.Equal(t, 1, feb.ReceiptCount)
	assert.InDelta(t, 2000.25, feb.TotalAmount, 0.001)

	assert.Equal(t, 3, result.ReceiptCount, "cancelled donation must not count toward the top-line receipt count")
	assert.InDelta(t, 3500.75, result.TotalAmount, 0.001, "cancelled donation's amount must not count toward the top-line total")
	assert.InDelta(t, 3500.75/3, result.AveragePerReceipt, 0.001)
}

// TestReportSummary_DailyBreakdown_OneRowPerDay proves group_by=day produces
// one breakdown row per exact donated_at date (not collapsed by month).
func TestReportSummary_DailyBreakdown_OneRowPerDay(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}
	f := setupReportFixture(t)
	ctx := context.Background()

	f.seedIssued(t, ctx, "นาย รายวัน หนึ่ง", "6060606060601", "2026-03-01", 300.00)
	f.seedIssued(t, ctx, "นาย รายวัน สอง", "6060606060602", "2026-03-02", 700.00)

	result, err := f.reportSvc.Summary(ctx, report.SummaryFilter{GroupBy: "day"})
	require.NoError(t, err)

	require.Len(t, result.Breakdown, 2, "two distinct donated_at days must produce two daily buckets")

	day1 := findPeriodRow(t, result.Breakdown, "2026-03-01")
	assert.Equal(t, 1, day1.ReceiptCount)
	assert.InDelta(t, 300.00, day1.TotalAmount, 0.001)

	day2 := findPeriodRow(t, result.Breakdown, "2026-03-02")
	assert.Equal(t, 1, day2.ReceiptCount)
	assert.InDelta(t, 700.00, day2.TotalAmount, 0.001)

	assert.Equal(t, 2, result.ReceiptCount)
	assert.InDelta(t, 1000.00, result.TotalAmount, 0.001)
}

// TestReportSummary_DateRangeFilter proves From/To narrows the result to only
// donations within the inclusive range.
func TestReportSummary_DateRangeFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}
	f := setupReportFixture(t)
	ctx := context.Background()

	f.seedIssued(t, ctx, "นาย ก่อนช่วง", "7070707070701", "2026-04-01", 111.00)
	f.seedIssued(t, ctx, "นาย ในช่วง", "7070707070702", "2026-04-15", 222.00)
	f.seedIssued(t, ctx, "นาย หลังช่วง", "7070707070703", "2026-04-30", 333.00)

	from := ptrTime(time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC))
	to := ptrTime(time.Date(2026, time.April, 20, 0, 0, 0, 0, time.UTC))

	result, err := f.reportSvc.Summary(ctx, report.SummaryFilter{GroupBy: "day", From: from, To: to})
	require.NoError(t, err)

	require.Len(t, result.Breakdown, 1, "only the in-range donation must appear")
	assert.Equal(t, 1, result.ReceiptCount)
	assert.InDelta(t, 222.00, result.TotalAmount, 0.001)
}

// TestReportSummary_EmptyRange_ZeroNoPanic proves a date range matching zero
// issued donations returns an all-zero SummaryResult with an empty Breakdown —
// AveragePerReceipt must be 0 (divide-by-zero guarded), never a panic.
func TestReportSummary_EmptyRange_ZeroNoPanic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}
	f := setupReportFixture(t)
	ctx := context.Background()

	from := ptrTime(time.Date(1999, time.January, 1, 0, 0, 0, 0, time.UTC))
	to := ptrTime(time.Date(1999, time.December, 31, 0, 0, 0, 0, time.UTC))

	result, err := f.reportSvc.Summary(ctx, report.SummaryFilter{GroupBy: "month", From: from, To: to})
	require.NoError(t, err)

	assert.Equal(t, 0, result.ReceiptCount)
	assert.Equal(t, 0.0, result.TotalAmount)
	assert.Equal(t, 0.0, result.AveragePerReceipt, "average must be guarded to 0, not NaN/panic, when count is 0")
	assert.Empty(t, result.Breakdown)
}

// TestReportSummary_InvalidGroupBy proves an unrecognized GroupBy is rejected
// with ErrInvalidGroupBy — service-layer defense-in-depth beyond the
// handler's allowlist (no DB round trip needed for this case).
func TestReportSummary_InvalidGroupBy(t *testing.T) {
	svc := report.NewService(db.New(nil))
	_, err := svc.Summary(context.Background(), report.SummaryFilter{GroupBy: "year"})
	require.Error(t, err)
	assert.ErrorIs(t, err, report.ErrInvalidGroupBy)
}
