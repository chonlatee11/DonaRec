// Package audit_test tests the AuditService hash-chain implementation.
package audit_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donnarec/donnarec-api/internal/audit"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/testutil"
	"go.uber.org/zap"
)

// TestHashChainVerification verifies:
//  1. Appending 5 sequential entries produces a valid chain (VerifyChain → true).
//  2. Directly tampering one row's action column (via superuser) causes VerifyChain
//     to return (false, brokenID) identifying the corrupted row (D-17, T-1-audit-02).
func TestHashChainVerification(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	superPool, _ := testutil.SetupTestPostgresAsAppRole(t)
	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	queries := db.New(superPool)
	svc := audit.NewAuditService(superPool, queries, logger)

	// Append 5 sequential audit entries
	for i := 0; i < 5; i++ {
		err := svc.AppendAuditEntry(ctx, audit.AuditEntry{
			ActorID:    "00000000-0000-0000-0000-000000000001",
			ActorEmail: "chain-test@example.com",
			Action:     "user.create",
			Resource:   "/api/admin/users",
			IPAddress:  "127.0.0.1",
		})
		require.NoError(t, err, "AppendAuditEntry #%d must succeed", i+1)
	}

	// VerifyChain must return true on a clean chain
	ok, brokenID, err := svc.VerifyChain(ctx)
	require.NoError(t, err, "VerifyChain must not error on clean chain")
	assert.True(t, ok, "VerifyChain must return true for an unmodified chain")
	assert.Equal(t, int64(0), brokenID, "brokenID must be 0 for a valid chain")

	// Tamper: update action on the 3rd row (id = 3) via superuser
	// (donnarec_app cannot UPDATE — we use superPool for this privileged attack simulation)
	_, err = superPool.Exec(ctx,
		`UPDATE audit_log SET action = 'TAMPERED' WHERE id = 3`)
	require.NoError(t, err, "superuser can tamper with audit_log for test purposes")

	// VerifyChain must detect the tampering and return false + broken id
	ok2, broken2, err2 := svc.VerifyChain(ctx)
	require.NoError(t, err2, "VerifyChain must not error even on tampered chain")
	assert.False(t, ok2, "VerifyChain must return false when a row is tampered")
	assert.Equal(t, int64(3), broken2, "VerifyChain must identify id=3 as the broken row")
}
