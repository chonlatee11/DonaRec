// Package testutil provides shared test infrastructure for donnarec-api integration tests.
package testutil

import (
	"context"
	"fmt"
	"strings"
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

// SetupTestPostgresAsAppRole runs the same setup as SetupTestPostgres (superuser runs
// migrations), then returns a *second* pool connected as the 'donnarec_app' restricted
// role. This pool proves DB-level REVOKE behaviour: the app role cannot UPDATE/DELETE
// audit_log even though superuser created the table (D-17, T-1-audit-01).
//
// Usage:
//
//	_, appPool := testutil.SetupTestPostgresAsAppRole(t)
//
// The first return value is the superuser pool (useful for privileged operations like
// seeding test data); the second is the restricted app-role pool.
func SetupTestPostgresAsAppRole(t *testing.T) (*pgxpool.Pool, *pgxpool.Pool) {
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

	// Superuser pool (runs migrations, seeds data in tests)
	superPool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err, "failed to create superuser pgxpool")
	t.Cleanup(func() { superPool.Close() })

	// Run migrations (creates donnarec_app role + REVOKE)
	migrateURL := "pgx5://" + connStr[len("postgres://"):]
	m, err := migrate.New("file://../../migrations", migrateURL)
	require.NoError(t, err, "failed to create migrator")
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migration up failed: %v", err)
	}

	// Build connection string for donnarec_app role
	// Replace "test:test@" with "donnarec_app:donnarec_app_test@" in connStr
	appConnStr := strings.Replace(connStr, "test:test@", "donnarec_app:donnarec_app_test@", 1)
	// Extract host:port from connStr for building the app pool URL
	// connStr looks like: postgres://test:test@host:port/donnarec_test?sslmode=disable
	appConnStr = fmt.Sprintf("%s", appConnStr)

	appPool, err := pgxpool.New(ctx, appConnStr)
	require.NoError(t, err, "failed to create donnarec_app pgxpool")
	t.Cleanup(func() { appPool.Close() })

	return superPool, appPool
}
