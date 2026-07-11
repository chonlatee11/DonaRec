// Package donation — Task 2 test (plan 06-01): source filter on
// SearchDonations/CountDonations (FR-08, D-77).
//
// Seeds one flow_a donation (via the existing svc.Create path, which always
// produces source='flow_a' per migration 000015's DEFAULT) and one flow_b
// donation (via a raw INSERT — the public-submission service path that
// produces source='flow_b' records doesn't exist until plan 03; this test
// only proves the filter/query layer, not the future create path).
//
// Requires Docker (testcontainers). Skip with -short.
package donation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/crypto"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/testutil"
)

// TestSearchDonations_SourceFilter verifies that the source filter threaded
// through SearchDonations/CountDonations/ListFilter.Source correctly:
//   - excludes flow_a rows when source=flow_b
//   - excludes flow_b rows when source=flow_a
//   - returns both when source is unset (nil narg skips the filter, D-53)
//   - CountDonations' total always matches the filtered item count (D-R2)
func TestSearchDonations_SourceFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires Docker")
	}

	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", testKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	svc := NewDonationService(pool, queries, nil, nil, kp, zap.NewNop())

	// Test user to satisfy created_by FK for both rows.
	userRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:           "source-filter-test@example.com",
		DisplayName:     "Source Filter Test Maker",
		KeycloakSubject: "b3a1c9e2-1111-4c3d-9a2b-source0filter1",
	})
	require.NoError(t, err, "test user must be created")

	claims := auth.KeycloakClaims{
		Subject:     "b3a1c9e2-1111-4c3d-9a2b-source0filter1",
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	const flowADonorName = "Source Filter Flow A Donor"
	const flowBDonorName = "Source Filter Flow B Donor"

	// Seed flow_a row via the real service Create path — source defaults to
	// 'flow_a' from migration 000015's DEFAULT (no source param on Create).
	flowAReq := CreateDonationRequest{
		DonorName:  flowADonorName,
		DonorTaxID: "1234567890123",
		Amount:     1000.00,
		DonatedAt:  "2024-06-15",
	}
	flowAResp, err := svc.Create(ctx, flowAReq, userRow.ID, claims)
	require.NoError(t, err, "flow_a seed donation must be created")

	// Seed flow_b row via raw INSERT — the public-submission service path
	// (plan 03) does not exist yet; this proves the filter/query layer only.
	var flowBID string
	err = pool.QueryRow(ctx, `
		INSERT INTO donations (
			created_by, donor_name, donor_tax_id_enc, donor_tax_id_dek,
			amount, donated_at, source
		) VALUES ($1, $2, $3, $4, $5, $6, 'flow_b')
		RETURNING id`,
		userRow.ID, flowBDonorName, []byte{0x00}, []byte{0x00}, 2000.00, "2024-06-16",
	).Scan(&flowBID)
	require.NoError(t, err, "flow_b seed donation must be created")

	donorNames := func(items []DonationListItem) []string {
		names := make([]string, 0, len(items))
		for _, item := range items {
			names = append(names, item.DonorName)
		}
		return names
	}

	t.Run("source=flow_b excludes flow_a", func(t *testing.T) {
		source := "flow_b"
		items, total, err := svc.Search(ctx, ListFilter{Source: &source}, claims)
		require.NoError(t, err)
		names := donorNames(items)
		assert.Contains(t, names, flowBDonorName, "flow_b row must be present")
		assert.NotContains(t, names, flowADonorName, "flow_a row must be absent")
		assert.EqualValues(t, len(items), total, "CountDonations total must match filtered item count")
	})

	t.Run("source=flow_a excludes flow_b", func(t *testing.T) {
		source := "flow_a"
		items, total, err := svc.Search(ctx, ListFilter{Source: &source}, claims)
		require.NoError(t, err)
		names := donorNames(items)
		assert.Contains(t, names, flowADonorName, "flow_a row must be present")
		assert.NotContains(t, names, flowBDonorName, "flow_b row must be absent")
		assert.EqualValues(t, len(items), total, "CountDonations total must match filtered item count")
	})

	t.Run("source unset returns both", func(t *testing.T) {
		items, total, err := svc.Search(ctx, ListFilter{}, claims)
		require.NoError(t, err)
		names := donorNames(items)
		assert.Contains(t, names, flowADonorName, "flow_a row must be present when source filter is unset")
		assert.Contains(t, names, flowBDonorName, "flow_b row must be present when source filter is unset")
		assert.EqualValues(t, len(items), total, "CountDonations total must match unfiltered item count")
	})

	// Keep flowAResp/flowBID referenced (avoid unused-var lint noise; also a
	// light sanity check that both rows persisted with the ids we expect).
	require.NotEmpty(t, flowAResp.ID)
	require.NotEmpty(t, flowBID)
}
