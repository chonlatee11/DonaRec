// Package receiptno — allocator.go
//
// Allocator is the single code path that may hand out a receipt number (D-35).
// It implements the caller-managed transaction pattern (D-33): callers own the
// pgx.Tx and drive commit/rollback — Allocate only performs work within it.
//
// Allocation steps (per D-32, D-41, D-42):
//  1. Compute fiscal year from issueDate (pure function, no DB call).
//  2. Bind sqlc queries to the caller's tx via queries.WithTx(tx).
//  3. Lock the counter row FOR UPDATE (LockCounterForUpdate).
//     If pgx.ErrNoRows → new fiscal year: call InitCounterRow, then re-lock.
//  4. Increment the counter (IncrementCounter) — safe because lock is held.
//  5. Read format config (GetReceiptNumberConfig) inside the same tx (D-32).
//  6. Render the formatted string snapshot (formatReceiptNo).
//  7. Insert the ledger row (InsertReceiptNumberLedger) — UNIQUE backstop fires here (D-37).
//  8. Return AllocatedReceipt with values frozen from the ledger row (D-42).
//
// Anti-patterns explicitly absent (threat register T-02-07/T-02-08/T-02-09):
//   - NO tx.Commit / tx.Rollback (caller-managed, D-33)
//   - NO time.Now() (issueDate is caller-supplied, D-40)
//   - NO MAX(running_no) (Pitfall 3, T-02-08)
//   - NO pgxpool.Pool in signature (Pitfall 2, T-02-07)
package receiptno

import (
	"context"
	"errors"
	"fmt"
	"time"

	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/jackc/pgx/v5"
)

// AllocatedReceipt is the value returned by Allocate.
// All fields are frozen snapshots from the moment of allocation (D-42):
// Formatted is persisted to the ledger and will never change even if config changes.
type AllocatedReceipt struct {
	// FiscalYear is the Thai Buddhist Era fiscal year (e.g. 2569).
	FiscalYear int
	// RunningNo is the sequential number within the fiscal year (e.g. 1).
	RunningNo int
	// Formatted is the frozen formatted receipt number string (e.g. "2569/000001").
	// This is the string persisted in receipt_numbers.formatted (D-42).
	Formatted string
	// AllocatedAt is the DB-side timestamp from the ledger INSERT (not time.Now()).
	AllocatedAt time.Time
}

// Allocator holds the sqlc queries used for counter/ledger/config operations.
// It exposes a single method: Allocate(ctx, tx, issueDate).
//
// The queries field is *db.Queries (concrete) rather than db.Querier (interface)
// because (*db.Queries).WithTx returns *db.Queries, not db.Querier — the bound
// query object (qtx) must be *db.Queries for the call chain to work correctly
// (Key Observation #1, 02-PATTERNS.md).
type Allocator struct {
	queries *db.Queries
}

// NewAllocator constructs an Allocator with the given sqlc Queries.
// Panics if queries is nil — this is a programming-error guard (caller configuration bug).
func NewAllocator(queries *db.Queries) *Allocator {
	if queries == nil {
		panic("receiptno.NewAllocator: queries must not be nil")
	}
	return &Allocator{queries: queries}
}

// Allocate assigns the next gap-less receipt number for the fiscal year derived
// from issueDate and records it in the ledger — all within the caller's transaction.
//
// The caller MUST:
//   - Supply an open pgx.Tx (not a pool).
//   - Run the tx at READ COMMITTED isolation (see below).
//   - Commit or roll back the transaction after Allocate returns.
//   - Roll back on error to undo both the counter increment and the ledger insert.
//
// Isolation requirement (READ COMMITTED): the gap-less + non-blocking serialization
// guarantee depends on the caller's tx running at READ COMMITTED — pgxpool's
// pool.Begin (used by db.WithTx) defaults to this, so the standard path is safe.
// Under REPEATABLE READ or SERIALIZABLE the re-lock (LockCounterForUpdate /
// IncrementCounter) after a concurrent commit raises SQLSTATE 40001
// ("could not serialize access due to concurrent update") instead of re-reading
// the committed value, and Allocate does NOT retry (D-36) — a concurrent approval
// would surface as a hard failure. Do NOT call Allocate from a stricter-isolation tx.
//
// Allocate MUST NOT be called with a nil tx — the UNIQUE constraint backstop and
// the counter rollback-safety both depend on counter + ledger being in the same tx.
//
// On success, returns AllocatedReceipt with Formatted equal to the value stored in
// the ledger (D-42). On error, returns an empty AllocatedReceipt and a wrapped error.
func (a *Allocator) Allocate(ctx context.Context, tx pgx.Tx, issueDate time.Time) (AllocatedReceipt, error) {
	// Step 1: Compute fiscal year (pure function — no DB call, no time.Now()).
	fy := fiscalYear(issueDate)

	// Step 2: Bind sqlc queries to the caller's transaction.
	// All subsequent DB calls go through qtx so they participate in the caller's tx.
	qtx := a.queries.WithTx(tx)

	// Step 3: Lock the counter row FOR UPDATE.
	// This serializes concurrent allocations for the same fiscal year:
	// only one transaction holds this lock at a time (NFR-04).
	_, err := qtx.LockCounterForUpdate(ctx, int32(fy))
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return AllocatedReceipt{}, fmt.Errorf("lock counter row: %w", err)
		}
		// ErrNoRows means this is the first allocation for a new fiscal year (D-41).
		// Init the counter row (ON CONFLICT DO NOTHING — safe under concurrent first-alloc).
		if initErr := qtx.InitCounterRow(ctx, int32(fy)); initErr != nil {
			return AllocatedReceipt{}, fmt.Errorf("init counter row: %w", initErr)
		}
		// Re-lock after init: the row now exists, so FOR UPDATE acquires the lock.
		// Concurrent sessions that also did InitCounterRow will block here until
		// this tx commits, then proceed with the correct serialized order (Pitfall 1).
		if _, lockErr := qtx.LockCounterForUpdate(ctx, int32(fy)); lockErr != nil {
			return AllocatedReceipt{}, fmt.Errorf("lock counter row (after init): %w", lockErr)
		}
	}

	// Step 4: Increment the counter while holding the FOR UPDATE lock.
	// IncrementCounter does UPDATE … SET last_running_no = last_running_no + 1 RETURNING …
	next, err := qtx.IncrementCounter(ctx, int32(fy))
	if err != nil {
		return AllocatedReceipt{}, fmt.Errorf("increment counter: %w", err)
	}

	// Step 5: Read format config inside the same tx (D-32).
	// Reading within the tx ensures config is consistent with this allocation's snapshot.
	cfg, err := qtx.GetReceiptNumberConfig(ctx)
	if err != nil {
		return AllocatedReceipt{}, fmt.Errorf("get receipt number config: %w", err)
	}

	// Step 6: Render the frozen formatted string from primitive components (D-42).
	// formatReceiptNo accepts primitives (not the sqlc row) for wave-independent testability.
	formatted := formatReceiptNo(
		fy,
		int(next),
		cfg.Separator,
		int(cfg.RunningNoPadding),
		cfg.YearFormat,
		cfg.Prefix,
	)

	// Step 7: Insert the ledger row — this is where the number is "born" (D-35).
	// The UNIQUE(fiscal_year, running_no) constraint fires here if a logic bug
	// produces a duplicate, providing a last-line-of-defense backstop (D-37).
	ledger, err := qtx.InsertReceiptNumberLedger(ctx, db.InsertReceiptNumberLedgerParams{
		FiscalYear: int32(fy),
		RunningNo:  next,
		Formatted:  formatted,
	})
	if err != nil {
		return AllocatedReceipt{}, fmt.Errorf("insert receipt number ledger: %w", err)
	}

	// Step 8: Return the frozen snapshot from the ledger row (D-42).
	// AllocatedAt comes from the DB-side now() in the INSERT, not from time.Now().
	return AllocatedReceipt{
		FiscalYear:  int(ledger.FiscalYear),
		RunningNo:   int(ledger.RunningNo),
		Formatted:   ledger.Formatted,
		AllocatedAt: ledger.AllocatedAt.Time,
	}, nil
}
