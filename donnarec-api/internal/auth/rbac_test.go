package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/donnarec/donnarec-api/internal/auth"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// TestRequireRoles_Unit verifies RBAC guard behavior without a live Keycloak server.
// These tests run under -short as they use in-memory claim injection.
func TestRequireRoles_Unit(t *testing.T) {
	t.Run("allows request when claims have required role (maker)", func(t *testing.T) {
		router := gin.New()
		router.Use(injectClaims(KeycloakClaims{
			Subject: "user-1",
			Email:   "maker@hospital.th",
			RealmAccess: RealmRoles{
				Roles: []string{RoleMaker},
			},
		}))
		router.GET("/protected", RequireRoles(RoleMaker), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("denies request when claims lack required role (admin)", func(t *testing.T) {
		router := gin.New()
		router.Use(injectClaims(KeycloakClaims{
			Subject: "user-1",
			Email:   "maker@hospital.th",
			RealmAccess: RealmRoles{
				Roles: []string{RoleMaker},
			},
		}))
		router.GET("/admin", RequireRoles(RoleAdmin), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/admin", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("returns 401 when no claims in context", func(t *testing.T) {
		router := gin.New()
		// No claims injected
		router.GET("/protected", RequireRoles(RoleMaker), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("multi-role: claims [maker,checker] passes both maker and checker guards (D-02)", func(t *testing.T) {
		// A user holding both RoleMaker and RoleChecker must be able to pass
		// a RequireRoles(RoleMaker) guard AND a RequireRoles(RoleChecker) guard independently.
		// This validates Decision D-02: multi-role users are allowed.
		claims := KeycloakClaims{
			Subject: "dual-role-user",
			Email:   "dual@hospital.th",
			RealmAccess: RealmRoles{
				// roles stored under realm_access.roles — never top-level (Pitfall 1)
				Roles: []string{RoleMaker, RoleChecker},
			},
		}

		testCases := []struct {
			name     string
			required string
		}{
			{"passes maker guard", RoleMaker},
			{"passes checker guard", RoleChecker},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				router := gin.New()
				router.Use(injectClaims(claims))
				router.GET("/guarded", RequireRoles(tc.required), func(c *gin.Context) {
					c.Status(http.StatusOK)
				})

				w := httptest.NewRecorder()
				req, _ := http.NewRequest(http.MethodGet, "/guarded", nil)
				router.ServeHTTP(w, req)

				assert.Equal(t, http.StatusOK, w.Code, "expected 200 for required role %q", tc.required)
			})
		}
	})

	t.Run("multi-role: user without admin role is denied admin guard", func(t *testing.T) {
		router := gin.New()
		router.Use(injectClaims(KeycloakClaims{
			Subject: "maker-checker-user",
			Email:   "mc@hospital.th",
			RealmAccess: RealmRoles{
				// realm_access.roles — never top-level "roles" claim (Pitfall 1)
				Roles: []string{RoleMaker, RoleChecker},
			},
		}))
		router.GET("/admin", RequireRoles(RoleAdmin), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/admin", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("HasRole correctly reads realm_access.roles (not top-level roles)", func(t *testing.T) {
		// This test explicitly validates Pitfall 1: roles MUST come from realm_access.roles
		claims := KeycloakClaims{
			RealmAccess: RealmRoles{
				Roles: []string{RoleMaker, RoleChecker}, // realm_access.roles
			},
		}
		assert.True(t, claims.HasRole(RoleMaker), "should find maker in realm_access.roles")
		assert.True(t, claims.HasRole(RoleChecker), "should find checker in realm_access.roles")
		assert.False(t, claims.HasRole(RoleAdmin), "should NOT find admin")
	})
}

// injectClaims is a test helper that injects pre-built KeycloakClaims
// into the Gin context, simulating what RequireAuth() does after token validation.
func injectClaims(claims KeycloakClaims) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("claims", claims)
		c.Next()
	}
}
