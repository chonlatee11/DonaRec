// Package testutil provides shared test infrastructure for donnarec-api integration tests.
package testutil

import (
	"context"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// SetupTestPostgres spins up a PostgreSQL 17 testcontainer, runs all migrations,
// and returns a ready pgxpool.Pool. The container is terminated on test cleanup.
//
// Usage in test files:
//
//	pool := testutil.SetupTestPostgres(t)
func SetupTestPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:17",
		postgres.WithDatabase("donnarec_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	require.NoError(t, err, "failed to start postgres container")

	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("warning: failed to terminate postgres container: %v", err)
		}
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "failed to get connection string")

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err, "failed to create pgxpool")

	t.Cleanup(func() {
		pool.Close()
	})

	// Run migrations using golang-migrate pgx/v5 driver
	// NOTE: migration path is relative to the test binary location; testcontainers
	// tests are run from the package directory. The migrations directory is at
	// ../../migrations relative to each internal/* package.
	migrateURL := "pgx5://" + connStr[len("postgres://"):]
	m, err := migrate.New("file://../../migrations", migrateURL)
	require.NoError(t, err, "failed to create migrator")

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migration up failed: %v", err)
	}

	return pool
}
