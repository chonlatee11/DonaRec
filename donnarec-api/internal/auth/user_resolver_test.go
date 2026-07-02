package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/donnarec/donnarec-api/internal/auth"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestResolveAppUser is the middleware half of the created-by-fk-mismatch regression
// guard: it proves auth.ResolveAppUser resolves a provisioned Keycloak subject to the
// correct users.id (stored under AppUserIDContextKey) and 403s an unprovisioned subject
// with user_not_provisioned — before any handler that would write a REFERENCES users(id)
// column runs. No DB/Docker: the resolver is stubbed, so this runs in -short too.
func TestResolveAppUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var wantID pgtype.UUID
	require.NoError(t, wantID.Scan("11111111-1111-1111-1111-111111111111"))

	const provisionedSub = "kc-sub-provisioned"

	// Stub resolver: known subject -> users.id; anything else -> ErrSubjectNotProvisioned
	// (exactly what the real closure over GetUserByKeycloakSubject returns on pgx.ErrNoRows).
	resolver := func(_ context.Context, subject string) (pgtype.UUID, error) {
		if subject == provisionedSub {
			return wantID, nil
		}
		return pgtype.UUID{}, ErrSubjectNotProvisioned
	}

	// newRouter builds a router that injects claims (as RequireAuth would) then runs
	// ResolveAppUser; the terminal handler echoes the resolved app_user_id.
	newRouter := func(claims KeycloakClaims) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("claims", claims); c.Next() })
		r.Use(ResolveAppUser(resolver, zap.NewNop()))
		r.GET("/x", func(c *gin.Context) {
			got, ok := c.Get(AppUserIDContextKey)
			require.True(t, ok, "app_user_id must be set for a provisioned subject")
			id, ok := got.(pgtype.UUID)
			require.True(t, ok, "app_user_id must be a pgtype.UUID")
			c.JSON(http.StatusOK, gin.H{"id": id.String()})
		})
		return r
	}

	t.Run("provisioned subject passes and sets correct app_user_id", func(t *testing.T) {
		r := newRouter(KeycloakClaims{Subject: provisionedSub})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/x", nil)
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "provisioned subject must pass")
		assert.Contains(t, w.Body.String(), wantID.String(),
			"resolved app_user_id must equal the provisioned users.id")
	})

	t.Run("unprovisioned subject aborts 403 user_not_provisioned", func(t *testing.T) {
		r := newRouter(KeycloakClaims{Subject: "kc-sub-unknown"})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/x", nil)
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusForbidden, w.Code, "unprovisioned subject must be 403")
		assert.Contains(t, w.Body.String(), "user_not_provisioned")
	})

	t.Run("missing claims aborts 401 no_auth_context", func(t *testing.T) {
		// ResolveAppUser without a preceding RequireAuth (no claims in context).
		r := gin.New()
		r.Use(ResolveAppUser(resolver, zap.NewNop()))
		r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/x", nil)
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "no_auth_context")
	})
}
