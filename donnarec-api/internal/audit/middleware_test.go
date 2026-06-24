// Package audit_test tests the AuditMiddleware.
package audit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/donnarec/donnarec-api/internal/audit"
	"github.com/donnarec/donnarec-api/internal/auth"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/testutil"
)

// setupMiddlewareTestRouter creates a minimal Gin router with AuditMiddleware
// and registers routes for testing.
func setupMiddlewareTestRouter(t *testing.T, svc *audit.AuditService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(audit.AuditMiddleware(svc))

	// Inject fake claims so the middleware can extract the actor
	fakeClaimsMiddleware := func(c *gin.Context) {
		c.Set("claims", auth.KeycloakClaims{
			Subject: "00000000-0000-0000-0000-000000000099",
			Email:   "testactor@example.com",
			RealmAccess: auth.RealmRoles{
				Roles: []string{"maker"},
			},
		})
		c.Next()
	}

	// Mutation routes (should be audited)
	r.POST("/api/items", fakeClaimsMiddleware, func(c *gin.Context) {
		c.Set("audit_after", map[string]string{"id": "42", "name": "test"})
		c.Status(http.StatusCreated)
	})
	r.PUT("/api/items/:id", fakeClaimsMiddleware, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.PATCH("/api/items/:id", fakeClaimsMiddleware, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.DELETE("/api/items/:id", fakeClaimsMiddleware, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Read-only route (should NOT be audited)
	r.GET("/api/items", fakeClaimsMiddleware, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// PII-reveal route (GET that SHOULD be audited)
	r.GET("/api/donors/:id/reveal", fakeClaimsMiddleware, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	return r
}

// countAuditRows counts all rows in audit_log for assertion.
func countAuditRows(t *testing.T, pool interface{ QueryRow(ctx context.Context, sql string, args ...any) interface{ Scan(dest ...any) error } }, ctx context.Context) int {
	t.Helper()
	// Use direct SQL via the pool we received
	return 0 // placeholder — will use pgxpool directly
}

// TestAuditMiddlewareCoverage verifies:
//   - POST/PUT/PATCH/DELETE through the test router writes exactly 1 audit row each.
//   - A plain GET writes 0 audit rows (FR-13, D-15).
func TestAuditMiddlewareCoverage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()
	logger, _ := zap.NewDevelopment()
	queries := db.New(pool)
	svc := audit.NewAuditService(pool, queries, logger)

	router := setupMiddlewareTestRouter(t, svc)

	mutations := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/items"},
		{http.MethodPut, "/api/items/1"},
		{http.MethodPatch, "/api/items/1"},
		{http.MethodDelete, "/api/items/1"},
	}

	for i, m := range mutations {
		var body *strings.Reader
		if m.method == http.MethodPost {
			body = strings.NewReader(`{"name":"test"}`)
		} else {
			body = strings.NewReader("")
		}
		req := httptest.NewRequest(m.method, m.path, body)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		// Check audit row count after each mutation
		var count int
		err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_log`).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, i+1, count,
			"after %s %s: expected %d audit rows", m.method, m.path, i+1)
	}

	// GET /api/items — should NOT write an audit row
	rowsBefore := 4
	req := httptest.NewRequest(http.MethodGet, "/api/items", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var countAfterGet int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_log`).Scan(&countAfterGet)
	require.NoError(t, err)
	assert.Equal(t, rowsBefore, countAfterGet,
		"GET /api/items must not write an audit row (non-reveal GET)")

	// Verify action field for a mutation entry
	var action, resource string
	err = pool.QueryRow(ctx, `
		SELECT action, resource FROM audit_log ORDER BY id LIMIT 1
	`).Scan(&action, &resource)
	require.NoError(t, err)
	assert.NotEmpty(t, action, "audit action must be set")
	assert.NotEmpty(t, resource, "audit resource must be set")
}

// TestPIIRevealAudit verifies that a GET to a PII-reveal endpoint writes
// exactly one audit row (D-13 mechanism — the reveal action is always logged).
func TestPIIRevealAudit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()
	logger, _ := zap.NewDevelopment()
	queries := db.New(pool)
	svc := audit.NewAuditService(pool, queries, logger)

	router := setupMiddlewareTestRouter(t, svc)

	// GET /api/donors/:id/reveal — tagged as PII reveal endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/donors/123/reveal", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "reveal endpoint must return 200")

	var count int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_log`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count,
		"PII-reveal GET must write exactly 1 audit row (D-13)")

	// Verify action contains "pii.reveal" or similar
	var action string
	err = pool.QueryRow(ctx, `SELECT action FROM audit_log LIMIT 1`).Scan(&action)
	require.NoError(t, err)
	assert.Contains(t, action, "reveal",
		"audit action for PII reveal must identify the reveal operation, got %q", action)
}

// TestAuditMiddlewareNoAbortOnError verifies that if the audit write fails,
// the middleware does NOT abort the user request (it logs the error but continues).
// This test uses a closed/nil pool to force an audit write failure.
func TestAuditMiddlewareNoAbortOnError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testutil.SetupTestPostgres(t)
	logger, _ := zap.NewDevelopment()
	queries := db.New(pool)
	svc := audit.NewAuditService(pool, queries, logger)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(audit.AuditMiddleware(svc))

	// Close the pool to force audit write failure
	pool.Close()

	r.POST("/api/test", func(c *gin.Context) {
		c.Set("claims", auth.KeycloakClaims{
			Subject: "00000000-0000-0000-0000-000000000001",
			Email:   "fail@example.com",
		})
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/test", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// The handler response must still come through (not aborted by middleware)
	assert.Equal(t, http.StatusCreated, rec.Code,
		"handler response must not be aborted when audit write fails")
}
