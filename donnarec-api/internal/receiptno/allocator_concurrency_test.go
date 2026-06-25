// Package receiptno_test — concurrency proof tests for the gap-less allocator.
//
// This file contains the hardest invariant tests for Phase 2 (NFR-04 / SC#4):
//
//   - TestAllocator_Concurrency: 50 parallel allocations against a real PostgreSQL 17
//     instance; asserts COUNT(DISTINCT running_no) == COUNT(*) (zero duplicates) and
//     the committed set is a contiguous 1..N sequence (zero gaps when all commit).
//
//   - TestAllocator_ConcurrentNewYear: 50 goroutines race to allocate the first number
//     of a fresh fiscal year simultaneously; asserts no duplicate, no panic, ledger 1..N.
//
//   - TestAllocator_UniqueConstraintBackstop: deliberately INSERTs a duplicate
//     (fiscal_year, running_no) pair via raw SQL and asserts the error maps to
//     pgerrcode.UniqueViolation.
//
// All tests run under -race and require a live PostgreSQL container (skip with -short).
package receiptno_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	dbhelpers "github.com/donnarec/donnarec-api/internal/db"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/receiptno"
	"github.com/donnarec/donnarec-api/internal/testutil"
)

// fy2569Date returns a date in Asia/Bangkok that falls in Thai fiscal year 2569
// (Jan–Sep 2026 CE = FY 2569). Used by concurrency tests.
func fy2569Date() time.Time {
	return bkkTime(2026, 3, 15, 10, 0, 0) // Mar 15 2026 BKK → FY 2569
}

// TestAllocator_Concurrency fires N=50 parallel allocations against a real Postgres
// container, each inside its own db.WithTx commit, and asserts:
//
//  1. Zero duplicate running_no values (no two goroutines receive the same number).
//  2. The set of committed numbers is exactly 1..50 (contiguous — zero gaps).
//  3. The ledger (receipt_numbers) query matches the committed set exactly.
//
// Run under -race to detect data races in the test harness itself (Pitfall 6).
func TestAllocator_Concurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)

	issueDate := fy2569Date()
	const N = 50

	// mu guards committed to prevent data races under -race.
	var mu sync.Mutex
	committed := make([]int, 0, N)

	g, gctx := errgroup.WithContext(ctx)
	for i := 0; i < N; i++ {
		g.Go(func() error {
			return dbhelpers.WithTx(gctx, pool, func(tx pgx.Tx) error {
				r, err := alloc.Allocate(gctx, tx, issueDate)
				if err != nil {
					return err
				}
				mu.Lock()
				committed = append(committed, r.RunningNo)
				mu.Unlock()
				return nil
			})
		})
	}

	require.NoError(t, g.Wait(), "all 50 parallel allocations must succeed")

	// Sort to make gap/dup assertions straightforward.
	sort.Ints(committed)
	require.Len(t, committed, N, "must have exactly %d committed numbers", N)

	// Assert zero duplicates: sorted slice must be strictly increasing.
	for i := 1; i < len(committed); i++ {
		assert.Less(t, committed[i-1], committed[i],
			"duplicate running_no detected at index %d: %d appears twice",
			i, committed[i-1])
	}

	// Assert contiguous 1..N: after sorting and deduplication check, values must be 1..50.
	for i, no := range committed {
		assert.Equal(t, i+1, no,
			"running_no at position %d must be %d (contiguous gap-less), got %d",
			i, i+1, no)
	}

	// Assert via raw SQL: COUNT(*) == COUNT(DISTINCT running_no) — zero duplicates.
	var totalCount, distinctCount int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*), COUNT(DISTINCT running_no) FROM receipt_numbers WHERE fiscal_year = $1`,
		2569).Scan(&totalCount, &distinctCount)
	require.NoError(t, err, "ledger count query must not fail")
	assert.Equal(t, N, totalCount, "ledger must have exactly %d rows", N)
	assert.Equal(t, N, distinctCount, "COUNT(DISTINCT running_no) must equal COUNT(*) — zero duplicates")

	// Assert ledger rows match committed set exactly (ORDER BY running_no).
	rows, err := pool.Query(ctx,
		`SELECT running_no FROM receipt_numbers WHERE fiscal_year = $1 ORDER BY running_no`,
		2569)
	require.NoError(t, err)
	defer rows.Close()
	var ledgerNos []int
	for rows.Next() {
		var no int
		require.NoError(t, rows.Scan(&no))
		ledgerNos = append(ledgerNos, no)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, committed, ledgerNos,
		"ledger running_no sequence must match committed set exactly")
}

// TestAllocator_ConcurrentNewYear races 50 goroutines all attempting to allocate the
// FIRST number of a fresh fiscal year (FY 2575 — unused by any other test) simultaneously.
// This is the Pitfall 1 scenario: concurrent first-allocation of a brand-new counter row.
//
// Asserts: no duplicate, no panic, counter ends at N, ledger has 1..N.
func TestAllocator_ConcurrentNewYear(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)

	// FY 2575: Oct 1 2031 BKK → fiscal year 2575 (unused by any other test).
	issueDate := bkkTime(2031, 11, 1, 9, 0, 0)
	const N = 50
	const expectedFY = 2575

	var mu sync.Mutex
	committed := make([]int, 0, N)

	g, gctx := errgroup.WithContext(ctx)
	for i := 0; i < N; i++ {
		g.Go(func() error {
			return dbhelpers.WithTx(gctx, pool, func(tx pgx.Tx) error {
				r, err := alloc.Allocate(gctx, tx, issueDate)
				if err != nil {
					return err
				}
				mu.Lock()
				committed = append(committed, r.RunningNo)
				mu.Unlock()
				return nil
			})
		})
	}

	// Must not panic or error — concurrent first-allocation must be handled safely.
	require.NoError(t, g.Wait(),
		"concurrent first-allocation of new fiscal year must not error or panic")

	sort.Ints(committed)
	require.Len(t, committed, N, "must have exactly %d committed numbers", N)

	// Assert zero duplicates.
	for i := 1; i < len(committed); i++ {
		assert.Less(t, committed[i-1], committed[i],
			"duplicate running_no at index %d (concurrent new-year race)", i)
	}

	// Assert contiguous 1..N.
	for i, no := range committed {
		assert.Equal(t, i+1, no,
			"concurrent new-year: running_no at position %d must be %d, got %d",
			i, i+1, no)
	}

	// Assert counter row holds N.
	var counterVal int32
	err := pool.QueryRow(ctx,
		`SELECT last_running_no FROM receipt_number_counters WHERE fiscal_year = $1`,
		expectedFY).Scan(&counterVal)
	require.NoError(t, err, "counter row must exist for FY %d", expectedFY)
	assert.Equal(t, int32(N), counterVal,
		"counter last_running_no must be %d after %d commits", N, N)

	// Assert ledger has 1..N.
	rows, err := pool.Query(ctx,
		`SELECT running_no FROM receipt_numbers WHERE fiscal_year = $1 ORDER BY running_no`,
		expectedFY)
	require.NoError(t, err)
	defer rows.Close()
	var ledgerNos []int
	for rows.Next() {
		var no int
		require.NoError(t, rows.Scan(&no))
		ledgerNos = append(ledgerNos, no)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, committed, ledgerNos,
		"ledger must match committed set exactly (concurrent new-year)")
}

// TestAllocator_UniqueConstraintBackstop verifies that the UNIQUE(fiscal_year, running_no)
// constraint on receipt_numbers fires when a duplicate is deliberately inserted via raw SQL.
//
// This proves decision D-37: the DB-level backstop is active and returns a pgconn.PgError
// with Code == pgerrcode.UniqueViolation ("23505"), giving the app a last-line-of-defense
// against logic bugs in the allocator.
func TestAllocator_UniqueConstraintBackstop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	// Insert a ledger row normally to establish the baseline.
	_, err := pool.Exec(ctx,
		`INSERT INTO receipt_numbers (fiscal_year, running_no, formatted, allocated_at)
		 VALUES ($1, $2, $3, now())`,
		2580, 1, "2580/000001")
	require.NoError(t, err, "first insert must succeed")

	// Deliberately insert a duplicate (same fiscal_year + running_no).
	// This must trigger the UNIQUE constraint backstop (D-37).
	_, dupErr := pool.Exec(ctx,
		`INSERT INTO receipt_numbers (fiscal_year, running_no, formatted, allocated_at)
		 VALUES ($1, $2, $3, now())`,
		2580, 1, "2580/000001-dup")
	require.Error(t, dupErr, "duplicate insert must return an error")

	// Assert error is a *pgconn.PgError with UniqueViolation code (SQLSTATE 23505).
	var pgErr *pgconn.PgError
	require.True(t, errors.As(dupErr, &pgErr),
		"error must be *pgconn.PgError, got: %T", dupErr)
	assert.Equal(t, pgerrcode.UniqueViolation, pgErr.Code,
		"SQLSTATE must be 23505 (unique_violation) — UNIQUE(fiscal_year, running_no) backstop is active")
}
