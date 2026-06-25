// Package receiptno_test provides integration tests for the gap-less allocator.
//
// Tests verify the functional correctness of Allocate(ctx, tx, issueDate):
//   - Single allocation returns RunningNo 1 with correct formatted string
//   - Sequential allocations in the same fiscal year are gap-less (1, 2, 3, ...)
//   - First allocation for a new fiscal year auto-starts at 1 (FR-17)
//   - Two fiscal years tracked independently (FY 2569 at N, FY 2570 starts at 1)
//   - Default config (separator "/", padding 6, BE4) produces "2569/000001"
//
// All tests require a live PostgreSQL container via testcontainers (skip with -short).
package receiptno_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbhelpers "github.com/donnarec/donnarec-api/internal/db"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/receiptno"
	"github.com/donnarec/donnarec-api/internal/testutil"
	"github.com/jackc/pgx/v5"
)

// bkkTime is a helper that constructs a time in Asia/Bangkok timezone.
// It panics if tzdata is unavailable (the same programming-error guard as fiscalYear).
func bkkTime(year, month, day, hour, min, sec int) time.Time {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		panic("Asia/Bangkok timezone not available: " + err.Error())
	}
	return time.Date(year, time.Month(month), day, hour, min, sec, 0, loc)
}

// assertLedgerRows queries the receipt_numbers ledger and returns
// (running_no, formatted) pairs for the given fiscal year, ordered by running_no.
func assertLedgerRows(t *testing.T, pool interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, ctx context.Context, fiscalYear int) []struct {
	RunningNo int
	Formatted string
} {
	t.Helper()
	rows, err := pool.Query(ctx,
		`SELECT running_no, formatted FROM receipt_numbers WHERE fiscal_year = $1 ORDER BY running_no`,
		fiscalYear)
	require.NoError(t, err)
	defer rows.Close()

	var result []struct {
		RunningNo int
		Formatted string
	}
	for rows.Next() {
		var r struct {
			RunningNo int
			Formatted string
		}
		require.NoError(t, rows.Scan(&r.RunningNo, &r.Formatted))
		result = append(result, r)
	}
	require.NoError(t, rows.Err())
	return result
}

// TestAllocator_SingleAllocate verifies that the first allocation for FY 2569
// returns RunningNo=1 and Formatted="2569/000001" (default config).
func TestAllocator_SingleAllocate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)

	// Jan 15 2026 Asia/Bangkok → FY 2569
	issueDate := bkkTime(2026, 1, 15, 10, 0, 0)

	var got receiptno.AllocatedReceipt
	err := dbhelpers.WithTx(ctx, pool, func(tx pgx.Tx) error {
		var allocErr error
		got, allocErr = alloc.Allocate(ctx, tx, issueDate)
		return allocErr
	})
	require.NoError(t, err)

	assert.Equal(t, 2569, got.FiscalYear)
	assert.Equal(t, 1, got.RunningNo)
	assert.Equal(t, "2569/000001", got.Formatted)
	assert.False(t, got.AllocatedAt.IsZero(), "AllocatedAt must be set from DB now()")

	// Verify ledger row matches returned struct
	ledger := assertLedgerRows(t, pool, ctx, 2569)
	require.Len(t, ledger, 1)
	assert.Equal(t, 1, ledger[0].RunningNo)
	assert.Equal(t, "2569/000001", ledger[0].Formatted)
}

// TestAllocator_SequentialGapless verifies that three sequential allocations
// within FY 2569 produce RunningNo 1, 2, 3 with no gaps or duplicates.
func TestAllocator_SequentialGapless(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)

	issueDate := bkkTime(2026, 3, 1, 9, 0, 0) // FY 2569

	var receipts [3]receiptno.AllocatedReceipt
	for i := 0; i < 3; i++ {
		err := dbhelpers.WithTx(ctx, pool, func(tx pgx.Tx) error {
			var allocErr error
			receipts[i], allocErr = alloc.Allocate(ctx, tx, issueDate)
			return allocErr
		})
		require.NoError(t, err, "allocation %d failed", i+1)
	}

	// Assert sequential, gap-less running numbers
	for i, r := range receipts {
		assert.Equal(t, 2569, r.FiscalYear, "fiscal year mismatch at index %d", i)
		assert.Equal(t, i+1, r.RunningNo, "running_no must be %d, got %d", i+1, r.RunningNo)
	}

	// Assert ledger matches exactly: 1, 2, 3
	ledger := assertLedgerRows(t, pool, ctx, 2569)
	require.Len(t, ledger, 3, "ledger must have exactly 3 rows")
	for i, row := range ledger {
		assert.Equal(t, i+1, row.RunningNo, "ledger running_no at index %d", i)
	}
}

// TestAllocator_NewFiscalYearStartsAtOne verifies that allocating for a fiscal year
// with no counter row auto-initializes and starts at RunningNo=1 (FR-17, D-41).
// This test uses FY 2571 (a year not seeded by any other test) to avoid interference.
func TestAllocator_NewFiscalYearStartsAtOne(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)

	// Oct 15 2027 Asia/Bangkok → FY 2571 (Oct-Dec → ceYear+544 = 2027+544 = 2571)
	issueDate := bkkTime(2027, 10, 15, 8, 0, 0)

	var got receiptno.AllocatedReceipt
	err := dbhelpers.WithTx(ctx, pool, func(tx pgx.Tx) error {
		var allocErr error
		got, allocErr = alloc.Allocate(ctx, tx, issueDate)
		return allocErr
	})
	require.NoError(t, err)

	assert.Equal(t, 2571, got.FiscalYear)
	assert.Equal(t, 1, got.RunningNo, "new fiscal year must start at RunningNo=1")
	assert.Equal(t, "2571/000001", got.Formatted)

	// Verify counter table has the new row
	var counterVal int32
	err = pool.QueryRow(ctx,
		`SELECT last_running_no FROM receipt_number_counters WHERE fiscal_year = $1`,
		2571).Scan(&counterVal)
	require.NoError(t, err)
	assert.Equal(t, int32(1), counterVal)
}

// TestAllocator_MultiYearIsolation verifies that allocating across two different fiscal
// years keeps independent counters: FY 2569 and FY 2570 start at 1 and do not interfere.
func TestAllocator_MultiYearIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)

	// FY 2569: Jan 2026 Asia/Bangkok (Jan → ceYear+543 = 2026+543 = 2569)
	fy2569Date := bkkTime(2026, 1, 20, 10, 0, 0)
	// FY 2570: Oct 2026 Asia/Bangkok (Oct → ceYear+544 = 2026+544 = 2570)
	fy2570Date := bkkTime(2026, 10, 5, 10, 0, 0)

	// Allocate twice in FY 2569
	for i := 0; i < 2; i++ {
		err := dbhelpers.WithTx(ctx, pool, func(tx pgx.Tx) error {
			_, allocErr := alloc.Allocate(ctx, tx, fy2569Date)
			return allocErr
		})
		require.NoError(t, err, "FY 2569 allocation %d failed", i+1)
	}

	// Allocate once in FY 2570
	var fy2570Receipt receiptno.AllocatedReceipt
	err := dbhelpers.WithTx(ctx, pool, func(tx pgx.Tx) error {
		var allocErr error
		fy2570Receipt, allocErr = alloc.Allocate(ctx, tx, fy2570Date)
		return allocErr
	})
	require.NoError(t, err)

	// FY 2570 must start fresh at 1 (independent counter)
	assert.Equal(t, 2570, fy2570Receipt.FiscalYear)
	assert.Equal(t, 1, fy2570Receipt.RunningNo, "FY 2570 must start at 1 independently of FY 2569")
	assert.Equal(t, "2570/000001", fy2570Receipt.Formatted)

	// FY 2569 ledger must still have exactly 2 rows (1 and 2)
	ledger2569 := assertLedgerRows(t, pool, ctx, 2569)
	require.Len(t, ledger2569, 2)
	assert.Equal(t, 1, ledger2569[0].RunningNo)
	assert.Equal(t, 2, ledger2569[1].RunningNo)

	// FY 2570 ledger must have exactly 1 row
	ledger2570 := assertLedgerRows(t, pool, ctx, 2570)
	require.Len(t, ledger2570, 1)
	assert.Equal(t, 1, ledger2570[0].RunningNo)
}

// TestAllocator_DefaultConfigFormat verifies that the default config (seeded by migration)
// produces the expected "YYYY/NNNNNN" format: BE4 year + "/" separator + 6-digit zero-pad.
// This mirrors the acceptance criterion: "2569/000001" for FY 2569, RunningNo 1.
func TestAllocator_DefaultConfigFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)

	// Sep 15 2026 Asia/Bangkok → FY 2569 (Sep → ceYear+543 = 2026+543 = 2569)
	issueDate := bkkTime(2026, 9, 15, 12, 0, 0)

	var got receiptno.AllocatedReceipt
	err := dbhelpers.WithTx(ctx, pool, func(tx pgx.Tx) error {
		var allocErr error
		got, allocErr = alloc.Allocate(ctx, tx, issueDate)
		return allocErr
	})
	require.NoError(t, err)

	// Default config: separator="/", running_no_padding=6, year_format="BE4", prefix=""
	// Expected: "2569" + "/" + "000001" = "2569/000001"
	assert.Equal(t, "2569/000001", got.Formatted,
		"default config must produce BE4 year + '/' + 6-digit zero-padded running number")

	// Verify Formatted in returned struct matches what is persisted in the ledger (D-42)
	ledger := assertLedgerRows(t, pool, ctx, 2569)
	require.Len(t, ledger, 1)
	assert.Equal(t, got.Formatted, ledger[0].Formatted,
		"Formatted must be the frozen snapshot persisted in the ledger (D-42)")
}
