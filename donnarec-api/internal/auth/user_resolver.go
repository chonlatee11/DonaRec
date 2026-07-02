package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

// This middleware lives in internal/auth (not internal/donation) because it belongs to
// the auth chain — it runs alongside RequireAuth/RequireRoles and reads the same "claims"
// context value. It stays DB-agnostic via an injected UserIDResolver (returning the
// low-level pgtype.UUID, never importing the db package), so internal/auth keeps zero
// coupling to sqlc/the query layer.

// AppUserIDContextKey is the gin.Context key under which ResolveAppUser stores the
// authenticated caller's internal users.id (pgtype.UUID). Handlers read it to populate
// columns that REFERENCES users(id) — never the raw Keycloak "sub" (bug:
// created-by-fk-mismatch).
const AppUserIDContextKey = "app_user_id"

// ErrSubjectNotProvisioned is the sentinel a UserIDResolver returns when a validly
// authenticated Keycloak subject has no active row in the application's users table.
// ResolveAppUser maps it to HTTP 403 user_not_provisioned. It is defined here (not in the
// db package) so internal/auth remains DB-agnostic.
var ErrSubjectNotProvisioned = errors.New("auth: keycloak subject is not a provisioned application user")

// UserIDResolver resolves a Keycloak subject ("sub" claim) to the internal users.id.
// It is injected — rather than importing the db package — so internal/auth stays
// DB-agnostic. Implementations must return ErrSubjectNotProvisioned when no active user
// row matches the subject.
type UserIDResolver func(ctx context.Context, subject string) (pgtype.UUID, error)

// ResolveAppUser returns middleware that MUST run AFTER RequireAuth. It reads the
// KeycloakClaims placed in the context by RequireAuth, resolves claims.Subject to the
// caller's internal users.id via resolve, and stores the result under
// AppUserIDContextKey for downstream handlers.
//
// This centralises the Keycloak-sub -> users.id resolution (bug: created-by-fk-mismatch)
// so handlers/services receive an already-resolved users.id instead of every service
// method performing its own lookup.
//
// Responses:
//   - 401 no_auth_context       — claims missing (RequireAuth not run before this)
//   - 500 invalid_claims_type   — claims present but wrong type
//   - 403 user_not_provisioned  — subject resolves to no active users row
//   - 500 user_resolution_failed — unexpected resolver error
func ResolveAppUser(resolve UserIDResolver, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, exists := c.Get("claims")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no_auth_context"})
			return
		}
		kc, ok := raw.(KeycloakClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "invalid_claims_type"})
			return
		}

		appUserID, err := resolve(c.Request.Context(), kc.Subject)
		if err != nil {
			if errors.Is(err, ErrSubjectNotProvisioned) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "user_not_provisioned"})
				return
			}
			// Pattern C: never log PII — record only that resolution failed, no subject/email.
			logger.Error("app user resolution failed", zap.Error(err))
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "user_resolution_failed"})
			return
		}

		c.Set(AppUserIDContextKey, appUserID)
		c.Next()
	}
}
