// Package audit_test tests the audit package using external test package convention.
package audit_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donnarec/donnarec-api/internal/testutil"
)

// TestAuditImmutability verifies that the app role (donnarec_app) cannot UPDATE
// or DELETE rows in the audit_log table (D-17, T-1-audit-01, NFR-05).
//
// We use the superuser pool to INSERT a seed row (superuser can insert),
// then connect as donnarec_app (a restricted non-owner role) and verify that
// UPDATE and DELETE are rejected with "permission denied".
//
// Background: In PostgreSQL, the table OWNER bypasses REVOKE grants. The test
// container creates the table as user 'test' (superuser), so we must connect
// with a separate non-owner role (donnarec_app) to validate REVOKE enforcement.
func TestAuditImmutability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	superPool, appPool := testutil.SetupTestPostgresAsAppRole(t)
	ctx := context.Background()

	// Insert a seed row via the SUPERUSER pool (INSERT must succeed for superuser)
	var insertedID int64
	err := superPool.QueryRow(ctx, `
		INSERT INTO audit_log (
			actor_id, actor_email, action, resource,
			prev_hash, row_hash
		) VALUES (
			gen_random_uuid(), 'test@example.com', 'user.create', '/api/admin/users',
			'GENESIS', 'placeholder_hash'
		) RETURNING id
	`).Scan(&insertedID)
	require.NoError(t, err, "superuser INSERT into audit_log must succeed")
	assert.Greater(t, insertedID, int64(0), "inserted id must be positive")

	// Attempt UPDATE via the RESTRICTED donnarec_app pool — must be denied
	_, updateErr := appPool.Exec(ctx, `
		UPDATE audit_log SET action = 'tampered' WHERE id = $1
	`, insertedID)
	require.Error(t, updateErr, "donnarec_app UPDATE on audit_log must be rejected (D-17)")
	assert.Contains(t, updateErr.Error(), "permission denied",
		"UPDATE error must be a permission-denied error, got: %v", updateErr)

	// Attempt DELETE via the RESTRICTED donnarec_app pool — must also be denied
	_, deleteErr := appPool.Exec(ctx, `
		DELETE FROM audit_log WHERE id = $1
	`, insertedID)
	require.Error(t, deleteErr, "donnarec_app DELETE on audit_log must be rejected (D-17)")
	assert.Contains(t, deleteErr.Error(), "permission denied",
		"DELETE error must be a permission-denied error, got: %v", deleteErr)

	// Verify that SELECT still works for donnarec_app (needed for VerifyChain)
	var count int
	err = appPool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_log`).Scan(&count)
	require.NoError(t, err, "donnarec_app SELECT on audit_log must succeed")
	assert.Equal(t, 1, count, "exactly 1 row should be present")

	// Verify that INSERT still works for donnarec_app (needed for AppendAuditEntry)
	var secondID int64
	err = appPool.QueryRow(ctx, `
		INSERT INTO audit_log (
			actor_id, actor_email, action, resource,
			prev_hash, row_hash
		) VALUES (
			gen_random_uuid(), 'app@example.com', 'user.list', '/api/admin/users',
			'placeholder_hash', 'row_hash_2'
		) RETURNING id
	`).Scan(&secondID)
	require.NoError(t, err, "donnarec_app INSERT into audit_log must succeed")
	assert.Greater(t, secondID, insertedID, "second row id must be greater than first")
}

// TestAuditRetainColumns verifies the audit_log table has the required columns
// for retention policy enforcement (created_at + prev_hash + row_hash).
func TestAuditRetainColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	// Query information_schema to verify columns exist
	rows, err := pool.Query(ctx, `
		SELECT column_name, is_nullable
		FROM information_schema.columns
		WHERE table_name = 'audit_log'
		ORDER BY ordinal_position
	`)
	require.NoError(t, err)
	defer rows.Close()

	columnSet := make(map[string]string) // column_name → is_nullable
	for rows.Next() {
		var colName, isNullable string
		require.NoError(t, rows.Scan(&colName, &isNullable))
		columnSet[colName] = isNullable
	}
	require.NoError(t, rows.Err())

	// Required columns
	required := []string{"id", "actor_id", "actor_email", "action", "resource",
		"before_json", "after_json", "ip_address", "created_at",
		"prev_hash", "row_hash"}
	for _, col := range required {
		assert.Contains(t, columnSet, col, "audit_log must have column %q", col)
	}

	// prev_hash and row_hash must be NOT NULL
	assert.Equal(t, "NO", columnSet["prev_hash"], "prev_hash must be NOT NULL")
	assert.Equal(t, "NO", columnSet["row_hash"], "row_hash must be NOT NULL")
}
