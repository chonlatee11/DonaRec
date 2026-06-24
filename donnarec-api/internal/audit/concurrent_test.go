// Package audit_test tests concurrent audit log insertion for chain integrity.
package audit_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donnarec/donnarec-api/internal/audit"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/testutil"
	"go.uber.org/zap"
)

// TestConcurrentAuditInserts verifies that 50 goroutines concurrently appending
// audit entries produce a valid, gap-free hash chain with no duplicate prev_hash
// values (Pitfall 2 mitigation, T-1-audit-conc, NFR-05).
//
// Run with -race to catch data races.
func TestConcurrentAuditInserts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	superPool, _ := testutil.SetupTestPostgresAsAppRole(t)
	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	queries := db.New(superPool)
	svc := audit.NewAuditService(superPool, queries, logger)

	const goroutines = 50

	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = svc.AppendAuditEntry(ctx, audit.AuditEntry{
				ActorID:    "00000000-0000-0000-0000-000000000002",
				ActorEmail: "concurrent@example.com",
				Action:     "concurrent.insert",
				Resource:   "/api/test",
				IPAddress:  "10.0.0.1",
			})
		}(i)
	}

	wg.Wait()

	// All goroutines must succeed
	for i, err := range errs {
		assert.NoError(t, err, "goroutine %d must not error", i)
	}

	// Verify row count = 50
	var count int
	err := superPool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_log`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, goroutines, count, "must have exactly 50 rows in audit_log")

	// Verify no duplicate prev_hash (chain linkage is unique per insertion order)
	var dupCount int
	err = superPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
			SELECT prev_hash, COUNT(*) c
			FROM audit_log
			GROUP BY prev_hash
			HAVING COUNT(*) > 1
		) dups
	`).Scan(&dupCount)
	require.NoError(t, err)
	assert.Equal(t, 0, dupCount,
		"no prev_hash must appear more than once (each is linked to a unique previous row)")

	// VerifyChain must return true on all 50 rows
	ok, brokenID, err := svc.VerifyChain(ctx)
	require.NoError(t, err)
	assert.True(t, ok, "VerifyChain must be true after 50 concurrent inserts")
	assert.Equal(t, int64(0), brokenID, "brokenID must be 0 for valid chain")
}
