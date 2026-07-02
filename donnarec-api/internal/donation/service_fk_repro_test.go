// Package donation_test — regression guard for bug "created-by-fk-mismatch".
//
// Debug session: .planning/debug/created-by-fk-mismatch.md
//
// Root cause: donations.created_by REFERENCES users(id), but users.id is an
// independently generated gen_random_uuid() that is NEVER equal to the Keycloak
// "sub" (claims.Subject). Writing claims.Subject into created_by FK-violates for
// any real login.
//
// FIX (refactor): the Keycloak-sub -> users.id resolution now happens ONCE in the
// auth.ResolveAppUser middleware, which passes the resolved users.id down to the
// service as the explicit actingUserID parameter. The service writes actingUserID
// (NEVER claims.Subject) into created_by.
//
// This test is the SERVICE-LEVEL half of the regression guard: it proves that
// Create persists the resolved users.id passed as actingUserID — not the raw
// Keycloak sub carried in claims.Subject. The MIDDLEWARE half (unprovisioned sub
// -> 403; provisioned sub -> correct app_user_id in context) lives in
// internal/auth/user_resolver_test.go.
//
// Requires Docker testcontainers. Skip with -short.
package donation_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/crypto"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/donation"
	"github.com/donnarec/donnarec-api/internal/testutil"
)

// TestCreate_ActingUserIDWritesCorrectCreatedBy guards created-by-fk-mismatch at the
// service layer (now GREEN — the fix is applied).
//
// Setup mirrors real provisioning:
//   - users.id         = DB-generated gen_random_uuid() (via CreateUser)
//   - keycloak_subject = a distinct, realistic Keycloak "sub" UUID
//   - claims.Subject   = the SAME keycloak_subject (as a real validated OIDC token carries)
//   - actingUserID     = the resolved users.id (what auth.ResolveAppUser would inject)
//
// EXPECTED (fixed behavior): Create persists created_by = actingUserID (users.id),
// NOT claims.Subject.
//
// PRE-FIX (bug, historical): Create wrote created_by = claims.Subject (the KC sub),
// which did not exist in users.id -> Postgres 23503 foreign_key_violation on
// donations_created_by_fkey.
func TestCreate_ActingUserIDWritesCorrectCreatedBy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", integTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	svc := donation.NewDonationService(pool, queries, nil, nil, kp, zap.NewNop())

	// Realistic Keycloak sub — a UUID string, but DISTINCT from users.id.
	keycloakSub := uuid.NewString()

	userRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:           "real-kc-subject@example.com",
		DisplayName:     "Real KC Subject Maker",
		KeycloakSubject: keycloakSub,
	})
	require.NoError(t, err, "provisioning the user must succeed")

	// Sanity: confirm the defect's precondition — users.id != keycloak_subject.
	require.NotEqual(t, userRow.ID.String(), keycloakSub,
		"precondition: users.id must be independently generated, distinct from keycloak_subject")

	// claims.Subject is what a real OIDC-validated request carries: the raw KC sub.
	// It must NOT leak into created_by — only actingUserID (users.id) may.
	realMakerClaims := auth.KeycloakClaims{
		Subject:     keycloakSub,
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	// actingUserID = the resolved users.id, as auth.ResolveAppUser middleware would inject.
	resp, createErr := svc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  "นาย ทดสอบ FK Mismatch",
		DonorTaxID: "1234567890123",
		Amount:     1000.00,
		DonatedAt:  "2026-07-01",
	}, userRow.ID, realMakerClaims)

	// --- Fixed expectation: Create succeeds and writes the resolved users.id ---
	require.NoError(t, createErr,
		"Create with actingUserID = resolved users.id must succeed (fix for created-by-fk-mismatch)")
	require.NotNil(t, resp)

	// The persisted created_by must be actingUserID (users.id), never the raw KC sub.
	require.Equal(t, userRow.ID.String(), resp.CreatedBy,
		"created_by must equal the actingUserID (users.id), not the raw Keycloak sub")
	require.NotEqual(t, keycloakSub, resp.CreatedBy,
		"created_by must NOT be the raw Keycloak sub (that was the bug)")

	// Cross-check directly against the DB row (not just the service response mapping).
	var pgDonationID pgtype.UUID
	require.NoError(t, pgDonationID.Scan(resp.ID))
	persisted, getErr := queries.GetDonationByID(ctx, pgDonationID)
	require.NoError(t, getErr)
	require.Equal(t, userRow.ID, persisted.CreatedBy,
		"donations.created_by column must equal the resolved users.id")
}
