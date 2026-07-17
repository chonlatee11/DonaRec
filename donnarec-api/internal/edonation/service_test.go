// internal/edonation/service_test.go — TDD RED→GREEN tests for edonation.Service.Export
// (Task 1, plan 05-02).
//
// Fixtures use the REAL donation lifecycle (Create→Submit→Approve[→Cancel]) via
// internal/donation.DonationService — the only way to produce a genuinely 'issued'
// (or cancelled) donation with real encrypted PII, mirroring
// internal/donation/service_integration_test.go's createAndSubmit helper.
package edonation_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/donnarec/donnarec-api/internal/audit"
	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/crypto"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/donation"
	"github.com/donnarec/donnarec-api/internal/edonation"
	"github.com/donnarec/donnarec-api/internal/receiptno"
	"github.com/donnarec/donnarec-api/internal/testutil"
)

// exportTestKEK is a 32-byte hex key for this file's test use only (same value as
// the donation package's integration tests — test-only, never a real secret).
const exportTestKEK = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

// exportFixture bundles the services + provisioned users needed to seed donations
// at any lifecycle stage for the export tests.
type exportFixture struct {
	pool          *pgxpool.Pool
	queries       *db.Queries
	donationSvc   *donation.DonationService
	edonSvc       *edonation.Service
	makerID       pgtype.UUID
	checkerID     pgtype.UUID
	makerClaims   auth.KeycloakClaims
	checkerClaims auth.KeycloakClaims
}

// setupExportFixture spins a fresh Postgres testcontainer + wires the real
// donation.DonationService and edonation.Service against it, provisioning one
// maker and one checker user (mirrors service_integration_test.go's pattern).
func setupExportFixture(t *testing.T) *exportFixture {
	t.Helper()
	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", exportTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	auditSvc := audit.NewAuditService(pool, queries, zap.NewNop())
	alloc := receiptno.NewAllocator(queries)
	donationSvc := donation.NewDonationService(pool, queries, alloc, auditSvc, kp, zap.NewNop())
	edonSvc := edonation.NewService(pool, queries, auditSvc, kp, zap.NewNop())

	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "maker-export-test@example.com", DisplayName: "Maker Export Test",
		KeycloakSubject: "11111111-1111-1111-1111-111111111111",
	})
	require.NoError(t, err)
	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-export-test@example.com", DisplayName: "Checker Export Test",
		KeycloakSubject: "22222222-2222-2222-2222-222222222222",
	})
	require.NoError(t, err)

	return &exportFixture{
		pool:        pool,
		queries:     queries,
		donationSvc: donationSvc,
		edonSvc:     edonSvc,
		makerID:     makerRow.ID,
		checkerID:   checkerRow.ID,
		makerClaims: auth.KeycloakClaims{
			Subject:     "11111111-1111-1111-1111-111111111111",
			RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
		},
		checkerClaims: auth.KeycloakClaims{
			Subject:     "22222222-2222-2222-2222-222222222222",
			RealmAccess: auth.RealmRoles{Roles: []string{"checker"}},
		},
	}
}

// seedIssued creates a draft donation, submits it, and approves it — returning
// the issued DonationDetailResponse plus its plaintext tax ID (for decrypt-
// correctness assertions).
func (f *exportFixture) seedIssued(t *testing.T, ctx context.Context, name, taxID, date string, amount float64) (*donation.DonationDetailResponse, string) {
	t.Helper()
	d, err := f.donationSvc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  name,
		DonorTaxID: taxID,
		Amount:     amount,
		DonatedAt:  date,
	}, f.makerID, f.makerClaims)
	require.NoError(t, err)

	_, err = f.donationSvc.Submit(ctx, d.ID, f.makerClaims)
	require.NoError(t, err)

	issued, err := f.donationSvc.Approve(ctx, d.ID, f.checkerID, f.checkerClaims)
	require.NoError(t, err)
	require.Equal(t, "issued", issued.Status)
	return issued, taxID
}

// seedDraft creates a donation and leaves it in 'draft' status (never submitted).
func (f *exportFixture) seedDraft(t *testing.T, ctx context.Context, name, taxID, date string, amount float64) *donation.DonationDetailResponse {
	t.Helper()
	d, err := f.donationSvc.Create(ctx, donation.CreateDonationRequest{
		DonorName:  name,
		DonorTaxID: taxID,
		Amount:     amount,
		DonatedAt:  date,
	}, f.makerID, f.makerClaims)
	require.NoError(t, err)
	return d
}

// seedCancelled creates, submits, approves, then cancels a donation (D-47:
// Checker/Admin only) — the receipt number is retained on the cancelled record.
func (f *exportFixture) seedCancelled(t *testing.T, ctx context.Context, name, taxID, date string, amount float64) *donation.DonationDetailResponse {
	t.Helper()
	issued, _ := f.seedIssued(t, ctx, name, taxID, date, amount)
	cancelled, err := f.donationSvc.Cancel(ctx, issued.ID, donation.CancelDonationRequest{
		Reason: "test cancellation for export fixture",
	}, f.checkerID, f.checkerClaims)
	require.NoError(t, err)
	require.Equal(t, "cancelled", cancelled.Status)
	return cancelled
}

// countAuditRows returns the number of audit_log rows with the given action —
// a plain raw-SQL COUNT (mirrors service_integration_test.go's direct pool.QueryRow
// assertions against receipt_numbers/outbox_jobs).
func countAuditRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, action string) int {
	t.Helper()
	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE action = $1`, action).Scan(&count))
	return count
}

// --- Task 1 tests -----------------------------------------------------------

// TestExport_Forbidden_MakerRole proves the service-layer role gate rejects a
// Maker-only caller BEFORE any DB call (T-05-02-RBAC defense-in-depth). Unit-level:
// no Postgres connection needed since the check runs before any DB access.
func TestExport_Forbidden_MakerRole(t *testing.T) {
	svc := edonation.NewService(nil, nil, nil, nil, zap.NewNop())

	makerClaims := auth.KeycloakClaims{
		Subject:     "00000000-0000-0000-0000-000000000099",
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	_, err := svc.Export(context.Background(), edonation.ExportFilter{}, makerClaims)
	require.Error(t, err)
	assert.ErrorIs(t, err, edonation.ErrForbidden)
}

// TestExport_IssuedOnly proves Export returns ONLY issued donations — cancelled
// and draft records are excluded (D-66) — with the donor tax ID correctly
// decrypted to its original 13-digit plaintext (D-64), and exactly ONE summary
// audit row is written with count matching the returned row set (T-05-02-UNAUDITED).
func TestExport_IssuedOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}
	f := setupExportFixture(t)
	ctx := context.Background()

	issued1, taxID1 := f.seedIssued(t, ctx, "นาย หนึ่ง ทดสอบ", "1111111111111", "2026-03-01", 1000.00)
	issued2, taxID2 := f.seedIssued(t, ctx, "นาง สอง ทดสอบ", "2222222222222", "2026-03-02", 2000.00)
	issued3, taxID3 := f.seedIssued(t, ctx, "นาย สาม ทดสอบ", "3333333333333", "2026-03-03", 3000.00)
	f.seedCancelled(t, ctx, "นาง สี่ ยกเลิก", "4444444444444", "2026-03-04", 4000.00)
	f.seedDraft(t, ctx, "นาย ห้า ร่าง", "5555555555555", "2026-03-05", 5000.00)

	rows, err := f.edonSvc.Export(ctx, edonation.ExportFilter{}, f.checkerClaims)
	require.NoError(t, err)
	require.Len(t, rows, 3, "only the 3 issued donations must be returned — cancelled and draft excluded (D-66)")

	byNationalID := map[string]edonation.ExportRow{}
	for _, r := range rows {
		byNationalID[r.NationalID] = r
	}

	require.Contains(t, byNationalID, taxID1)
	require.Contains(t, byNationalID, taxID2)
	require.Contains(t, byNationalID, taxID3)

	assert.Equal(t, "นาย หนึ่ง ทดสอบ", byNationalID[taxID1].DonorName)
	assert.Equal(t, "2026-03-01", byNationalID[taxID1].DonatedAt)
	assert.NotEmpty(t, byNationalID[taxID1].ReceiptFormatted, "issued donation must have a receipt_formatted string")
	assert.Equal(t, *issued1.ReceiptFormatted, byNationalID[taxID1].ReceiptFormatted)
	assert.Equal(t, *issued2.ReceiptFormatted, byNationalID[taxID2].ReceiptFormatted)
	assert.Equal(t, *issued3.ReceiptFormatted, byNationalID[taxID3].ReceiptFormatted)

	// T-05-02-UNAUDITED: exactly ONE summary audit row for this export event, not
	// one row per donation.
	assert.Equal(t, 1, countAuditRows(t, ctx, f.pool, "edonation.export"),
		"exactly one audit_log row must be written per export event")

	var afterJSON []byte
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT after_json FROM audit_log WHERE action = 'edonation.export' LIMIT 1`).Scan(&afterJSON))
	// Postgres canonicalizes jsonb text (key order by length, space after ':') on
	// storage/read-back — assert on the semantic value via a JSON round-trip rather
	// than a literal compact-JSON substring match.
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(afterJSON, &decoded))
	assert.Equal(t, float64(3), decoded["count"], "audit after_json count must equal the exported row count")
}

// TestExport_KeyedStatusFilter proves the keyed_status filter returns only
// unkeyed issued rows when set to false (D-66).
func TestExport_KeyedStatusFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}
	f := setupExportFixture(t)
	ctx := context.Background()

	unkeyed1, _ := f.seedIssued(t, ctx, "นาย ยังไม่คีย์ หนึ่ง", "6666666666661", "2026-04-01", 100.00)
	unkeyed2, _ := f.seedIssued(t, ctx, "นาย ยังไม่คีย์ สอง", "6666666666662", "2026-04-02", 200.00)
	keyed, _ := f.seedIssued(t, ctx, "นาย คีย์แล้ว", "6666666666663", "2026-04-03", 300.00)

	var keyedID pgtype.UUID
	require.NoError(t, keyedID.Scan(keyed.ID))
	require.NoError(t, f.queries.SetKeyedBulk(ctx, db.SetKeyedBulkParams{
		Keyed:       true,
		KeyedAt:     pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		KeyedBy:     f.checkerID,
		DonationIds: []pgtype.UUID{keyedID},
	}))

	falseVal := false
	rows, err := f.edonSvc.Export(ctx, edonation.ExportFilter{KeyedStatus: &falseVal}, f.checkerClaims)
	require.NoError(t, err)
	require.Len(t, rows, 2, "keyed_status=false must return only the 2 unkeyed issued donations")

	var formattedSet []string
	for _, r := range rows {
		formattedSet = append(formattedSet, r.ReceiptFormatted)
	}
	assert.Contains(t, formattedSet, *unkeyed1.ReceiptFormatted)
	assert.Contains(t, formattedSet, *unkeyed2.ReceiptFormatted)
	assert.NotContains(t, formattedSet, *keyed.ReceiptFormatted)
}

// TestExport_DateRangeFilter proves the from/to date-range filter bounds by
// donated_at, inclusive on both ends (D-66).
func TestExport_DateRangeFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}
	f := setupExportFixture(t)
	ctx := context.Background()

	f.seedIssued(t, ctx, "นาย นอกช่วง ก่อน", "7777777777771", "2026-05-01", 100.00)
	mid, _ := f.seedIssued(t, ctx, "นาย ในช่วง", "7777777777772", "2026-05-15", 200.00)
	f.seedIssued(t, ctx, "นาย นอกช่วง หลัง", "7777777777773", "2026-05-31", 300.00)

	from, err := time.ParseInLocation("2006-01-02", "2026-05-10", time.UTC)
	require.NoError(t, err)
	to, err := time.ParseInLocation("2006-01-02", "2026-05-20", time.UTC)
	require.NoError(t, err)

	rows, err := f.edonSvc.Export(ctx, edonation.ExportFilter{From: &from, To: &to}, f.checkerClaims)
	require.NoError(t, err)
	require.Len(t, rows, 1, "only the donation dated within [2026-05-10, 2026-05-20] must be returned")
	assert.Equal(t, *mid.ReceiptFormatted, rows[0].ReceiptFormatted)
}

// TestExport_EmptyResult proves Export returns an empty (not nil-panicking) slice
// with a committed audit row (count=0) when the filter matches nothing — the
// handler (Task 2) is responsible for mapping this to a 4xx before streaming.
func TestExport_EmptyResult(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}
	f := setupExportFixture(t)
	ctx := context.Background()

	rows, err := f.edonSvc.Export(ctx, edonation.ExportFilter{}, f.checkerClaims)
	require.NoError(t, err)
	assert.Empty(t, rows)
	assert.Equal(t, 1, countAuditRows(t, ctx, f.pool, "edonation.export"))
}
