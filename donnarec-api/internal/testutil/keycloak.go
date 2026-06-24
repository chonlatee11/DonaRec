// Package testutil provides shared test infrastructure for donnarec-api integration tests.
package testutil

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

// KeycloakTestServer holds a local httptest server that emulates Keycloak's
// OIDC discovery + JWKS endpoints. Use it to test auth middleware without
// a live Keycloak instance.
//
// The server exposes:
//   - GET /realms/donnarec/.well-known/openid-configuration  (discovery)
//   - GET /realms/donnarec/protocol/openid-connect/certs     (JWKS)
type KeycloakTestServer struct {
	Server     *httptest.Server
	PrivateKey *rsa.PrivateKey
	KeyID      string
	// IssuerURL is the full base URL of the test server + realm path,
	// used as the "iss" claim in minted tokens.
	IssuerURL string
}

// NewKeycloakTestServer creates and starts a local OIDC / JWKS server backed
// by a freshly generated RSA-2048 key pair. Call t.Cleanup(s.Server.Close) after.
func NewKeycloakTestServer(t *testing.T) *KeycloakTestServer {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "generate RSA key")

	keyID := "test-key-1"
	ts := &KeycloakTestServer{
		PrivateKey: privateKey,
		KeyID:      keyID,
	}

	mux := http.NewServeMux()

	// Discovery endpoint
	mux.HandleFunc("/realms/donnarec/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		baseURL := "http://" + r.Host
		doc := map[string]interface{}{
			"issuer":                 baseURL + "/realms/donnarec",
			"jwks_uri":               baseURL + "/realms/donnarec/protocol/openid-connect/certs",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported": []string{"code"},
			"subject_types_supported":  []string{"public"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(doc)
	})

	// JWKS endpoint
	mux.HandleFunc("/realms/donnarec/protocol/openid-connect/certs", func(w http.ResponseWriter, r *http.Request) {
		pub := &privateKey.PublicKey
		n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
		jwks := map[string]interface{}{
			"keys": []map[string]interface{}{
				{
					"kty": "RSA",
					"alg": "RS256",
					"use": "sig",
					"kid": keyID,
					"n":   n,
					"e":   e,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	})

	ts.Server = httptest.NewServer(mux)
	ts.IssuerURL = ts.Server.URL + "/realms/donnarec"

	t.Cleanup(ts.Server.Close)
	return ts
}

// MintToken creates a signed JWT with the given roles embedded under
// realm_access.roles (Keycloak convention — never top-level "roles").
// The token is signed with the test server's RSA private key and is
// valid for 5 minutes from call time.
//
// clientID is used as the "aud" claim (matches KEYCLOAK_CLIENT_ID).
//
// Usage:
//
//	srv := testutil.NewKeycloakTestServer(t)
//	token := srv.MintToken("donnarec-backend", "maker", "checker")
func (ts *KeycloakTestServer) MintToken(clientID string, roles ...string) string {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   "test-subject-uuid",
		"email": "test@example.com",
		"iss":   ts.IssuerURL,
		"aud":   []string{clientID},
		"iat":   now.Unix(),
		"exp":   now.Add(5 * time.Minute).Unix(),
		"realm_access": map[string]interface{}{
			"roles": roles,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = ts.KeyID

	signed, err := token.SignedString(ts.PrivateKey)
	if err != nil {
		panic("testutil.MintToken: failed to sign token: " + err.Error())
	}
	return signed
}

// MintTestToken is a convenience wrapper for single-server test scenarios.
// It creates a new KeycloakTestServer (and registers cleanup with t), then mints
// a token with the default client ID "donnarec-backend".
//
// Deprecated: Prefer NewKeycloakTestServer(t).MintToken(...) for tests that
// need the server URL (e.g., to configure the auth middleware).
// This helper is retained for simple unit tests that just need a signed token.
func MintTestToken(t *testing.T, roles ...string) (serverURL string, token string) {
	t.Helper()
	srv := NewKeycloakTestServer(t)
	tok := srv.MintToken("donnarec-backend", roles...)
	return srv.Server.URL, tok
}
