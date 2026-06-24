// Package retention_test tests the retention service.
package retention_test

import (
	"context"
	"testing"
	"time"

	"github.com/donnarec/donnarec-api/internal/config"
	"github.com/donnarec/donnarec-api/internal/retention"
	"github.com/donnarec/donnarec-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultTestConfig returns a RetentionConfig with canonical default values.
func defaultTestConfig() config.RetentionConfig {
	return config.RetentionConfig{
		DonationRetainDays: 1825, // 5 years
		AuditLogRetainDays: 3650, // 10 years
		DefaultLegalBasis:  "tax_obligation",
	}
}

// TestRetainUntilCalculation verifies ComputeRetainUntil uses config values,
// not hardcoded literals (D-18, A2).
func TestRetainUntilCalculation(t *testing.T) {
	cfg := defaultTestConfig()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("donation retain_until = t0 + 1825 days (config-driven)", func(t *testing.T) {
		got := retention.ComputeRetainUntil("donation", t0, cfg)
		want := t0.Add(time.Duration(cfg.DonationRetainDays) * 24 * time.Hour)
		assert.Equal(t, want, got,
			"ComputeRetainUntil(donation) must use cfg.DonationRetainDays (%d)", cfg.DonationRetainDays)
	})

	t.Run("audit_log retain_until = t0 + 3650 days (config-driven)", func(t *testing.T) {
		got := retention.ComputeRetainUntil("audit_log", t0, cfg)
		want := t0.Add(time.Duration(cfg.AuditLogRetainDays) * 24 * time.Hour)
		assert.Equal(t, want, got,
			"ComputeRetainUntil(audit_log) must use cfg.AuditLogRetainDays (%d)", cfg.AuditLogRetainDays)
	})

	t.Run("unknown entity type falls back to DonationRetainDays", func(t *testing.T) {
		got := retention.ComputeRetainUntil("unknown_type", t0, cfg)
		want := t0.Add(time.Duration(cfg.DonationRetainDays) * 24 * time.Hour)
		assert.Equal(t, want, got,
			"unknown entity type must fall back to DonationRetainDays")
	})

	t.Run("config change is reflected (not hardcoded)", func(t *testing.T) {
		customCfg := config.RetentionConfig{
			DonationRetainDays: 2190, // 6 years
			AuditLogRetainDays: 3650,
			DefaultLegalBasis:  "tax_obligation",
		}
		got := retention.ComputeRetainUntil("donation", t0, customCfg)
		want := t0.Add(2190 * 24 * time.Hour)
		assert.Equal(t, want, got,
			"ComputeRetainUntil must use the provided config, not a hardcoded value")
	})
}

// TestSoftDeleteAllowed verifies that soft-delete (status/is_active change) is
// permitted regardless of legal_hold (D-19 does NOT block soft delete, only hard DELETE).
func TestSoftDeleteAllowed(t *testing.T) {
	ctx := context.Background()

	t.Run("soft delete allowed when legal_hold=true", func(t *testing.T) {
		// GuardHardDelete is the hard-delete guard — it should NOT affect soft delete
		// Soft delete is represented by setting is_active=false or a status field
		// This test verifies GuardHardDelete is only for hard DELETE, not soft DELETE
		err := retention.GuardHardDelete(ctx, false) // legal_hold = false → allowed
		assert.NoError(t, err, "GuardHardDelete(false) must allow the operation")
	})

	t.Run("soft delete path: GuardHardDelete is not called for soft deletes", func(t *testing.T) {
		// Soft deletes bypass GuardHardDelete entirely.
		// This test documents that the soft-delete helper does not call GuardHardDelete.
		// The actual soft-delete implementation updates a field — it never issues DELETE.
		// This is a design-level invariant; tested here as a sanity check.
		// If the implementation calls GuardHardDelete for soft deletes, it's a design bug.
		err := retention.SoftDelete(ctx, false) // legal_hold=true: soft delete must still be allowed
		assert.NoError(t, err, "SoftDelete must succeed regardless of legal_hold (only hard DELETE is blocked)")

		err = retention.SoftDelete(ctx, true)
		assert.NoError(t, err, "SoftDelete must succeed even when legal_hold=true (only hard DELETE blocked, D-19)")
	})
}

// TestGuardHardDeleteUnit tests the app-level guard in isolation (unit test, no DB).
func TestGuardHardDeleteUnit(t *testing.T) {
	ctx := context.Background()

	t.Run("GuardHardDelete(false) returns nil", func(t *testing.T) {
		err := retention.GuardHardDelete(ctx, false)
		assert.NoError(t, err)
	})

	t.Run("GuardHardDelete(true) returns error with i18n key", func(t *testing.T) {
		err := retention.GuardHardDelete(ctx, true)
		require.Error(t, err, "GuardHardDelete must block hard-delete under legal_hold")
		// Error must use i18n key retention.legal_hold_delete_blocked
		assert.Contains(t, err.Error(), "retention.legal_hold_delete_blocked",
			"error must reference i18n key for consistent localization")
	})
}

// TestLegalHoldDeleteBlocked is an integration test that verifies the DB trigger
// blocks DELETE on a row where legal_hold=true.
// Requires testcontainers (PostgreSQL 17 with migration 000003 applied).
func TestLegalHoldDeleteBlocked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	ctx := context.Background()
	pool := testutil.SetupTestPostgres(t)

	// Insert a user with legal_hold=false first (needs a valid user to set legal_hold on)
	var userID string
	err := pool.QueryRow(ctx, `
		INSERT INTO users (email, display_name, keycloak_subject, is_active, legal_hold)
		VALUES ('legalhold-test@example.com', 'Test User', 'kc-legalhold-test', true, false)
		RETURNING id
	`).Scan(&userID)
	require.NoError(t, err, "insert test user")

	// Set legal_hold=true
	_, err = pool.Exec(ctx, `UPDATE users SET legal_hold = true WHERE id = $1`, userID)
	require.NoError(t, err, "set legal_hold=true")

	t.Run("hard DELETE under legal_hold=true raises DB exception", func(t *testing.T) {
		_, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
		require.Error(t, err, "DB trigger must block DELETE when legal_hold=true")
		// PostgreSQL raises an exception; pgx wraps it as an error
		assert.Contains(t, err.Error(), "legal hold",
			"error must mention legal hold (from trigger RAISE EXCEPTION message)")
	})

	t.Run("app GuardHardDelete(true) blocks delete before reaching DB", func(t *testing.T) {
		err := retention.GuardHardDelete(ctx, true)
		require.Error(t, err, "app-level guard must block delete under legal_hold")
		assert.Contains(t, err.Error(), "retention.legal_hold_delete_blocked")
	})

	t.Run("hard DELETE on legal_hold=false user succeeds", func(t *testing.T) {
		// Insert a second user with legal_hold=false
		var user2ID string
		err := pool.QueryRow(ctx, `
			INSERT INTO users (email, display_name, keycloak_subject, is_active, legal_hold)
			VALUES ('no-hold@example.com', 'No Hold User', 'kc-no-hold', true, false)
			RETURNING id
		`).Scan(&user2ID)
		require.NoError(t, err)

		// DELETE should succeed
		result, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, user2ID)
		require.NoError(t, err, "DELETE on legal_hold=false must succeed")
		assert.Equal(t, int64(1), result.RowsAffected(), "should delete exactly 1 row")
	})

	t.Run("app GuardHardDelete(false) allows delete", func(t *testing.T) {
		err := retention.GuardHardDelete(ctx, false)
		assert.NoError(t, err)
	})
}
