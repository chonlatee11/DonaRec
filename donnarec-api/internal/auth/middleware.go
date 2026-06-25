package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AuthMiddleware validates Bearer JWTs issued by the configured Keycloak realm.
//
// It uses coreos/go-oidc/v3 for OIDC discovery and token verification.
// The verifier enforces:
//   - Token signature (via JWKS fetched from Keycloak's discovery endpoint)
//   - Issuer ("iss") must match expectedIssuer (configurable via OIDC_ISSUER env)
//   - Audience ("aud") must include the configured clientID (Pitfall 3)
//
// The discovery URL (keycloakBaseURL/realms/realm) may differ from expectedIssuer.
// This supports deployments where the Go API reaches Keycloak via an internal Docker
// hostname (keycloak:8080) for discovery, but tokens carry the public hostname
// (localhost:8080 or prod domain) in their "iss" claim.
//
// Roles are parsed from realm_access.roles — never from a top-level "roles" claim (Pitfall 1).
type AuthMiddleware struct {
	verifier *oidc.IDTokenVerifier
	logger   *zap.Logger
}

// NewAuthMiddleware initialises an AuthMiddleware by discovering the Keycloak realm's
// OIDC configuration and fetching its JWKS.
//
// keycloakBaseURL — base URL of the Keycloak server for OIDC discovery (internal Docker URL)
//
//	e.g. "http://keycloak:8080" (docker-compose) or "http://localhost:8080" (local dev)
//
// realm           — Keycloak realm name, e.g. "donnarec"
// clientID        — backend client ID used as the expected JWT audience (Pitfall 3)
// expectedIssuer  — expected "iss" claim in tokens (OIDC_ISSUER env); typically the
//
//	public URL seen by the browser, e.g. "http://localhost:8080/realms/donnarec".
//	When empty, falls back to <keycloakBaseURL>/realms/<realm> (original behaviour).
func NewAuthMiddleware(keycloakBaseURL, realm, clientID, expectedIssuer string, logger *zap.Logger) (*AuthMiddleware, error) {
	// providerURL follows the Keycloak realm URL format.
	// go-oidc will GET {providerURL}/.well-known/openid-configuration to discover JWKS URI.
	providerURL := fmt.Sprintf("%s/realms/%s", keycloakBaseURL, realm)

	// If no expectedIssuer supplied, fall back to the discovery URL (original behaviour).
	if expectedIssuer == "" {
		expectedIssuer = providerURL
	}

	// InsecureIssuerURLContext instructs go-oidc to accept providerURL as the discovery
	// endpoint even when its issuer claim in the discovery document differs from providerURL.
	// The context carries the override; oidc.NewProvider respects it during discovery.
	// This solves the hostname mismatch (GAP 2): discovery at internal keycloak:8080 while
	// tokens carry iss=http://localhost:8080/realms/donnarec (public hostname).
	//
	// Security note: this does NOT skip signature, audience, or expiry checks.
	// provider.Verifier enforces all of those via the standard JWKS path.
	ctx := oidc.InsecureIssuerURLContext(context.Background(), expectedIssuer)

	provider, err := oidc.NewProvider(ctx, providerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc provider init for realm %q at %s: %w", realm, keycloakBaseURL, err)
	}

	// ClientID in oidc.Config causes the verifier to enforce that the token's
	// "aud" claim includes this client ID. Without it, any token from the realm
	// would be accepted — an audience bypass (RESEARCH.md Pitfall 3).
	// Signature (JWKS), expiry, and aud checks remain in full effect.
	verifier := provider.Verifier(&oidc.Config{ClientID: clientID})

	return &AuthMiddleware{
		verifier: verifier,
		logger:   logger,
	}, nil
}

// RequireAuth returns a Gin middleware that validates the Bearer token in the
// Authorization header. On success it sets "claims" in the Gin context so that
// downstream handlers and RequireRoles can read them.
//
// Response codes:
//   - 401: missing token, invalid token, wrong issuer/audience, expired token
//   - Claims are set via c.Set("claims", KeycloakClaims{...})
func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		rawToken := extractBearerToken(c)
		if rawToken == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing_token",
			})
			return
		}

		// Verify signature + expiry + iss + aud
		idToken, err := m.verifier.Verify(c.Request.Context(), rawToken)
		if err != nil {
			m.logger.Debug("token verification failed",
				zap.String("reason", err.Error()),
				// Never log the raw token value
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid_token",
			})
			return
		}

		// Parse custom claims — roles from realm_access.roles (Pitfall 1)
		var claims KeycloakClaims
		if err := idToken.Claims(&claims); err != nil {
			m.logger.Error("failed to parse token claims",
				zap.Error(err),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "claims_parse_error",
			})
			return
		}

		// Inject claims for downstream handlers and RBAC guard
		c.Set("claims", claims)
		c.Next()
	}
}

// extractBearerToken extracts the raw JWT string from the Authorization header.
// Returns an empty string if the header is absent or not in "Bearer <token>" format.
//
// The scheme is matched case-insensitively per RFC 6750/7235 ("Bearer", "bearer",
// "BEARER" are all valid), and the remaining token is trimmed so that a header with
// only whitespace after the scheme (e.g. "Bearer    ") yields "" (treated as missing)
// rather than passing a whitespace token to Verify (WR-03).
func extractBearerToken(c *gin.Context) string {
	const prefix = "bearer "
	h := c.GetHeader("Authorization")
	if len(h) < len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}
