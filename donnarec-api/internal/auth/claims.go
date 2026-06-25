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
	//
	// NOTE (CR-03): for a bearerOnly backend receiving *access* tokens, the
	// "email" claim is frequently absent unless an explicit Keycloak protocol
	// mapper adds it to the access token. Do not rely on Email being present;
	// use ActorIdentity() which falls back to PreferredUsername.
	Email string `json:"email"`

	// PreferredUsername is the "preferred_username" claim. Unlike "email", this
	// claim is present in Keycloak access tokens by default, so it is a reliable
	// fallback identity for the audit trail (FR-13) when Email is empty (CR-03).
	PreferredUsername string `json:"preferred_username"`

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

// ActorIdentity returns a non-empty identity string for the audit trail (FR-13).
// It prefers Email but falls back to PreferredUsername, which Keycloak access
// tokens carry by default even when "email" is not mapped (CR-03). This prevents
// silently empty actor_email values on legally-significant audit rows.
func (kc KeycloakClaims) ActorIdentity() string {
	if kc.Email != "" {
		return kc.Email
	}
	return kc.PreferredUsername
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
