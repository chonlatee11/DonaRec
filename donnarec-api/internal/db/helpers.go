// Package db provides database helpers for donnarec-api.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WithTx executes fn inside a single PostgreSQL transaction.
// If fn returns an error, the transaction is rolled back; otherwise it is committed.
// The deferred rollback is a no-op after a successful commit (pgx ignores it).
//
// Pattern B (Shared Pattern) — used by every service that needs atomicity.
//
// Usage:
//
//	err := db.WithTx(ctx, pool, func(tx pgx.Tx) error {
//	    qtx := queries.WithTx(tx)
//	    _, err := qtx.CreateUser(ctx, params)
//	    return err
//	})
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback is a best-effort cleanup; commit error is returned

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
