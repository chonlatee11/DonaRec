// Package receiptno_test — rollback proof tests for the gap-less allocator.
//
// This file proves the key distinction between the counter-table approach and
// PostgreSQL SEQUENCE (nextval): when a transaction that called Allocate is rolled back,
// both the counter increment and the ledger insert are rolled back together — the rolled-back
// number is reused by the next successful allocation, leaving zero gap in the ledger.
//
// Decision D-36: allocator bubbles errors up to caller, no internal retry.
// Decision D-33: caller owns commit/rollback — Allocate ONLY works inside caller's tx.
//
// Tests:
//   - TestAllocator_Rollback: deliberate rollback after Allocate leaves no phantom row;
//     counter returns to prior value; next allocation reuses freed number (no gap).
//   - TestAllocator_RollbackMixedSequence: commit some / rollback some out of N goroutines;
//     ledger is exactly the contiguous committed set (no gaps between committed numbers).
//
// All tests run under -race and require a live PostgreSQL container (skip with -short).
package receiptno_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	dbhelpers "github.com/donnarec/donnarec-api/internal/db"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/receiptno"
	"github.com/donnarec/donnarec-api/internal/testutil"
)

// errDeliberateRollback is a sentinel error returned from a db.WithTx closure
// to trigger an explicit rollback without appearing as a "real" test failure.
var errDeliberateRollback = errors.New("deliberate rollback")

// TestAllocator_Rollback is the canonical rollback proof (NFR-04 / T-02-11):
//
//  1. Allocate a receipt number inside a db.WithTx closure that returns errDeliberateRollback
//     → db.WithTx rolls back the transaction (counter UPDATE + ledger INSERT both rolled back).
//  2. Assert the ledger contains NO row for the rolled-back running_no.
//  3. Assert the counter last_running_no is back to 0 (unchanged from before).
//  4. Run a successful allocation — it must receive running_no = 1 (reuses the freed number).
//
// This is the core proof that the counter-table approach has no gap on rollback,
// unlike PostgreSQL SEQUENCE where nextval is not transactional.
func TestAllocator_Rollback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)

	// FY 2575: unused fiscal year — guaranteed clean counter (no prior allocations).
	// May (month 5) is Jan–Sep → fiscal year = CE year + 543 = 2032 + 543 = 2575.
	issueDate := bkkTime(2032, 5, 20, 10, 0, 0) // May 20 2032 BKK → FY 2575
	const expectedFY = 2575

	// Step 1: Allocate then rollback.
	var rolledBackNo int
	rollbackErr := dbhelpers.WithTx(ctx, pool, func(tx pgx.Tx) error {
		r, err := alloc.Allocate(ctx, tx, issueDate)
		if err != nil {
			return err
		}
		rolledBackNo = r.RunningNo
		// Return sentinel error → db.WithTx rolls back the transaction.
		return errDeliberateRollback
	})
	// db.WithTx returns the closure's error (sentinel) — this is expected, not a failure.
	require.ErrorIs(t, rollbackErr, errDeliberateRollback,
		"rollback closure must return the deliberate error")
	assert.Equal(t, 1, rolledBackNo, "first attempted allocation must have been running_no=1")

	// Step 2: Assert ledger has NO row for the rolled-back running_no.
	var phantomCount int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM receipt_numbers WHERE fiscal_year = $1 AND running_no = $2`,
		expectedFY, rolledBackNo).Scan(&phantomCount)
	require.NoError(t, err)
	assert.Equal(t, 0, phantomCount,
		"ledger must contain NO row for rolled-back running_no=%d (no phantom row)", rolledBackNo)

	// Step 3: Assert counter returned to 0 (rollback undid the increment).
	var counterVal int32
	counterErr := pool.QueryRow(ctx,
		`SELECT last_running_no FROM receipt_number_counters WHERE fiscal_year = $1`,
		expectedFY).Scan(&counterVal)
	// Counter row may or may not exist after rollback (depends on whether InitCounterRow committed).
	// If it exists, its value must be 0 (rollback undid the increment).
	if counterErr == nil {
		assert.Equal(t, int32(0), counterVal,
			"counter last_running_no must be 0 after rollback (increment was undone)")
	}
	// If counter row doesn't exist, the entire tx (including InitCounterRow) was rolled back.
	// Either is correct — what matters is the next allocation gets running_no=1.

	// Step 4: Successful allocation must reuse the freed running_no (no gap).
	var reusedReceipt receiptno.AllocatedReceipt
	require.NoError(t, dbhelpers.WithTx(ctx, pool, func(tx pgx.Tx) error {
		var allocErr error
		reusedReceipt, allocErr = alloc.Allocate(ctx, tx, issueDate)
		return allocErr
	}), "post-rollback allocation must succeed")

	assert.Equal(t, 1, reusedReceipt.RunningNo,
		"next allocation after rollback must reuse running_no=1 (no gap — counter-vs-SEQUENCE distinction)")
	assert.Equal(t, expectedFY, reusedReceipt.FiscalYear,
		"reused allocation must be in FY %d", expectedFY)

	// Step 5: Assert ledger now has exactly 1 row (the committed one, not the rolled-back one).
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
	require.Len(t, ledgerNos, 1, "ledger must have exactly 1 row after rollback + 1 commit")
	assert.Equal(t, 1, ledgerNos[0],
		"ledger must contain running_no=1 (the reused, committed number)")
}

// TestAllocator_RollbackMixedSequence commits M allocations and rolls back (N-M) across
// N=30 goroutines (rollback every 3rd). It asserts:
//
//  1. No duplicate running_no in the ledger (COUNT(*) == COUNT(DISTINCT running_no)).
//  2. The ledger rows form a contiguous 1..M sequence where M = committed count.
//
// This is the "mix of committed + rolled-back txs" scenario from the RESEARCH.md rollback note.
// Counter semantics guarantee: rollback frees the number → next commit reuses it → no gap.
func TestAllocator_RollbackMixedSequence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)
	alloc := receiptno.NewAllocator(queries)

	// FY 2576: unused fiscal year to isolate from other tests.
	// Feb (month 2) is Jan–Sep → fiscal year = CE year + 543 = 2033 + 543 = 2576.
	issueDate := bkkTime(2033, 2, 10, 9, 0, 0) // Feb 10 2033 BKK → FY 2576
	const expectedFY = 2576
	const N = 30
	const rollbackEvery = 3 // rollback goroutine index i when i%rollbackEvery == 0

	// committed collects running_nos from successful allocations.
	// mu guards committed from concurrent writes (Pitfall 6 — data race under -race).
	var mu sync.Mutex
	committed := make([]int, 0, N)

	// errgroup.WithContext: if any goroutine returns a non-nil error that is not the
	// deliberate rollback sentinel, gctx2 is cancelled and Wait() returns that error.
	// Deliberate rollback errors are swallowed by each goroutine so they do not
	// cancel gctx2 prematurely (which would abort other in-flight allocations).
	g, gctx := errgroup.WithContext(ctx)
	for i := 0; i < N; i++ {
		idx := i
		g.Go(func() error {
			err := dbhelpers.WithTx(gctx, pool, func(tx pgx.Tx) error {
				r, allocErr := alloc.Allocate(gctx, tx, issueDate)
				if allocErr != nil {
					return allocErr
				}
				if idx%rollbackEvery == 0 {
					// Deliberate rollback: counter UPDATE + ledger INSERT both rolled back.
					// The freed running_no will be reused by a subsequent commit → no gap.
					return errDeliberateRollback
				}
				mu.Lock()
				committed = append(committed, r.RunningNo)
				mu.Unlock()
				return nil
			})
			// Swallow deliberate rollback sentinel — not a real failure.
			if errors.Is(err, errDeliberateRollback) {
				return nil
			}
			return err
		})
	}

	require.NoError(t, g.Wait(), "all non-rollback goroutines must succeed")

	committedCount := len(committed)
	sort.Ints(committed)

	// Assert zero duplicates.
	for i := 1; i < len(committed); i++ {
		assert.Less(t, committed[i-1], committed[i],
			"duplicate running_no detected at index %d: %d (mixed rollback scenario)",
			i, committed[i-1])
	}

	// Assert contiguous 1..committedCount.
	// Rollback semantics: freed numbers are reused by subsequent commits → contiguous ledger.
	for i, no := range committed {
		assert.Equal(t, i+1, no,
			"mixed rollback: running_no at position %d must be %d (contiguous), got %d",
			i, i+1, no)
	}

	// Assert via raw SQL: COUNT(*) == committedCount, COUNT(DISTINCT running_no) == committedCount.
	var totalCount, distinctCount int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*), COUNT(DISTINCT running_no) FROM receipt_numbers WHERE fiscal_year = $1`,
		expectedFY).Scan(&totalCount, &distinctCount)
	require.NoError(t, err)
	assert.Equal(t, committedCount, totalCount,
		"ledger must have exactly %d rows (committed count)", committedCount)
	assert.Equal(t, committedCount, distinctCount,
		"COUNT(DISTINCT running_no) must equal COUNT(*) — no duplicates in ledger")

	// Assert ledger rows are contiguous 1..committedCount.
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
		"ledger ORDER BY running_no must match committed set exactly (contiguous, no gaps)")
}
