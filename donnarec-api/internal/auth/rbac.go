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

// RequireAnyRole returns a Gin middleware factory that passes if the authenticated
// user holds AT LEAST ONE of the specified roles (logical OR).
//
// Use this for routes open to several distinct roles — e.g. any staff member
// (maker OR checker OR admin) may read the donation list, and either a checker OR
// an admin may review. This is the correct guard for "any of" access; RequireRoles
// (logical AND) is for the rare "must hold ALL of" case and for single-role guards.
//
// Must be placed AFTER RequireAuth() so claims are already populated.
//
//	donationGroup.Use(RequireAnyRole(RoleMaker, RoleChecker, RoleAdmin))
func RequireAnyRole(allowedRoles ...string) gin.HandlerFunc {
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

		for _, r := range allowedRoles {
			if kc.HasRole(r) {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": "insufficient_role",
		})
	}
}
