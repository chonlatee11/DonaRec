// Package auth provides JWT/OIDC authentication and RBAC authorization
// for donnarec-api. It validates tokens issued by the self-hosted Keycloak realm.
package auth

// KeycloakClaims maps the JWT claim structure emitted by Keycloak.
//
// IMPORTANT: Roles are nested under realm_access.roles — never a top-level "roles"
// field. This is the Keycloak convention (RESEARCH.md Pitfall 1). Any code that
// reads roles from a different path is incorrect and must be fixed.
type KeycloakClaims struct {
	// Subject is the Keycloak user ID ("sub" claim) — stable across logins.
	Subject string `json:"sub"`

	// Email is the user's email address from Keycloak.
	Email string `json:"email"`

	// RealmAccess holds the realm-level roles assigned to this user in Keycloak.
	// Access via claims.RealmAccess.Roles — never from a top-level "roles" field.
	RealmAccess RealmRoles `json:"realm_access"`
}

// RealmRoles holds the role list nested under "realm_access" in a Keycloak JWT.
type RealmRoles struct {
	// Roles is the list of realm roles assigned to the user.
	// Maps to JSON: { "realm_access": { "roles": ["maker", "admin"] } }
	Roles []string `json:"roles"`
}

// HasRole returns true if this user holds the named role.
// Role lookup is O(n) over the realm roles list; this is acceptable for the
// small role sets used in this system (maker/checker/admin — at most 3 roles).
func (kc KeycloakClaims) HasRole(role string) bool {
	for _, r := range kc.RealmAccess.Roles {
		if r == role {
			return true
		}
	}
	return false
}
