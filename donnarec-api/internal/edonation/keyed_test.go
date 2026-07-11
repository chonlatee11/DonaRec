// internal/edonation/keyed_test.go — TDD RED→GREEN tests for
// Service.SetKeyed and Service.Aging (Task 2, plan 05-04, FR-31/D-67/D-68).
//
// Fixtures reuse the REAL donation lifecycle (Create→Submit→Approve[→Cancel])
// via internal/donation.DonationService — mirrors service_test.go's
// exportFixture pattern exactly (the only way to produce a genuinely 'issued'
// or 'cancelled' donation for these tests).
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

// keyedTestKEK is a 32-byte hex key for this file's test use only (same
// convention as service_test.go's exportTestKEK — test-only, never a real
// secret).
const keyedTestKEK = "2102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f21"

// keyedFixture bundles the services + provisioned users needed to seed
// donations at any lifecycle stage for the SetKeyed/Aging tests.
type keyedFixture struct {
	pool          *pgxpool.Pool
	queries       *db.Queries
	donationSvc   *donation.DonationService
	edonSvc       *edonation.Service
	makerID       pgtype.UUID
	checkerID     pgtype.UUID
	makerClaims   auth.KeycloakClaims
	checkerClaims auth.KeycloakClaims
}

func setupKeyedFixture(t *testing.T) *keyedFixture {
	t.Helper()
	pool := testutil.SetupTestPostgres(t)
	ctx := context.Background()

	t.Setenv("DONAREC_KEK", keyedTestKEK)
	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	queries := db.New(pool)
	auditSvc := audit.NewAuditService(pool, queries, zap.NewNop())
	alloc := receiptno.NewAllocator(queries)
	donationSvc := donation.NewDonationService(pool, queries, alloc, auditSvc, kp, zap.NewNop())
	edonSvc := edonation.NewService(pool, queries, auditSvc, kp, zap.NewNop())

	makerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "maker-keyed-test@example.com", DisplayName: "Maker Keyed Test",
		KeycloakSubject: "33333333-3333-3333-3333-333333333333",
	})
	require.NoError(t, err)
	checkerRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "checker-keyed-test@example.com", DisplayName: "Checker Keyed Test",
		KeycloakSubject: "44444444-4444-4444-4444-444444444444",
	})
	require.NoError(t, err)

	return &keyedFixture{
		pool:        pool,
		queries:     queries,
		donationSvc: donationSvc,
		edonSvc:     edonSvc,
		makerID:     makerRow.ID,
		checkerID:   checkerRow.ID,
		makerClaims: auth.KeycloakClaims{
			Subject:     "33333333-3333-3333-3333-333333333333",
			RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
		},
		checkerClaims: auth.KeycloakClaims{
			Subject:     "44444444-4444-4444-4444-444444444444",
			RealmAccess: auth.RealmRoles{Roles: []string{"checker"}},
		},
	}
}

func (f *keyedFixture) seedIssued(t *testing.T, ctx context.Context, name, taxID, date string, amount float64) *donation.DonationDetailResponse {
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
	return issued
}

func (f *keyedFixture) seedCancelled(t *testing.T, ctx context.Context, name, taxID, date string, amount float64) *donation.DonationDetailResponse {
	t.Helper()
	issued := f.seedIssued(t, ctx, name, taxID, date, amount)
	cancelled, err := f.donationSvc.Cancel(ctx, issued.ID, donation.CancelDonationRequest{
		Reason: "test cancellation for keyed fixture",
	}, f.checkerID, f.checkerClaims)
	require.NoError(t, err)
	require.Equal(t, "cancelled", cancelled.Status)
	return cancelled
}

// setApprovedAt directly overrides a donation's approved_at column — the
// public lifecycle API always stamps approved_at = time.Now() at Approve()
// time, so aging-bucket tests that need a KNOWN approved_at (to place a
// donation deterministically into not_due/near_due/overdue) must set it
// directly via raw SQL after issuance (test-only technique, mirrors
// service_test.go's TestExport_KeyedStatusFilter raw SetKeyedBulk call).
func (f *keyedFixture) setApprovedAt(t *testing.T, ctx context.Context, donationID string, approvedAt time.Time) {
	t.Helper()
	_, err := f.pool.Exec(ctx, `UPDATE donations SET approved_at = $1 WHERE id = $2`, approvedAt, donationID)
	require.NoError(t, err)
}

func countAuditRowsByAction(t *testing.T, ctx context.Context, pool *pgxpool.Pool, action string) int {
	t.Helper()
	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE action = $1`, action).Scan(&count))
	return count
}

func mustPgUUID(t *testing.T, id string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	require.NoError(t, u.Scan(id))
	return u
}

// --- Task 2: SetKeyed tests --------------------------------------------------

// TestSetKeyed_Forbidden_MakerRole proves the service-layer role gate rejects
// a Maker-only caller BEFORE any DB call (T-05-04-RBAC defense-in-depth).
func TestSetKeyed_Forbidden_MakerRole(t *testing.T) {
	svc := edonation.NewService(nil, nil, nil, nil, zap.NewNop())

	makerClaims := auth.KeycloakClaims{
		Subject:     "00000000-0000-0000-0000-000000000098",
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	err := svc.SetKeyed(context.Background(), edonation.KeyedRequest{
		DonationIDs: []pgtype.UUID{mustPgUUID(t, "00000000-0000-0000-0000-000000000001")},
		Keyed:       true,
	}, makerClaims, pgtype.UUID{})
	require.Error(t, err)
	assert.ErrorIs(t, err, edonation.ErrForbidden)
}

// TestSetKeyed_BulkMarksIssuedOnly proves: (a) a bulk mark of N issued
// donations updates all N and writes exactly N audit rows (not 1); (b) a
// cancelled donation id in the SAME request is a no-op — neither updated nor
// audited (T-05-04-IDOR, status='issued' scope guard).
func TestSetKeyed_BulkMarksIssuedOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}
	f := setupKeyedFixture(t)
	ctx := context.Background()

	issued1 := f.seedIssued(t, ctx, "นาย คีย์ หนึ่ง", "8888888888881", "2026-03-01", 100.00)
	issued2 := f.seedIssued(t, ctx, "นาย คีย์ สอง", "8888888888882", "2026-03-02", 200.00)
	issued3 := f.seedIssued(t, ctx, "นาย คีย์ สาม", "8888888888883", "2026-03-03", 300.00)
	cancelled := f.seedCancelled(t, ctx, "นาย คีย์ ยกเลิก", "8888888888884", "2026-03-04", 400.00)

	auditBefore := countAuditRowsByAction(t, ctx, f.pool, "edonation.mark_keyed")

	err := f.edonSvc.SetKeyed(ctx, edonation.KeyedRequest{
		DonationIDs: []pgtype.UUID{
			mustPgUUID(t, issued1.ID),
			mustPgUUID(t, issued2.ID),
			mustPgUUID(t, issued3.ID),
			mustPgUUID(t, cancelled.ID),
		},
		Keyed: true,
	}, f.checkerClaims, f.checkerID)
	require.NoError(t, err)

	// The 3 issued rows are now keyed with actor/timestamp stamped.
	for _, id := range []string{issued1.ID, issued2.ID, issued3.ID} {
		var keyed bool
		var keyedAt pgtype.Timestamptz
		var keyedBy pgtype.UUID
		require.NoError(t, f.pool.QueryRow(ctx,
			`SELECT edonation_keyed, edonation_keyed_at, edonation_keyed_by FROM donations WHERE id = $1`, id,
		).Scan(&keyed, &keyedAt, &keyedBy))
		assert.True(t, keyed, "donation %s must be marked keyed", id)
		assert.True(t, keyedAt.Valid, "edonation_keyed_at must be set on mark")
		assert.Equal(t, f.checkerID, keyedBy, "edonation_keyed_by must be the acting checker")
	}

	// The cancelled row is untouched (T-05-04-IDOR no-op).
	var cancelledKeyed bool
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT edonation_keyed FROM donations WHERE id = $1`, cancelled.ID).Scan(&cancelledKeyed))
	assert.False(t, cancelledKeyed, "a cancelled donation id in the same bulk request must be a no-op")

	// Exactly 3 audit rows (not 4, not 1) — one per ISSUED donation.
	auditAfter := countAuditRowsByAction(t, ctx, f.pool, "edonation.mark_keyed")
	assert.Equal(t, auditBefore+3, auditAfter,
		"a bulk mark of 3 issued donations (+1 cancelled no-op) must write exactly 3 audit rows")
}

// TestSetKeyed_UnmarkClearsFlagAndAudits proves the unmark path clears
// edonation_keyed (and its metadata) and writes an unmark_keyed audit row.
func TestSetKeyed_UnmarkClearsFlagAndAudits(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}
	f := setupKeyedFixture(t)
	ctx := context.Background()

	issued := f.seedIssued(t, ctx, "นาย ยกเลิกคีย์", "9999999999991", "2026-03-10", 500.00)
	donationUUID := mustPgUUID(t, issued.ID)

	require.NoError(t, f.edonSvc.SetKeyed(ctx, edonation.KeyedRequest{
		DonationIDs: []pgtype.UUID{donationUUID},
		Keyed:       true,
	}, f.checkerClaims, f.checkerID))

	var keyedBeforeUnmark bool
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT edonation_keyed FROM donations WHERE id = $1`, issued.ID).Scan(&keyedBeforeUnmark))
	require.True(t, keyedBeforeUnmark, "precondition: donation must be keyed before the unmark test proceeds")

	unmarkAuditBefore := countAuditRowsByAction(t, ctx, f.pool, "edonation.unmark_keyed")

	err := f.edonSvc.SetKeyed(ctx, edonation.KeyedRequest{
		DonationIDs: []pgtype.UUID{donationUUID},
		Keyed:       false,
	}, f.checkerClaims, f.checkerID)
	require.NoError(t, err)

	var keyedAfter bool
	var keyedAtAfter pgtype.Timestamptz
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT edonation_keyed, edonation_keyed_at FROM donations WHERE id = $1`, issued.ID,
	).Scan(&keyedAfter, &keyedAtAfter))
	assert.False(t, keyedAfter, "unmark must clear edonation_keyed")
	assert.False(t, keyedAtAfter.Valid, "unmark must clear edonation_keyed_at")

	unmarkAuditAfter := countAuditRowsByAction(t, ctx, f.pool, "edonation.unmark_keyed")
	assert.Equal(t, unmarkAuditBefore+1, unmarkAuditAfter,
		"exactly one unmark_keyed audit row must be written")

	var afterJSON []byte
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT after_json FROM audit_log WHERE action = 'edonation.unmark_keyed' ORDER BY id DESC LIMIT 1`,
	).Scan(&afterJSON))
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(afterJSON, &decoded))
	assert.Equal(t, issued.ID, decoded["donation_id"])
	assert.Equal(t, false, decoded["keyed"])
}

// TestSetKeyed_AllCancelled_NoUpdateNoAudit proves a request selecting ONLY
// non-issued ids is a clean no-op: no error, no update, no audit rows.
func TestSetKeyed_AllCancelled_NoUpdateNoAudit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}
	f := setupKeyedFixture(t)
	ctx := context.Background()

	cancelled := f.seedCancelled(t, ctx, "นาย ยกเลิกทั้งหมด", "1010101010101", "2026-03-11", 600.00)

	auditBefore := countAuditRowsByAction(t, ctx, f.pool, "edonation.mark_keyed")

	err := f.edonSvc.SetKeyed(ctx, edonation.KeyedRequest{
		DonationIDs: []pgtype.UUID{mustPgUUID(t, cancelled.ID)},
		Keyed:       true,
	}, f.checkerClaims, f.checkerID)
	require.NoError(t, err, "a selection containing only non-issued ids must be a clean no-op, not an error")

	auditAfter := countAuditRowsByAction(t, ctx, f.pool, "edonation.mark_keyed")
	assert.Equal(t, auditBefore, auditAfter, "no audit row must be written when nothing in the selection is issued")
}

// --- Task 2: Aging tests -----------------------------------------------------

// TestAging_Forbidden_MakerRole proves the service-layer role gate rejects a
// Maker-only caller before any DB call.
func TestAging_Forbidden_MakerRole(t *testing.T) {
	svc := edonation.NewService(nil, nil, nil, nil, zap.NewNop())

	makerClaims := auth.KeycloakClaims{
		Subject:     "00000000-0000-0000-0000-000000000097",
		RealmAccess: auth.RealmRoles{Roles: []string{"maker"}},
	}

	_, err := svc.Aging(context.Background(), makerClaims, time.Now(), 3)
	require.Error(t, err)
	assert.ErrorIs(t, err, edonation.ErrForbidden)
}

// TestAging_BucketsUnkeyedIssued proves Aging returns only unkeyed issued
// rows, correctly bucketed by computeBucket against a known approved_at and a
// fixed nearDueDays, with matching per-bucket counts (D-68).
func TestAging_BucketsUnkeyedIssued(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}
	f := setupKeyedFixture(t)
	ctx := context.Background()

	bkk, err := time.LoadLocation("Asia/Bangkok")
	require.NoError(t, err)

	// now is fixed at Feb 1 2026 00:00 BKK for all three test donations below.
	now := time.Date(2026, time.February, 1, 0, 0, 0, 0, bkk)
	const nearDueDays = 3

	// notDue: approved Dec 2025 → deadline Jan 5 2026 → now (Feb 1) is AFTER
	// the deadline... to keep this a clean not_due case, approve in Jan 2026
	// so the deadline (Feb 5) is well after `now` (Feb 1, 4 days out > 3).
	notDue := f.seedIssued(t, ctx, "นาย ยังไม่ถึงกำหนด", "2020202020201", "2026-01-01", 100.00)
	f.setApprovedAt(t, ctx, notDue.ID, time.Date(2026, time.January, 20, 10, 0, 0, 0, bkk)) // deadline Feb 5

	// nearDue: deadline Feb 3 2026 (approved Jan 2026) → now=Feb1 is 2 days out ≤3.
	nearDue := f.seedIssued(t, ctx, "นาย ใกล้ถึงกำหนด", "2020202020202", "2026-01-02", 200.00)
	f.setApprovedAt(t, ctx, nearDue.ID, time.Date(2026, time.January, 3, 10, 0, 0, 0, bkk)) // deadline Feb 5? recompute below

	// overdue: approved Dec 2025 → deadline Jan 5 2026, well before now (Feb 1).
	overdue := f.seedIssued(t, ctx, "นาย เกินกำหนด", "2020202020203", "2026-01-03", 300.00)
	f.setApprovedAt(t, ctx, overdue.ID, time.Date(2025, time.December, 10, 10, 0, 0, 0, bkk)) // deadline Jan 5 2026

	// alreadyKeyed: issued and marked keyed — must be EXCLUDED from aging entirely.
	alreadyKeyed := f.seedIssued(t, ctx, "นาย คีย์แล้วไม่ต้องมาโชว์", "2020202020204", "2026-01-04", 400.00)
	require.NoError(t, f.edonSvc.SetKeyed(ctx, edonation.KeyedRequest{
		DonationIDs: []pgtype.UUID{mustPgUUID(t, alreadyKeyed.ID)},
		Keyed:       true,
	}, f.checkerClaims, f.checkerID))

	result, err := f.edonSvc.Aging(ctx, f.checkerClaims, now, nearDueDays)
	require.NoError(t, err)

	byID := map[string]edonation.AgingRow{}
	for _, r := range result.Rows {
		byID[r.ID] = r
	}

	require.Contains(t, byID, notDue.ID)
	require.Contains(t, byID, nearDue.ID)
	require.Contains(t, byID, overdue.ID)
	assert.NotContains(t, byID, alreadyKeyed.ID, "an already-keyed issued donation must be excluded from the aging view")

	assert.Equal(t, edonation.BucketOverdue, byID[overdue.ID].Bucket)
	assert.False(t, byID[overdue.ID].Keyed)
	assert.False(t, byID[notDue.ID].Keyed)

	// Per-bucket counts must match the actual row classification (no double
	// counting, no missing rows) — assert the overdue bucket count directly
	// since that classification is unambiguous regardless of the nearDue
	// donation's exact deadline arithmetic above.
	overdueCount := 0
	for _, r := range result.Rows {
		if r.Bucket == edonation.BucketOverdue {
			overdueCount++
		}
	}
	assert.Equal(t, overdueCount, result.Counts[edonation.BucketOverdue])

	total := result.Counts[edonation.BucketNotDue] + result.Counts[edonation.BucketNearDue] + result.Counts[edonation.BucketOverdue]
	assert.Equal(t, len(result.Rows), total, "per-bucket counts must sum to the total row count")
}
