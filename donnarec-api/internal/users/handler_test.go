package users_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/donnarec/donnarec-api/internal/auth"
	. "github.com/donnarec/donnarec-api/internal/users"
	"github.com/donnarec/donnarec-api/internal/testutil"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// TestCreateUserRBAC verifies that the /api/admin/users endpoint correctly
// enforces Admin role (RBAC) for user creation (D-01).
func TestCreateUserRBAC(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test: requires Docker for testcontainers")
	}

	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)
	logger := zap.NewNop()
	svc := NewUserService(pool, queries, logger)
	handler := NewUserHandler(svc, logger)

	// Helper: build router with injected claims
	buildRouter := func(claims auth.KeycloakClaims) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("claims", claims)
			c.Next()
		})
		adminGroup := r.Group("/api/admin")
		adminGroup.Use(auth.RequireRoles(auth.RoleAdmin))
		adminGroup.POST("/users", handler.CreateUser)
		return r
	}

	validBody := map[string]interface{}{
		"email":            "newuser@hospital.th",
		"display_name":     "New User",
		"keycloak_subject": "kc-sub-handler-test-001",
		"roles":            []string{"maker"},
	}

	t.Run("admin token creates user (201)", func(t *testing.T) {
		router := buildRouter(auth.KeycloakClaims{
			Subject: "admin-subject",
			Email:   "admin@hospital.th",
			RealmAccess: auth.RealmRoles{
				Roles: []string{auth.RoleAdmin},
			},
		})

		body, _ := json.Marshal(validBody)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/admin/users", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("maker token is rejected with 403", func(t *testing.T) {
		router := buildRouter(auth.KeycloakClaims{
			Subject: "maker-subject",
			Email:   "maker@hospital.th",
			RealmAccess: auth.RealmRoles{
				Roles: []string{auth.RoleMaker},
			},
		})

		body, _ := json.Marshal(validBody)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/admin/users", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("checker token is rejected with 403", func(t *testing.T) {
		router := buildRouter(auth.KeycloakClaims{
			Subject: "checker-subject",
			Email:   "checker@hospital.th",
			RealmAccess: auth.RealmRoles{
				Roles: []string{auth.RoleChecker},
			},
		})

		body, _ := json.Marshal(validBody)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/admin/users", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("invalid request body returns 400", func(t *testing.T) {
		router := buildRouter(auth.KeycloakClaims{
			Subject: "admin-subject",
			Email:   "admin@hospital.th",
			RealmAccess: auth.RealmRoles{
				Roles: []string{auth.RoleAdmin},
			},
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/admin/users", bytes.NewReader([]byte("not json")))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing required fields returns 422", func(t *testing.T) {
		router := buildRouter(auth.KeycloakClaims{
			Subject: "admin-subject-2",
			Email:   "admin2@hospital.th",
			RealmAccess: auth.RealmRoles{
				Roles: []string{auth.RoleAdmin},
			},
		})

		// Missing email field
		invalidBody := map[string]interface{}{
			"display_name":     "Missing Email User",
			"keycloak_subject": "kc-sub-missing-email",
			"roles":            []string{"maker"},
		}
		body, _ := json.Marshal(invalidBody)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/admin/users", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("GET /api/me returns subject and email from claims", func(t *testing.T) {
		claims := auth.KeycloakClaims{
			Subject: "test-sub-me-001",
			Email:   "me@hospital.th",
			RealmAccess: auth.RealmRoles{
				Roles: []string{auth.RoleMaker},
			},
		}

		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("claims", claims)
			c.Next()
		})
		r.GET("/api/me", handler.Me)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/me", nil)
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "test-sub-me-001", resp["sub"])
		assert.Equal(t, "me@hospital.th", resp["email"])
	})
}
