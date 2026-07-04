// Package main — e2e_test.go
//
// End-to-end HTTP integration test satisfying the "integration-test gate"
// (.planning/CONVENTIONS.md): it drives the REAL request seam
//
//	HTTP → RequireAuth (real signed JWT) → RequireAnyRole → ResolveAppUser
//	     → handler → service → DB
//
// against a Postgres testcontainer and a local OIDC/JWKS test server that mints
// realistic Keycloak-shaped tokens (aud=donnarec-backend, realm_access.roles,
// iss=test issuer, sub=provisioned users' keycloak_subject).
//
// This is the regression guard for three seam bugs that isolated unit/service
// tests structurally cannot catch:
//
//	created-by-fk-mismatch  — created_by must be users.id, not the raw sub
//	fe-be-audience-mismatch — aud must include donnarec-backend or verify 401s
//	RBAC AND-bug            — RequireAnyRole must accept "any of", not "all of"
//
// The test lives in package main (not an _test package) because setupRouter is
// unexported and we REUSE it verbatim — the same wiring main() ships to prod.
//
// Requires Docker testcontainers. Skip with -short like the other integration tests.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/donnarec/donnarec-api/internal/audit"
	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/crypto"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/donation"
	"github.com/donnarec/donnarec-api/internal/receiptno"
	"github.com/donnarec/donnarec-api/internal/storage"
	"github.com/donnarec/donnarec-api/internal/testutil"
	"github.com/donnarec/donnarec-api/internal/users"
)

// e2eTestKEK is a 32-byte hex key for integration test use only (same value as
// the donation package integration tests — test-only, never a real secret).
const e2eTestKEK = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

// backendClientID is the audience the real verifier enforces (KEYCLOAK_CLIENT_ID).
const backendClientID = "donnarec-backend"

// e2eHarness bundles the fully-wired router (via the production setupRouter) plus
// the handles a test needs to provision users and mint matching tokens.
type e2eHarness struct {
	router  *gin.Engine
	kc      *testutil.KeycloakTestServer
	queries *db.Queries
	ctx     context.Context
}

// newE2EHarness spins a Postgres testcontainer, a local OIDC/JWKS server, builds
// the REAL auth middleware + services + handlers, and wires them through the
// production setupRouter. All cleanup is registered on t.
func newE2EHarness(t *testing.T) *e2eHarness {
	t.Helper()

	gin.SetMode(gin.TestMode)
	ctx := context.Background()

	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)

	t.Setenv("DONAREC_KEK", e2eTestKEK)
	keyProvider, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err, "crypto key provider must construct with test KEK")

	logger := zap.NewNop()

	// Local OIDC/JWKS server + REAL auth middleware pointed at it. discovery base
	// is the test server URL; expected issuer is its realm URL — signature, iss,
	// and aud verification all run for real (fe-be-audience-mismatch guard).
	kc := testutil.NewKeycloakTestServer(t)
	authMW, err := auth.NewAuthMiddleware(kc.Server.URL, "donnarec", backendClientID, kc.IssuerURL, logger)
	require.NoError(t, err, "NewAuthMiddleware must succeed against the test OIDC server")

	// Services (mirror cmd/server/main.go wiring).
	auditSvc := audit.NewAuditService(pool, queries, logger)
	userSvc := users.NewUserService(pool, queries, logger)
	allocator := receiptno.NewAllocator(queries)
	donationSvc := donation.NewDonationService(pool, queries, allocator, auditSvc, keyProvider, logger)

	// SlipService needs a *storage.StorageClient. The create/submit/approve flow
	// never touches slip storage, so we construct a real client against a dummy
	// endpoint: minio.New is lazy (no network call at construction), which is all
	// setupRouter requires. No slip route is exercised in this test.
	storageClient, err := storage.NewStorageClient("localhost:9000", "minioadmin", "minioadmin", "donnarec-slips", false)
	require.NoError(t, err, "storage client must construct (lazy — no connection)")
	slipSvc := donation.NewSlipService(pool, queries, storageClient, auditSvc, logger)

	// Handlers.
	userHandler := users.NewUserHandler(userSvc, logger)
	donationHandler := donation.NewDonationHandler(donationSvc, logger)
	slipHandler := donation.NewSlipHandler(slipSvc, logger)

	// appUserResolver: identical closure to main.go — maps Keycloak sub -> users.id,
	// translating pgx.ErrNoRows to auth.ErrSubjectNotProvisioned (403 in middleware).
	appUserResolver := func(ctx context.Context, subject string) (pgtype.UUID, error) {
		u, err := queries.GetUserByKeycloakSubject(ctx, subject)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return pgtype.UUID{}, auth.ErrSubjectNotProvisioned
			}
			return pgtype.UUID{}, err
		}
		return u.ID, nil
	}

	router := setupRouter(authMW, auditSvc, appUserResolver, userHandler, donationHandler, slipHandler, logger)

	return &e2eHarness{router: router, kc: kc, queries: queries, ctx: ctx}
}

// provisionUser inserts a users row with the given keycloak_subject and assigns
// the given roles, mirroring the seed pattern. Returns the resolved users.id.
func (h *e2eHarness) provisionUser(t *testing.T, email, displayName, subject string, roles ...db.UserRoleEnum) pgtype.UUID {
	t.Helper()
	u, err := h.queries.CreateUser(h.ctx, db.CreateUserParams{
		Email:           email,
		DisplayName:     displayName,
		KeycloakSubject: subject,
	})
	require.NoError(t, err, "CreateUser must succeed")
	for _, r := range roles {
		_, err := h.queries.AssignRole(h.ctx, db.AssignRoleParams{UserID: u.ID, Role: r})
		require.NoError(t, err, "AssignRole must succeed")
	}
	return u.ID
}

// do performs an HTTP request against the wired router with an optional bearer
// token and JSON body, returning the recorder for assertions.
func (h *e2eHarness) do(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, path, reader)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	return w
}

// dataEnvelope decodes the {"data": ...} wrapper every donation handler returns.
// Data is donation.DonationDetailResponse (D-R3 detail contract remediation) — the
// FE-aligned shape with server-computed auth flags (viewer_is_creator/can_approve/...)
// that GetByID and every mutation now return.
type dataEnvelope struct {
	Data donation.DonationDetailResponse `json:"data"`
}

// listEnvelope decodes the D-R2 pagination envelope
// {"data": {"items": [...], "total": N, "page": P, "per_page": 20}} — NOT a bare array.
type listEnvelope struct {
	Data donation.DonationListResult `json:"data"`
}

func decodeDonation(t *testing.T, w *httptest.ResponseRecorder) donation.DonationDetailResponse {
	t.Helper()
	var env dataEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "response body: %s", w.Body.String())
	return env.Data
}

// thirteenConsecutiveDigits matches a run of 13 consecutive digits — the shape of a
// plaintext Thai national/tax ID. national_id_masked must NEVER match this (T-11-02).
var thirteenConsecutiveDigits = regexp.MustCompile(`\d{13}`)

// validDonorBody returns a valid Create/Reissue donor payload.
func validDonorBody(name string) map[string]any {
	return map[string]any{
		"donor_name":           name,
		"donor_tax_id":         "1234567890123",
		"donor_address":        "123 ถนนทดสอบ กรุงเทพฯ",
		"amount":               1500.00,
		"donated_at":           "2026-03-15",
		"consent_given":        true,
		"consent_text_version": "v1",
		"consent_purpose":      "tax-receipt",
	}
}

// TestE2E_MakerCheckerIssuancePipeline drives the full Maker→Checker→Issuance
// pipeline over the real HTTP path, plus the seam regressions as subtests.
//
// Requires Docker testcontainers. Skip with -short.
func TestE2E_MakerCheckerIssuancePipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E integration test in short mode: requires Docker")
	}

	h := newE2EHarness(t)

	// Provision maker + checker with DISTINCT keycloak_subject values (SoD needs
	// approver_id != created_by). Tokens' sub matches each provisioned subject.
	const subMaker = "11111111-1111-1111-1111-111111111111"
	const subChecker = "22222222-2222-2222-2222-222222222222"
	makerID := h.provisionUser(t, "maker-e2e@example.com", "Maker E2E", subMaker, db.UserRoleEnumMaker)
	_ = h.provisionUser(t, "checker-e2e@example.com", "Checker E2E", subChecker, db.UserRoleEnumChecker)

	makerToken := h.kc.MintTokenForSubject(subMaker, backendClientID, "maker")
	checkerToken := h.kc.MintTokenForSubject(subChecker, backendClientID, "checker")

	// donationID is threaded across the happy-path steps (create → submit → approve → list).
	var donationID string

	t.Run("HappyPath_CreateSubmitApproveList", func(t *testing.T) {
		// --- Step 1: POST /api/donations (maker) → 201 ---
		// RBAC AND-bug regression: a maker-only token MUST be accepted here
		// (RequireAnyRole = "any of"), not 403.
		w := h.do(t, http.MethodPost, "/api/donations", makerToken, validDonorBody("นาย ทดสอบ E2E"))
		require.Equal(t, http.StatusCreated, w.Code, "maker must be accepted on create (RequireAnyRole); body: %s", w.Body.String())
		created := decodeDonation(t, w)
		donationID = created.ID
		require.NotEmpty(t, donationID)
		assert.Equal(t, "draft", created.Status)

		// created-by-fk-mismatch regression: created_by_id is the maker's users.id,
		// NOT the raw Keycloak subject (subMaker). created_by is now the creator's
		// DISPLAY NAME under the D-R3 detail contract, not a UUID.
		assert.Equal(t, makerID.String(), created.CreatedByID,
			"created_by_id must be the resolved users.id, not the keycloak subject")
		assert.NotEqual(t, subMaker, created.CreatedByID,
			"created_by_id must NOT be the raw keycloak subject (created-by-fk-mismatch)")
		assert.Equal(t, "Maker E2E", created.CreatedBy,
			"created_by must be the creator's display name (users join), not a raw UUID")

		// --- Step 2: POST /api/donations/{id}/submit (maker) → 200, pending_review ---
		w = h.do(t, http.MethodPost, "/api/donations/"+donationID+"/submit", makerToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "submit body: %s", w.Body.String())
		submitted := decodeDonation(t, w)
		assert.Equal(t, "pending_review", submitted.Status)

		// --- Step 2b: GET /api/donations/{id} — D-R3 detail contract, driven by BOTH
		// tokens on the same pending_review record. Server-computed auth flags (T-03-31)
		// must reflect each viewer's own resolved identity + role, never trust the client. ---

		// Maker viewing their own record: viewer_is_creator=true, can_approve=false (SoD).
		w = h.do(t, http.MethodGet, "/api/donations/"+donationID, makerToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "get-by-maker body: %s", w.Body.String())
		detailAsMaker := decodeDonation(t, w)
		assert.True(t, detailAsMaker.ViewerIsCreator,
			"maker viewing their own record: viewer_is_creator must be true")
		assert.False(t, detailAsMaker.CanApprove,
			"maker viewing their own record: can_approve must be false (SoD — creator cannot approve)")

		// Checker viewing the maker's record while pending_review: viewer_is_creator=false,
		// can_approve=true.
		w = h.do(t, http.MethodGet, "/api/donations/"+donationID, checkerToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "get-by-checker body: %s", w.Body.String())
		detailAsChecker := decodeDonation(t, w)
		assert.False(t, detailAsChecker.ViewerIsCreator,
			"checker viewing the maker's record: viewer_is_creator must be false")
		assert.True(t, detailAsChecker.CanApprove,
			"checker viewing a pending_review record they did not create: can_approve must be true")

		// national_id_masked must be present and NEVER contain a run of 13 consecutive
		// digits (i.e. never the plaintext national ID — T-11-02, FR-29).
		assert.NotEmpty(t, detailAsChecker.NationalIDMasked, "national_id_masked must be present")
		assert.False(t, thirteenConsecutiveDigits.MatchString(detailAsChecker.NationalIDMasked),
			"national_id_masked must not contain a run of 13 consecutive digits (never plaintext)")

		// created_by/created_by_id must match the maker on the detail contract too.
		assert.Equal(t, "Maker E2E", detailAsChecker.CreatedBy,
			"detail created_by must be the maker's display name")
		assert.Equal(t, makerID.String(), detailAsChecker.CreatedByID,
			"detail created_by_id must equal the maker's users.id")

		// edonation_keyed must be present as a bool field — false for a freshly created record.
		assert.False(t, detailAsChecker.EdonationKeyed,
			"edonation_keyed must default to false for a freshly submitted donation")

		// --- Step 3: POST /api/donations/{id}/approve (checker) → 200, issued ---
		// RBAC regression: a checker-only token (no admin) MUST be accepted on the
		// checker-only route, not 403.
		w = h.do(t, http.MethodPost, "/api/donations/"+donationID+"/approve", checkerToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "checker must be accepted on approve; body: %s", w.Body.String())
		issued := decodeDonation(t, w)
		assert.Equal(t, "issued", issued.Status)
		require.NotNil(t, issued.ReceiptFormatted, "receipt_formatted must be set after issuance")
		assert.NotEmpty(t, *issued.ReceiptFormatted,
			"receipt number must be allocated in the issuance tx (gap-less counter)")

		// --- Step 4: GET /api/donations?status=issued (checker) → 200, D-R2 envelope ---
		// bug #5 regression guard: the response body MUST be the nested pagination
		// envelope {"data":{"items":[...],"total":N,"page":P,"per_page":20}}, never a
		// bare {"data":[...]} array (that shape crashes DonationTable on undefined.length).
		w = h.do(t, http.MethodGet, "/api/donations?status=issued", checkerToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "list body: %s", w.Body.String())
		var list listEnvelope
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list), "response body: %s", w.Body.String())
		assert.Equal(t, 1, list.Data.Page, "default page must be 1")
		assert.Equal(t, 20, list.Data.PerPage, "per_page must be fixed at 20 (D-R2)")
		assert.GreaterOrEqual(t, list.Data.Total, int64(1),
			"total must be a real COUNT over the status=issued filter, not len(items)")
		found := false
		for _, d := range list.Data.Items {
			if d.ID == donationID {
				found = true
				assert.Equal(t, "issued", d.Status)
				assert.Equal(t, makerID.String(), d.CreatedByID,
					"created_by_id must be the maker's users.id UUID so the UI can route own-drafts")
				assert.Equal(t, "Maker E2E", d.CreatedBy,
					"created_by must be the creator's display name (users join), not a raw UUID")
			}
		}
		assert.True(t, found, "issued donation %s must appear in status=issued list", donationID)
	})

	t.Run("UnprovisionedSubject_403", func(t *testing.T) {
		// Validly-signed token whose sub has NO users row → ResolveAppUser 403s.
		orphanToken := h.kc.MintTokenForSubject("99999999-9999-9999-9999-999999999999", backendClientID, "maker")
		w := h.do(t, http.MethodPost, "/api/donations", orphanToken, validDonorBody("นาย ไม่มีในระบบ"))
		require.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
		assert.Contains(t, w.Body.String(), "user_not_provisioned")
	})

	t.Run("RBAC_MakerRejectedFromCheckerOnlyRoute", func(t *testing.T) {
		// Defense-in-depth complement to the happy path: a maker-only token is
		// blocked by the checker route guard (RequireAnyRole(checker,admin)) with
		// 403 "insufficient_role" — distinct from the SoD 403 ("sod_violation") below.
		w := h.do(t, http.MethodPost, "/api/donations/"+donationID+"/approve", makerToken, nil)
		require.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
		assert.Contains(t, w.Body.String(), "insufficient_role")
	})

	t.Run("SoD_SelfApprove_403", func(t *testing.T) {
		// A dual-role (maker+checker) user creates+submits, then approves their own
		// donation. The checker route guard passes (they hold checker), so the
		// request reaches the SERVICE SoD check: approver_id == created_by →
		// ErrSoDViolation, which the handler maps to 403 {"error":"sod_violation"}.
		const subDual = "33333333-3333-3333-3333-333333333333"
		_ = h.provisionUser(t, "dual-e2e@example.com", "Dual E2E", subDual, db.UserRoleEnumMaker, db.UserRoleEnumChecker)
		dualToken := h.kc.MintTokenForSubject(subDual, backendClientID, "maker", "checker")

		w := h.do(t, http.MethodPost, "/api/donations", dualToken, validDonorBody("นาย ทดสอบ SoD"))
		require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
		selfID := decodeDonation(t, w).ID

		w = h.do(t, http.MethodPost, "/api/donations/"+selfID+"/submit", dualToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

		w = h.do(t, http.MethodPost, "/api/donations/"+selfID+"/approve", dualToken, nil)
		require.Equal(t, http.StatusForbidden, w.Code, "self-approve must be blocked by SoD; body: %s", w.Body.String())
		assert.Contains(t, w.Body.String(), "sod_violation")
	})

	t.Run("Audience_WrongClient_401", func(t *testing.T) {
		// Token minted with aud="wrong-client" must fail the real audience check
		// (fe-be-audience-mismatch guard) → 401 invalid_token, before any handler.
		wrongAudToken := h.kc.MintTokenForSubject(subMaker, "wrong-client", "maker")
		w := h.do(t, http.MethodPost, "/api/donations", wrongAudToken, validDonorBody("นาย ผิด audience"))
		require.Equal(t, http.StatusUnauthorized, w.Code, "body: %s", w.Body.String())
		assert.Contains(t, w.Body.String(), "invalid_token")
	})

	t.Run("Cancel_RetainsReceiptNumber_RealPath", func(t *testing.T) {
		// Create → submit → approve a fresh donation over the real router so this
		// subtest owns its own record (independent of HappyPath's donationID).
		w := h.do(t, http.MethodPost, "/api/donations", makerToken, validDonorBody("นาง ทดสอบ ยกเลิก"))
		require.Equal(t, http.StatusCreated, w.Code, "create body: %s", w.Body.String())
		cancelTargetID := decodeDonation(t, w).ID

		w = h.do(t, http.MethodPost, "/api/donations/"+cancelTargetID+"/submit", makerToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "submit body: %s", w.Body.String())

		w = h.do(t, http.MethodPost, "/api/donations/"+cancelTargetID+"/approve", checkerToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "approve body: %s", w.Body.String())
		issuedForCancel := decodeDonation(t, w)
		require.NotNil(t, issuedForCancel.ReceiptFormatted, "receipt_formatted must be set after issuance")
		originalReceipt := *issuedForCancel.ReceiptFormatted
		require.NotEmpty(t, originalReceipt)

		// A maker-only token must be rejected from the checker-only cancel route
		// (RequireAnyRole(checker,admin) route guard) — 403, before the service layer.
		w = h.do(t, http.MethodPost, "/api/donations/"+cancelTargetID+"/cancel", makerToken,
			map[string]any{"reason": "e2e cancel — maker attempt"})
		require.Equal(t, http.StatusForbidden, w.Code,
			"maker token must be rejected from the checker-only cancel route; body: %s", w.Body.String())
		assert.Contains(t, w.Body.String(), "insufficient_role")

		// POST /api/donations/{id}/cancel (checker) → 200, status=cancelled, and the
		// receipt number is RETAINED — cancelling must never create a gap in the
		// gap-less counter (FR-19, D-47, the load-bearing invariant of the project).
		w = h.do(t, http.MethodPost, "/api/donations/"+cancelTargetID+"/cancel", checkerToken,
			map[string]any{"reason": "e2e cancel"})
		require.Equal(t, http.StatusOK, w.Code, "cancel body: %s", w.Body.String())
		cancelled := decodeDonation(t, w)
		assert.Equal(t, "cancelled", cancelled.Status)
		require.NotNil(t, cancelled.ReceiptFormatted,
			"receipt_formatted must still be set after cancellation — the number is retained, never cleared")
		assert.NotEmpty(t, *cancelled.ReceiptFormatted,
			"receipt number must be retained on cancel (gap-less invariant — cancelled records keep their number)")
		assert.Equal(t, originalReceipt, *cancelled.ReceiptFormatted,
			"the cancelled record's receipt number must be identical to the pre-cancel issued number")
	})
}
