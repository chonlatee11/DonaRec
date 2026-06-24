package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Role constants for donnarec-api.
// Use these constants everywhere — never bare string literals.
const (
	RoleMaker   = "maker"
	RoleChecker = "checker"
	RoleAdmin   = "admin"
)

// RequireRoles returns a Gin middleware factory that enforces RBAC.
// A request passes if and only if the authenticated user's claims contain
// ALL of the specified required roles (logical AND).
//
// For multi-role users (D-02): a user holding [maker, checker] can pass
// RequireRoles(RoleMaker) AND RequireRoles(RoleChecker) independently.
//
// Order in router: must be placed AFTER RequireAuth() so that claims are
// already populated in the Gin context.
//
//   adminGroup.Use(authMiddleware.RequireAuth())
//   adminGroup.Use(RequireRoles(RoleAdmin))
//
// SoD note: RequireNotCreator — implemented Phase 3 (D-04).
// This guard enforces role-level access; per-record creator exclusion is a
// separate, attribute-based check that belongs to the approval transaction.
func RequireRoles(requiredRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, exists := c.Get("claims")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "no_auth_context",
			})
			return
		}

		kc, ok := raw.(KeycloakClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid_claims_type",
			})
			return
		}

		for _, r := range requiredRoles {
			if !kc.HasRole(r) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "insufficient_role",
				})
				return
			}
		}

		c.Next()
	}
}
