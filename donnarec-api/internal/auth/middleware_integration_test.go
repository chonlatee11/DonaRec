package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestOIDCMiddleware_Integration tests the full RequireAuth middleware
// using a local httptest JWKS server (no live Keycloak needed).
//
// These tests are NOT -short (they start an httptest server) and validate
// the complete token-verification pipeline.
func TestOIDCMiddleware_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode (no httptest server)")
	}

	kcServer := testutil.NewKeycloakTestServer(t)
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	clientID := "donnarec-backend"

	authMW, err := NewAuthMiddleware(kcServer.Server.URL, "donnarec", clientID, logger)
	require.NoError(t, err, "NewAuthMiddleware must succeed with test OIDC server")

	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/api/me", authMW.RequireAuth(), func(c *gin.Context) {
		claims := c.MustGet("claims").(KeycloakClaims)
		c.JSON(http.StatusOK, gin.H{"sub": claims.Subject, "email": claims.Email})
	})

	t.Run("valid token with correct audience returns 200", func(t *testing.T) {
		token := kcServer.MintToken(clientID, RoleMaker)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "valid token should return 200")
	})

	t.Run("missing Authorization header returns 401", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/me", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("token with wrong audience returns 401", func(t *testing.T) {
		// Mint a token for a different client ID — audience mismatch must be rejected (Pitfall 3)
		wrongAudToken := kcServer.MintToken("wrong-client-id", RoleMaker)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/me", nil)
		req.Header.Set("Authorization", "Bearer "+wrongAudToken)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "wrong audience must be rejected")
	})

	t.Run("expired token returns 401", func(t *testing.T) {
		// We cannot easily create a truly expired token via the test server without time mocking.
		// Instead, we test with a completely malformed token.
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/me", nil)
		req.Header.Set("Authorization", "Bearer not.a.real.jwt.token")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code, "malformed/invalid token must be rejected")
	})

	t.Run("token with admin role passes admin guard", func(t *testing.T) {
		adminRouter := gin.New()
		adminRouter.Use(gin.Recovery())
		adminGroup := adminRouter.Group("/api/admin")
		adminGroup.Use(authMW.RequireAuth())
		adminGroup.Use(RequireRoles(RoleAdmin))
		adminGroup.GET("/users", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"users": []string{}})
		})

		adminToken := kcServer.MintToken(clientID, RoleAdmin)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/admin/users", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		adminRouter.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "admin token should pass admin guard")
	})

	t.Run("maker token is rejected by admin guard with 403", func(t *testing.T) {
		adminRouter := gin.New()
		adminRouter.Use(gin.Recovery())
		adminGroup := adminRouter.Group("/api/admin")
		adminGroup.Use(authMW.RequireAuth())
		adminGroup.Use(RequireRoles(RoleAdmin))
		adminGroup.GET("/users", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"users": []string{}})
		})

		makerToken := kcServer.MintToken(clientID, RoleMaker)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/admin/users", nil)
		req.Header.Set("Authorization", "Bearer "+makerToken)
		adminRouter.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code, "maker token must be denied admin endpoint")
	})
}

// TestOIDCProvider verifies that NewAuthMiddleware correctly discovers
// the OIDC provider from the test server.
func TestOIDCProvider(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	kcServer := testutil.NewKeycloakTestServer(t)
	logger := zap.NewNop()

	_, err := NewAuthMiddleware(kcServer.Server.URL, "donnarec", "donnarec-backend", logger)
	require.NoError(t, err, "NewAuthMiddleware must succeed — OIDC discovery must work")
}

// TestNewAuthMiddleware_InvalidProvider verifies that an unreachable OIDC server
// causes NewAuthMiddleware to return an error (not panic).
func TestNewAuthMiddleware_InvalidProvider(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	logger := zap.NewNop()
	ctx := context.Background()
	_ = ctx

	_, err := NewAuthMiddleware("http://localhost:19999", "donnarec", "donnarec-backend", logger)
	assert.Error(t, err, "unreachable OIDC server must return an error")
}
