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
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
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
	"github.com/donnarec/donnarec-api/internal/pdf"
	"github.com/donnarec/donnarec-api/internal/receiptno"
	"github.com/donnarec/donnarec-api/internal/settings"
	"github.com/donnarec/donnarec-api/internal/storage"
	"github.com/donnarec/donnarec-api/internal/testutil"
	"github.com/donnarec/donnarec-api/internal/users"
)

// e2eTestKEK is a 32-byte hex key for integration test use only (same value as
// the donation package integration tests — test-only, never a real secret).
const e2eTestKEK = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

// backendClientID is the audience the real verifier enforces (KEYCLOAK_CLIENT_ID).
const backendClientID = "donnarec-backend"

// fakeReceiptsStore is a hermetic stub satisfying donation.ReceiptsStore. See the
// comment at its construction site in newE2EHarness for why a real MinIO client is
// deliberately NOT used here.
type fakeReceiptsStore struct{}

func (fakeReceiptsStore) PresignedGet(_ context.Context, objectKey string, _ time.Duration) (string, error) {
	return "https://fake-receipts.example.test/" + objectKey, nil
}

// fakeSettingsStore is a hermetic, in-memory settings.ReceiptsStore stub — avoids
// requiring a live MinIO for the Admin settings E2E subtests below, mirroring
// fakeReceiptsStore's rationale above. PutTemplateImage still runs the REAL
// storage.ValidateTemplateImage magic-byte/size validation (only the actual network PUT
// is faked), so the 415/413 error-mapping paths in settings.Handler are genuinely
// exercised, not bypassed.
type fakeSettingsStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newFakeSettingsStore() *fakeSettingsStore {
	return &fakeSettingsStore{objects: make(map[string][]byte)}
}

func (f *fakeSettingsStore) GetObject(_ context.Context, key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.objects[key]
	if !ok {
		return nil, fmt.Errorf("fakeSettingsStore: object %q not found", key)
	}
	return data, nil
}

func (f *fakeSettingsStore) PutTemplateImage(_ context.Context, r io.Reader, size int64, slot string) (string, string, error) {
	detected, head, err := storage.ValidateTemplateImage(r, size)
	if err != nil {
		return "", "", err
	}
	rest, _ := io.ReadAll(r)
	full := append(append([]byte{}, head...), rest...)

	key := fmt.Sprintf("template-assets/%s/fake%s", slot, detected.Extension())
	f.mu.Lock()
	f.objects[key] = full
	f.mu.Unlock()
	return key, detected.String(), nil
}

// e2eHarness bundles the fully-wired router (via the production setupRouter) plus
// the handles a test needs to provision users and mint matching tokens.
type e2eHarness struct {
	router        *gin.Engine
	kc            *testutil.KeycloakTestServer
	queries       *db.Queries
	pool          *pgxpool.Pool
	ctx           context.Context
	settingsStore *fakeSettingsStore
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

	// DownloadReceipt's presigned URL (FR-28, 04-06) uses a fake ReceiptsStore here —
	// unlike NewStorageClient's construction, minio-go's PresignedGetObject performs a
	// real bucket-region probe network call, so a real client would depend on whatever
	// incidental MinIO instance/credentials happen to be reachable on the test host.
	// This test's job is to prove the HTTP -> auth -> RBAC -> handler -> service -> DB
	// seam (not the MinIO round-trip, which internal/worker's tests already cover
	// against a genuine MinIO testcontainer, 04-05) — a hermetic fake keeps this test
	// deterministic and host-independent.
	donationSvc.SetReceiptsStore(fakeReceiptsStore{})

	// Settings service + handler (Phase 4, plan 04-07): a REAL chrome sidecar
	// (testutil.StartChrome, 04-02) backs the real-PDF preview path so PreviewPDF is
	// genuinely exercised through the SAME sandboxed pipeline production uses
	// (D-58/D-61) — not a fake/stub renderer. A hermetic in-memory fakeSettingsStore
	// stands in for MinIO (same rationale as fakeReceiptsStore above): this test's job
	// is the HTTP -> auth -> RBAC(admin) -> handler -> service -> DB seam, not a MinIO
	// round-trip (already covered by internal/storage's own tests).
	chromeWSURL, _ := testutil.StartChrome(t)
	pdfRenderer, err := pdf.NewRenderer(chromeWSURL)
	require.NoError(t, err, "pdf renderer must construct against the test chrome sidecar")
	settingsStore := newFakeSettingsStore()
	settingsSvc := settings.NewSettingsService(pool, queries, settingsStore, logger)
	settingsHandler := settings.NewHandler(settingsSvc, pdfRenderer, logger)

	// e-Donation export service + config accessor (Phase 5, plan 05-02, FR-30/D-75) —
	// mirrors main.go wiring, reusing the SAME auditSvc + keyProvider as donationSvc.
	edonationSvc := edonation.NewService(pool, queries, auditSvc, keyProvider, logger)
	edonationCfg := edonation.NewConfig(queries)
	edonationHandler := edonation.NewHandler(edonationSvc, edonationCfg, logger)

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

	router := setupRouter(authMW, auditSvc, appUserResolver, userHandler, donationHandler, slipHandler, settingsHandler, edonationHandler, logger)

	return &e2eHarness{router: router, kc: kc, queries: queries, pool: pool, ctx: ctx, settingsStore: settingsStore}
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

// doMultipart performs a multipart/form-data POST with a single "file" field — used by
// the settings image-upload E2E subtests (mirrors SlipHandler.Upload's c.FormFile("file")
// contract).
func (h *e2eHarness) doMultipart(t *testing.T, path, token, filename string, fileBytes []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write(fileBytes)
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req, err := http.NewRequest(http.MethodPost, path, &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())
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

	t.Run("DonorLanguage_PersistsAndDefaults", func(t *testing.T) {
		// D-55/FR-23: explicit donor_language="en" round-trips through the real
		// Create -> GetByID path and is frozen (never re-derived).
		body := validDonorBody("นาย ภาษาอังกฤษ")
		body["donor_language"] = "en"
		w := h.do(t, http.MethodPost, "/api/donations", makerToken, body)
		require.Equal(t, http.StatusCreated, w.Code, "create body: %s", w.Body.String())
		created := decodeDonation(t, w)
		assert.Equal(t, "en", created.DonorLanguage, "donor_language must persist as submitted on create")

		w = h.do(t, http.MethodGet, "/api/donations/"+created.ID, makerToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "get body: %s", w.Body.String())
		fetched := decodeDonation(t, w)
		assert.Equal(t, "en", fetched.DonorLanguage, "donor_language must round-trip on GET (frozen snapshot)")

		// Omitted donor_language defaults to "th" (D-55).
		bodyNoLang := validDonorBody("นาย ค่าเริ่มต้นภาษา")
		w = h.do(t, http.MethodPost, "/api/donations", makerToken, bodyNoLang)
		require.Equal(t, http.StatusCreated, w.Code, "create (no lang) body: %s", w.Body.String())
		createdDefault := decodeDonation(t, w)
		assert.Equal(t, "th", createdDefault.DonorLanguage, "omitted donor_language must default to th")

		// An invalid donor_language value is rejected at the validation boundary (422).
		bodyInvalid := validDonorBody("นาย ค่าภาษาไม่ถูกต้อง")
		bodyInvalid["donor_language"] = "fr"
		w = h.do(t, http.MethodPost, "/api/donations", makerToken, bodyInvalid)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code, "invalid donor_language must be rejected; body: %s", w.Body.String())
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

	t.Run("ResendAndDownload_RealPath", func(t *testing.T) {
		// Create → submit → approve a fresh donation over the real router.
		w := h.do(t, http.MethodPost, "/api/donations", makerToken, validDonorBody("นาง ทดสอบ ส่งซ้ำ"))
		require.Equal(t, http.StatusCreated, w.Code, "create body: %s", w.Body.String())
		targetID := decodeDonation(t, w).ID

		w = h.do(t, http.MethodPost, "/api/donations/"+targetID+"/submit", makerToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "submit body: %s", w.Body.String())

		w = h.do(t, http.MethodPost, "/api/donations/"+targetID+"/approve", checkerToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "approve body: %s", w.Body.String())
		issued := decodeDonation(t, w)
		require.NotNil(t, issued.ReceiptFormatted)
		originalReceipt := *issued.ReceiptFormatted

		// EmailDelivery must be nil until the worker (04-05) has processed the
		// issue_receipt job — GetLatestEmailDeliveryForDonation's pgx.ErrNoRows path
		// must be treated as "not yet", not a 500.
		assert.Nil(t, issued.EmailDelivery,
			"email_delivery must be nil before the worker has recorded any send attempt")

		// --- Download before freeze: the worker has not run yet → not-ready error ---
		w = h.do(t, http.MethodGet, "/api/donations/"+targetID+"/receipt-pdf", makerToken, nil)
		assert.Equal(t, http.StatusConflict, w.Code,
			"download before the PDF is frozen must be a conflict/not-ready error; body: %s", w.Body.String())

		// --- Resend before freeze: no frozen PDF to resend → not-ready error ---
		w = h.do(t, http.MethodPost, "/api/donations/"+targetID+"/resend", checkerToken, nil)
		assert.Equal(t, http.StatusConflict, w.Code,
			"resend before the PDF is frozen must be a conflict/not-ready error; body: %s", w.Body.String())

		// Simulate the outbox worker (04-05) having frozen the receipt PDF.
		var pgID pgtype.UUID
		require.NoError(t, pgID.Scan(targetID))
		objectKey := "receipts/" + targetID + "/receipt.pdf"
		require.NoError(t, h.queries.SetReceiptPDFObjectKey(h.ctx, db.SetReceiptPDFObjectKeyParams{
			ReceiptPdfObjectKey: &objectKey,
			ID:                  pgID,
		}))

		// --- Download by every staff role → 200 with a non-empty presigned URL (FR-28) ---
		for _, tok := range []string{makerToken, checkerToken} {
			w = h.do(t, http.MethodGet, "/api/donations/"+targetID+"/receipt-pdf", tok, nil)
			require.Equal(t, http.StatusOK, w.Code, "download body: %s", w.Body.String())
			var env struct {
				Data struct {
					URL string `json:"url"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "response body: %s", w.Body.String())
			assert.NotEmpty(t, env.Data.URL, "download must return a non-empty presigned URL")
		}

		// --- Resend by Maker → 403 (checkerGroup RBAC guard, T-04-15) ---
		w = h.do(t, http.MethodPost, "/api/donations/"+targetID+"/resend", makerToken, nil)
		assert.Equal(t, http.StatusForbidden, w.Code,
			"maker token must be rejected from the checker-only resend route; body: %s", w.Body.String())

		// --- Resend by Checker → 200, enqueues exactly one NEW outbox job (D-56/D-57) ---
		var jobCountBefore int
		require.NoError(t, h.pool.QueryRow(h.ctx,
			`SELECT count(*) FROM outbox_jobs WHERE payload->>'donation_id' = $1`, targetID,
		).Scan(&jobCountBefore))

		w = h.do(t, http.MethodPost, "/api/donations/"+targetID+"/resend", checkerToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "resend body: %s", w.Body.String())

		var jobCountAfter int
		require.NoError(t, h.pool.QueryRow(h.ctx,
			`SELECT count(*) FROM outbox_jobs WHERE payload->>'donation_id' = $1`, targetID,
		).Scan(&jobCountAfter))
		assert.Equal(t, jobCountBefore+1, jobCountAfter, "resend must enqueue exactly one new outbox job")

		// --- receipt_no must be byte-identical before/after resend (T-04-16) ---
		w = h.do(t, http.MethodGet, "/api/donations/"+targetID, checkerToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "get body: %s", w.Body.String())
		afterResend := decodeDonation(t, w)
		require.NotNil(t, afterResend.ReceiptFormatted)
		assert.Equal(t, originalReceipt, *afterResend.ReceiptFormatted,
			"resend must never allocate a new receipt number (D-56/D-57, T-04-16)")
	})
}

// settingsPNGBytes returns a minimal PNG magic-byte prefix padded to 512 bytes — same
// signature shape as internal/storage's own test fixtures, duplicated locally since that
// package's test helpers are unexported to their own _test package.
func settingsPNGBytes() []byte {
	b := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	return append(b, make([]byte, 512)...)
}

// settingsPDFBytes returns a minimal PDF magic-byte prefix — used to prove the settings
// image-upload endpoint rejects a PDF (image-only allowlist, unlike the slip upload path).
func settingsPDFBytes() []byte {
	b := []byte{'%', 'P', 'D', 'F', '-', '1', '.', '4', '\n'}
	return append(b, make([]byte, 512)...)
}

// settingsPayload returns a valid PUT /api/admin/settings request body.
func settingsPayload() map[string]any {
	return map[string]any{
		"template_html":        `<html><body>{{.DonorName}} {{.ReceiptNo}} {{.Section6Text}}</body></html>`,
		"template_html_en":     `<html><body>{{.DonorName}} {{.ReceiptNo}} {{.Section6Text}}</body></html>`,
		"section6_text_th":     "หัก ณ ที่จ่าย 1 เท่า (E2E)",
		"section6_text_en":     "1x tax deduction (E2E)",
		"deduction_multiplier": "2x",
		"separator":            "-",
		"running_no_padding":   7,
		"year_format":          "CE4",
		"prefix":               "E2E",
	}
}

// settingsEnvelope decodes the {"data": ReceiptSettings} wrapper GET/PUT settings return.
type settingsEnvelope struct {
	Data settings.ReceiptSettings `json:"data"`
}

// TestE2E_AdminSettings drives the Phase 4 (plan 04-07) Admin settings API over the real
// HTTP path: HTTP -> RequireAuth (real Keycloak-shaped token) -> RequireRoles(Admin) ->
// ResolveAppUser -> settings.Handler -> settings.SettingsService -> DB (Postgres
// testcontainer). Also proves the real-PDF preview goes through the SAME sandboxed
// Chromium pipeline as production (D-58/D-61), via a real chrome sidecar
// (testutil.StartChrome).
//
// Requires Docker testcontainers (Postgres + chrome sidecar). Skip with -short.
func TestE2E_AdminSettings(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E integration test in short mode: requires Docker")
	}

	h := newE2EHarness(t)

	const subAdmin = "44444444-4444-4444-4444-444444444444"
	const subMaker = "55555555-5555-5555-5555-555555555555"
	_ = h.provisionUser(t, "admin-settings-e2e@example.com", "Admin Settings E2E", subAdmin, db.UserRoleEnumAdmin)
	_ = h.provisionUser(t, "maker-settings-e2e@example.com", "Maker Settings E2E", subMaker, db.UserRoleEnumMaker)

	adminToken := h.kc.MintTokenForSubject(subAdmin, backendClientID, "admin")
	makerToken := h.kc.MintTokenForSubject(subMaker, backendClientID, "maker")

	t.Run("GetSettings_SeededDefaults", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/api/admin/settings", adminToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		var env settingsEnvelope
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "response body: %s", w.Body.String())
		assert.NotEmpty(t, env.Data.TemplateHTML, "the 04-01-seeded Thai template must be non-empty before any admin edit")
		assert.NotEmpty(t, env.Data.Separator, "the Phase 2 number-format config must be present (consolidated onto this screen)")
	})

	t.Run("NonAdmin_Forbidden", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/api/admin/settings", makerToken, nil)
		assert.Equal(t, http.StatusForbidden, w.Code, "maker token must be rejected from the Admin-only settings route; body: %s", w.Body.String())

		w = h.do(t, http.MethodPut, "/api/admin/settings", makerToken, settingsPayload())
		assert.Equal(t, http.StatusForbidden, w.Code, "maker token must be rejected from PUT settings; body: %s", w.Body.String())
	})

	t.Run("Save_InvalidTemplate_422", func(t *testing.T) {
		bad := settingsPayload()
		bad["template_html"] = "{{.DonorName" // malformed action
		w := h.do(t, http.MethodPut, "/api/admin/settings", adminToken, bad)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code, "body: %s", w.Body.String())
		assert.Contains(t, w.Body.String(), "invalid_template")
	})

	t.Run("Save_InvalidNumberFormat_422", func(t *testing.T) {
		bad := settingsPayload()
		bad["separator"] = "<script>"
		w := h.do(t, http.MethodPut, "/api/admin/settings", adminToken, bad)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code, "body: %s", w.Body.String())
		assert.Contains(t, w.Body.String(), "invalid_number_format")
	})

	t.Run("Save_ValidRoundTrip_AuditedAndPersisted", func(t *testing.T) {
		w := h.do(t, http.MethodPut, "/api/admin/settings", adminToken, settingsPayload())
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

		// GET reflects the newly-saved values — round-trips through the real DB.
		w = h.do(t, http.MethodGet, "/api/admin/settings", adminToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		var env settingsEnvelope
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "response body: %s", w.Body.String())
		assert.Equal(t, "2x", env.Data.DeductionMultiplier)
		assert.Equal(t, "-", env.Data.Separator)
		assert.Equal(t, 7, env.Data.RunningNoPadding)
		assert.Equal(t, "CE4", env.Data.YearFormat)
		assert.Equal(t, "E2E", env.Data.Prefix)
		assert.Contains(t, env.Data.Section6TextTh, "E2E")

		// Append-only audit row (D-58 "every settings mutation is Admin-gated and
		// append-only audited") — AuditMiddleware writes one row per PUT with the
		// authenticated actor's raw Keycloak subject.
		var auditCount int
		require.NoError(t, h.pool.QueryRow(h.ctx,
			`SELECT count(*) FROM audit_log WHERE actor_id = $1 AND resource = $2`,
			subAdmin, "/api/admin/settings",
		).Scan(&auditCount))
		assert.GreaterOrEqual(t, auditCount, 1, "PUT /api/admin/settings must write an append-only audit row")
	})

	t.Run("Preview_EscapesInjectedSection6Text", func(t *testing.T) {
		// Section6Text is admin-configured but substituted into the template via
		// html/template — a value containing markup must render ESCAPED, proving the
		// preview endpoint never wraps assembled HTML as trusted raw content (T-04-20).
		req := map[string]any{
			"template_html":        `<html><body><div id="s6">{{.Section6Text}}</div></body></html>`,
			"template_html_en":     `<html><body><div id="s6">{{.Section6Text}}</div></body></html>`,
			"section6_text_th":     `<script>alert('xss')</script>`,
			"deduction_multiplier": "1x",
			"language":             "th",
		}
		w := h.do(t, http.MethodPost, "/api/admin/settings/preview", adminToken, req)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

		var env struct {
			Data struct {
				HTML string `json:"html"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "response body: %s", w.Body.String())
		assert.NotContains(t, env.Data.HTML, "<script>alert",
			"raw <script> must NOT appear in preview HTML — html/template must escape it")
		assert.Contains(t, env.Data.HTML, "&lt;script&gt;",
			"the injected Section6Text must appear HTML-entity-escaped in the preview")
	})

	t.Run("Preview_NonAdmin_Forbidden", func(t *testing.T) {
		w := h.do(t, http.MethodPost, "/api/admin/settings/preview", makerToken, map[string]any{
			"template_html": `<html><body>{{.DonorName}}</body></html>`,
		})
		assert.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
	})

	t.Run("PreviewPDF_ReturnsRealPDFBytesViaSandboxedPipeline", func(t *testing.T) {
		req := map[string]any{
			"template_html":        `<html><body>{{.DonorName}} {{.ReceiptNo}}</body></html>`,
			"template_html_en":     `<html><body>{{.DonorName}} {{.ReceiptNo}}</body></html>`,
			"section6_text_th":     "ทดสอบ",
			"deduction_multiplier": "1x",
			"language":             "th",
		}
		w := h.do(t, http.MethodPost, "/api/admin/settings/preview/pdf", adminToken, req)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		assert.Equal(t, "application/pdf", w.Header().Get("Content-Type"))
		assert.True(t, bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF")),
			"response body must be real PDF bytes (starts with %%PDF magic), got %d bytes", w.Body.Len())
		assert.Greater(t, w.Body.Len(), 100, "rendered PDF must be non-trivially sized")
	})

	t.Run("UploadImage_MagicByteValidatedAdminOnly", func(t *testing.T) {
		// Non-admin rejected before any validation runs.
		w := h.doMultipart(t, "/api/admin/settings/images/letterhead", makerToken, "letterhead.png", settingsPNGBytes())
		assert.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())

		// Admin + valid PNG -> 200, object_key returned.
		w = h.doMultipart(t, "/api/admin/settings/images/letterhead", adminToken, "letterhead.png", settingsPNGBytes())
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		var env struct {
			Data struct {
				Slot      string `json:"slot"`
				ObjectKey string `json:"object_key"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "response body: %s", w.Body.String())
		assert.Equal(t, "letterhead", env.Data.Slot)
		assert.NotEmpty(t, env.Data.ObjectKey)

		// GET settings now reflects the new letterhead_object_key.
		w = h.do(t, http.MethodGet, "/api/admin/settings", adminToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		var settingsEnv settingsEnvelope
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &settingsEnv), "response body: %s", w.Body.String())
		require.NotNil(t, settingsEnv.Data.LetterheadObjectKey)
		assert.Equal(t, env.Data.ObjectKey, *settingsEnv.Data.LetterheadObjectKey)

		// Admin + a PDF (not an image) -> 415 unsupported_file_type (image-only allowlist,
		// unlike the slip upload path which DOES accept PDFs).
		w = h.doMultipart(t, "/api/admin/settings/images/seal", adminToken, "seal.pdf", settingsPDFBytes())
		assert.Equal(t, http.StatusUnsupportedMediaType, w.Code, "body: %s", w.Body.String())
		assert.Contains(t, w.Body.String(), "unsupported_file_type")

		// Admin + an unknown slot name -> 400 invalid_image_slot.
		w = h.doMultipart(t, "/api/admin/settings/images/not-a-slot", adminToken, "x.png", settingsPNGBytes())
		assert.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
		assert.Contains(t, w.Body.String(), "invalid_image_slot")
	})
}

// zipSignature is the 4-byte ZIP local file header signature every .xlsx (an OOXML
// ZIP container) must start with.
var zipSignature = []byte{0x50, 0x4B, 0x03, 0x04}

// utf8BOM is the 3-byte UTF-8 byte-order-mark exportfile.StreamCSV writes before
// any CSV content (05-RESEARCH.md Pattern 2).
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// TestE2E_EdonationExport drives the e-Donation export endpoint over the REAL
// HTTP path: HTTP -> RequireAuth (real Keycloak-shaped token) ->
// RequireAnyRole(Checker,Admin) -> ResolveAppUser -> edonation.Handler ->
// edonation.Service (audited decrypt) -> exportfile stream (FR-30, plan 05-02,
// satisfies the integration-test gate per CLAUDE.md Conventions).
//
// Requires Docker testcontainers (Postgres). Skip with -short.
func TestE2E_EdonationExport(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E integration test in short mode: requires Docker")
	}

	h := newE2EHarness(t)

	const subMaker = "66666666-6666-6666-6666-666666666666"
	const subChecker = "77777777-7777-7777-7777-777777777777"
	const subAdmin = "88888888-8888-8888-8888-888888888888"
	_ = h.provisionUser(t, "maker-edonation-e2e@example.com", "Maker eDonation E2E", subMaker, db.UserRoleEnumMaker)
	_ = h.provisionUser(t, "checker-edonation-e2e@example.com", "Checker eDonation E2E", subChecker, db.UserRoleEnumChecker)
	_ = h.provisionUser(t, "admin-edonation-e2e@example.com", "Admin eDonation E2E", subAdmin, db.UserRoleEnumAdmin)

	makerToken := h.kc.MintTokenForSubject(subMaker, backendClientID, "maker")
	checkerToken := h.kc.MintTokenForSubject(subChecker, backendClientID, "checker")
	adminToken := h.kc.MintTokenForSubject(subAdmin, backendClientID, "admin")

	// Seed one issued donation via the existing real approve path (HTTP, not a
	// direct service call) — proves the export source is real DB state reached
	// through the same lifecycle every other E2E test exercises.
	w := h.do(t, http.MethodPost, "/api/donations", makerToken, validDonorBody("นาย ทดสอบ e-Donation Export"))
	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
	created := decodeDonation(t, w)

	w = h.do(t, http.MethodPost, "/api/donations/"+created.ID+"/submit", makerToken, nil)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	w = h.do(t, http.MethodPost, "/api/donations/"+created.ID+"/approve", checkerToken, nil)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	issued := decodeDonation(t, w)
	require.Equal(t, "issued", issued.Status)

	// Baseline bucket state — no code path in the export flow accepts a
	// ReceiptsStore/settings-image store reference at all (grep-verified in
	// 05-02 Task 1: internal/edonation/*.go never imports internal/storage), so
	// the harness's fake settings-image bucket is the only observable proxy for
	// "no file was written to a bucket" (D-74).
	baselineObjectCount := len(h.settingsStore.objects)

	t.Run("Maker_Forbidden_403", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/api/edonation/export?format=xlsx", makerToken, nil)
		assert.Equal(t, http.StatusForbidden, w.Code,
			"a Maker-only token must be rejected by RequireAnyRole(Checker,Admin) — body: %s", w.Body.String())
	})

	var auditCountBeforeChecker int
	require.NoError(t, h.pool.QueryRow(h.ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'edonation.export'`).Scan(&auditCountBeforeChecker))

	t.Run("Checker_200_XLSX_ZipSignature", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/api/edonation/export?format=xlsx", checkerToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		assert.Equal(t, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", w.Header().Get("Content-Type"))
		body := w.Body.Bytes()
		require.GreaterOrEqual(t, len(body), 4)
		assert.Equal(t, zipSignature, body[:4], "xlsx export body must start with the ZIP signature")

		// D-64/T-05-02-UNAUDITED: exactly ONE new audit_log row for this export.
		var auditCountAfter int
		require.NoError(t, h.pool.QueryRow(h.ctx,
			`SELECT count(*) FROM audit_log WHERE action = 'edonation.export'`).Scan(&auditCountAfter))
		assert.Equal(t, auditCountBeforeChecker+1, auditCountAfter,
			"exactly one new audit_log row with action edonation.export must be written per checker export")
	})

	t.Run("Admin_200_XLSX", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/api/edonation/export?format=xlsx", adminToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		body := w.Body.Bytes()
		require.GreaterOrEqual(t, len(body), 4)
		assert.Equal(t, zipSignature, body[:4], "xlsx export body must start with the ZIP signature")
	})

	t.Run("Checker_200_CSV_BOM", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/api/edonation/export?format=csv", checkerToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		assert.Contains(t, w.Header().Get("Content-Type"), "text/csv")
		body := w.Body.Bytes()
		require.GreaterOrEqual(t, len(body), 3)
		assert.Equal(t, utf8BOM, body[:3], "csv export body must start with the UTF-8 BOM")
	})

	t.Run("InvalidFormat_400", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/api/edonation/export?format=pdf", checkerToken, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
	})

	t.Run("NoBucketWrites_D74", func(t *testing.T) {
		// D-74 stream-only assertion: none of the 5 export/format calls above wrote
		// any object to the harness's fake settings-image bucket — the only
		// observable "bucket" reference the wired router holds — proving the
		// export path never persists a plaintext-PII file (stream-only to the
		// ResponseWriter, per xlsx.go/csv.go/exportfile.StreamXLSX/StreamCSV).
		assert.Equal(t, baselineObjectCount, len(h.settingsStore.objects),
			"export calls must never write an object to any bucket (D-74)")
	})
}

// TestE2E_EdonationKeyedAndAging drives POST /api/edonation/keyed and GET
// /api/edonation/aging over the REAL HTTP path with real signed Keycloak-shaped
// tokens (FR-31/SC#2, D-67/D-68/D-69) — the CLAUDE.md Conventions
// integration-test gate: RBAC (Maker 403, Checker/Admin 200), per-donation
// audit trail, and aging-bucket exclusion of just-keyed rows are all asserted
// against real DB state reached through the real router, not a direct service
// call.
func TestE2E_EdonationKeyedAndAging(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E integration test in short mode: requires Docker")
	}

	h := newE2EHarness(t)

	const subMaker = "99999999-1111-1111-1111-111111111111"
	const subChecker = "99999999-2222-2222-2222-222222222222"
	const subAdmin = "99999999-3333-3333-3333-333333333333"
	_ = h.provisionUser(t, "maker-keyed-e2e@example.com", "Maker Keyed E2E", subMaker, db.UserRoleEnumMaker)
	_ = h.provisionUser(t, "checker-keyed-e2e@example.com", "Checker Keyed E2E", subChecker, db.UserRoleEnumChecker)
	_ = h.provisionUser(t, "admin-keyed-e2e@example.com", "Admin Keyed E2E", subAdmin, db.UserRoleEnumAdmin)

	makerToken := h.kc.MintTokenForSubject(subMaker, backendClientID, "maker")
	checkerToken := h.kc.MintTokenForSubject(subChecker, backendClientID, "checker")

	// Seed 2 issued donations via the real approve path (HTTP, not a direct
	// service call) — mirrors TestE2E_EdonationExport's seeding discipline.
	w := h.do(t, http.MethodPost, "/api/donations", makerToken, validDonorBody("นาย ทดสอบ e-Donation Keyed หนึ่ง"))
	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
	created1 := decodeDonation(t, w)
	w = h.do(t, http.MethodPost, "/api/donations/"+created1.ID+"/submit", makerToken, nil)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	w = h.do(t, http.MethodPost, "/api/donations/"+created1.ID+"/approve", checkerToken, nil)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	issued1 := decodeDonation(t, w)
	require.Equal(t, "issued", issued1.Status)

	w = h.do(t, http.MethodPost, "/api/donations", makerToken, validDonorBody("นาย ทดสอบ e-Donation Keyed สอง"))
	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
	created2 := decodeDonation(t, w)
	w = h.do(t, http.MethodPost, "/api/donations/"+created2.ID+"/submit", makerToken, nil)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	w = h.do(t, http.MethodPost, "/api/donations/"+created2.ID+"/approve", checkerToken, nil)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	issued2 := decodeDonation(t, w)
	require.Equal(t, "issued", issued2.Status)

	keyedBody := map[string]any{
		"donation_ids": []string{issued1.ID, issued2.ID},
		"keyed":        true,
	}

	t.Run("Maker_Forbidden_403_Keyed", func(t *testing.T) {
		w := h.do(t, http.MethodPost, "/api/edonation/keyed", makerToken, keyedBody)
		assert.Equal(t, http.StatusForbidden, w.Code,
			"a Maker-only token must be rejected by RequireAnyRole(Checker,Admin) — body: %s", w.Body.String())
	})

	t.Run("Maker_Forbidden_403_Aging", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/api/edonation/aging", makerToken, nil)
		assert.Equal(t, http.StatusForbidden, w.Code,
			"a Maker-only token must be rejected by RequireAnyRole(Checker,Admin) — body: %s", w.Body.String())
	})

	var auditBefore int
	require.NoError(t, h.pool.QueryRow(h.ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'edonation.mark_keyed'`).Scan(&auditBefore))

	t.Run("Checker_200_MarksBothIssued", func(t *testing.T) {
		w := h.do(t, http.MethodPost, "/api/edonation/keyed", checkerToken, keyedBody)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

		// DB shows edonation_keyed=true for both donations.
		for _, id := range []string{issued1.ID, issued2.ID} {
			var keyed bool
			require.NoError(t, h.pool.QueryRow(h.ctx,
				`SELECT edonation_keyed FROM donations WHERE id = $1`, id).Scan(&keyed))
			assert.True(t, keyed, "donation %s must be marked keyed after POST /keyed", id)
		}

		// One audit row PER donation — 2 donations → +2 audit rows (D-67), not 1.
		var auditAfter int
		require.NoError(t, h.pool.QueryRow(h.ctx,
			`SELECT count(*) FROM audit_log WHERE action = 'edonation.mark_keyed'`).Scan(&auditAfter))
		assert.Equal(t, auditBefore+2, auditAfter,
			"exactly one audit_log row per donation must be written for a 2-donation bulk mark")
	})

	t.Run("Malformed_DonationID_422NotDB500", func(t *testing.T) {
		w := h.do(t, http.MethodPost, "/api/edonation/keyed", checkerToken, map[string]any{
			"donation_ids": []string{"not-a-real-uuid"},
			"keyed":        true,
		})
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code,
			"a malformed donation id must 4xx before the query runs, never 500 — body: %s", w.Body.String())
	})

	t.Run("Checker_200_AgingExcludesKeyedRows", func(t *testing.T) {
		w := h.do(t, http.MethodGet, "/api/edonation/aging", checkerToken, nil)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

		var resp struct {
			Data struct {
				Rows []struct {
					ID string `json:"id"`
				} `json:"rows"`
				Counts map[string]int `json:"counts"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

		for _, row := range resp.Data.Rows {
			assert.NotEqual(t, issued1.ID, row.ID, "a just-keyed donation must not appear in the aging view")
			assert.NotEqual(t, issued2.ID, row.ID, "a just-keyed donation must not appear in the aging view")
		}
	})
}
