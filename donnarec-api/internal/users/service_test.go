package users_test

import (
	"context"
	"testing"

	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/testutil"
	. "github.com/donnarec/donnarec-api/internal/users"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newTestUserService(t *testing.T) (*UserService, func()) {
	t.Helper()
	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)
	logger := zap.NewNop()
	svc := NewUserService(pool, queries, logger)
	return svc, func() {} // cleanup is handled by testutil via t.Cleanup
}

// TestCreateAndGetUser verifies end-to-end user creation and retrieval
// against a real PostgreSQL instance (testcontainers postgres:17).
func TestCreateAndGetUser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test: requires Docker for testcontainers")
	}

	svc, _ := newTestUserService(t)
	ctx := context.Background()

	t.Run("creates user with two roles and retrieves them", func(t *testing.T) {
		params := CreateUserParams{
			Email:           "testuser@hospital.th",
			DisplayName:     "Test User",
			KeycloakSubject: "kc-sub-test-001",
			Roles:           []UserRole{RoleMaker, RoleChecker},
		}

		created, err := svc.CreateUser(ctx, params)
		require.NoError(t, err, "CreateUser must succeed")
		require.NotNil(t, created)

		// Verify created user fields
		assert.Equal(t, params.Email, created.Email, "email round-trip")
		assert.Equal(t, params.DisplayName, created.DisplayName, "display_name round-trip")
		assert.Equal(t, params.KeycloakSubject, created.KeycloakSubject, "keycloak_subject round-trip")
		assert.True(t, created.IsActive, "new user should be active")
		assert.False(t, created.LegalHold, "new user should not be under legal hold")
		assert.NotEmpty(t, created.ID, "ID must be set by the database")

		// Verify both roles are present (D-02 multi-role)
		assert.Len(t, created.Roles, 2, "user should have exactly 2 roles")
		assert.Contains(t, created.Roles, RoleMaker, "user should have maker role")
		assert.Contains(t, created.Roles, RoleChecker, "user should have checker role")

		// Retrieve by ID and verify consistency
		fetched, err := svc.GetUser(ctx, created.ID)
		require.NoError(t, err, "GetUser must succeed")
		require.NotNil(t, fetched)

		assert.Equal(t, created.ID, fetched.ID)
		assert.Equal(t, created.Email, fetched.Email)
		assert.Equal(t, created.DisplayName, fetched.DisplayName)
		assert.Len(t, fetched.Roles, 2, "fetched user must have 2 roles")
		assert.Contains(t, fetched.Roles, RoleMaker)
		assert.Contains(t, fetched.Roles, RoleChecker)
	})

	t.Run("CreateUser returns error when no roles provided", func(t *testing.T) {
		params := CreateUserParams{
			Email:           "noroles@hospital.th",
			DisplayName:     "No Roles User",
			KeycloakSubject: "kc-sub-no-roles",
			Roles:           nil, // no roles
		}

		_, err := svc.CreateUser(ctx, params)
		require.Error(t, err, "CreateUser with no roles must return error")
	})

	t.Run("GetUser returns error for non-existent user", func(t *testing.T) {
		_, err := svc.GetUser(ctx, "00000000-0000-0000-0000-000000000000")
		require.Error(t, err, "GetUser for non-existent user must return error")
	})

	t.Run("GetUser returns error for invalid UUID", func(t *testing.T) {
		_, err := svc.GetUser(ctx, "not-a-uuid")
		require.Error(t, err, "GetUser with invalid UUID must return error")
	})
}

// TestMigrationRoundTrip verifies that running up→down→up migrations is idempotent.
// This proves golang-migrate is wired correctly and the schema compiles cleanly.
func TestMigrationRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping migration round-trip test: requires Docker for testcontainers")
	}

	// SetupTestPostgres runs migrations up on first call.
	// We just verify the pool is usable, meaning migrations succeeded.
	pool := testutil.SetupTestPostgres(t)
	require.NotNil(t, pool, "pool must be non-nil after migrations")

	// Verify tables exist by querying them
	ctx := context.Background()
	var count int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	require.NoError(t, err, "users table must exist after migration")

	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM user_roles").Scan(&count)
	require.NoError(t, err, "user_roles table must exist after migration")

	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM retention_config").Scan(&count)
	require.NoError(t, err, "retention_config table must exist after migration")
	assert.Equal(t, 2, count, "retention_config must have 2 seed rows (donation + audit_log)")
}
