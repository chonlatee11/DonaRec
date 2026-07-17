// Package main — e2e_public_test.go
//
// End-to-end HTTP integration test for the Flow B unauthenticated public
// submission seam (plan 06-03), satisfying the CONVENTIONS.md integration-test
// gate. This phase adds the codebase's FIRST unauthenticated route group —
// exactly the class the gate exists for — so it MUST be proven over the REAL
// HTTP path:
//
//	multipart POST /api/public/donations
//	  → ratelimit.PerIP → captcha.VerifyTurnstile (FAKE verifier, nil = pass)
//	  → PublicDonationHandler.CreatePublic → storage.PutSlip (real magic-byte
//	    validation, faked PUT) → DonationService.CreatePublicSubmission → DB
//
// The router is the SAME one main() ships (production setupRouter, via
// newE2EHarness). Only the CAPTCHA verdict and the MinIO network PUT are stubbed;
// the rate-limit middleware, magic-byte validation, atomic tx, audit, and outbox
// enqueue all run for real against a Postgres testcontainer.
//
// Requires Docker testcontainers. Skip with -short. Run under -race.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donnarec/donnarec-api/internal/captcha"
	"github.com/donnarec/donnarec-api/internal/donation"
)

// doPublicSubmission drives a multipart/form-data POST to /api/public/donations
// with the given donor fields and (optionally) a slip file. remoteAddr, when
// non-empty, sets req.RemoteAddr so gin's c.ClientIP() resolves a distinct IP —
// used to isolate the rate-limit subtest's bucket from the other subtests.
func (h *e2eHarness) doPublicSubmission(
	t *testing.T,
	fields map[string]string,
	slipFilename string,
	slipBytes []byte,
	remoteAddr string,
) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		require.NoError(t, mw.WriteField(k, v))
	}
	// The CAPTCHA token field is always present (the fake verifier ignores its
	// value, but the middleware reads the field for real).
	require.NoError(t, mw.WriteField(captcha.TokenField, "fake-turnstile-token"))
	if slipBytes != nil {
		part, err := mw.CreateFormFile(donation.PublicSlipField, slipFilename)
		require.NoError(t, err)
		_, err = part.Write(slipBytes)
		require.NoError(t, err)
	}
	require.NoError(t, mw.Close())

	req, err := http.NewRequest(http.MethodPost, "/api/public/donations", &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	return w
}

// doPublicSubmissionXFF is a variant of doPublicSubmission that also sets a
// caller-supplied X-Forwarded-For header — used by the SEC-06-FLAG2 spoofed-IP
// regression test below. Kept as a separate helper (rather than extending
// doPublicSubmission's signature) so every existing caller stays untouched.
// Slip is deliberately never attached (mirrors doPublicSubmission(..., "",
// nil, ...) callers) — a 400 slip_required still consumes a rate token since
// ratelimit.PerIP runs before the handler.
func (h *e2eHarness) doPublicSubmissionXFF(
	t *testing.T,
	fields map[string]string,
	remoteAddr string,
	xForwardedFor string,
) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		require.NoError(t, mw.WriteField(k, v))
	}
	require.NoError(t, mw.WriteField(captcha.TokenField, "fake-turnstile-token"))
	require.NoError(t, mw.Close())

	req, err := http.NewRequest(http.MethodPost, "/api/public/donations", &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	if xForwardedFor != "" {
		req.Header.Set("X-Forwarded-For", xForwardedFor)
	}
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	return w
}

// oversizedSlipBytes returns a PNG-magic-byte-prefixed payload sized well
// past main.go's publicBodyLimitBytes (11 MB) cap — used by the SEC-06-FLAG1
// oversized-body regression test below.
func oversizedSlipBytes() []byte {
	b := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	return append(b, make([]byte, 12<<20)...) // 12 MB > 11 MB cap
}

// validPublicFields returns a complete, valid public donor submission field set.
func validPublicFields(donorName string) map[string]string {
	return map[string]string{
		"donor_name":           donorName,
		"donor_tax_id":         "1234567890123",
		"donor_address":        "123 ถนนสาธารณะ กรุงเทพฯ",
		"donor_email":          "donor-e2e@example.com",
		"amount":               "2500.00",
		"donated_at":           "2026-03-15",
		"consent_given":        "true",
		"consent_text_version": "public-form-v1",
		"consent_purpose":      "tax-receipt",
		"donor_language":       "th",
	}
}

// TestPublicDonationE2E proves the unauthenticated Flow B seam end-to-end over the
// real router chain (Conventions integration-test gate).
//
// Requires Docker testcontainers. Skip with -short. Run under -race.
func TestPublicDonationE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E integration test in short mode: requires Docker")
	}

	h := newE2EHarness(t)

	t.Run("HappyPath_AtomicPendingReviewFlowB", func(t *testing.T) {
		const donorName = "นาย ทดสอบ Public E2E"
		w := h.doPublicSubmission(t, validPublicFields(donorName), "slip.png", settingsPNGBytes(), "")
		require.Equal(t, http.StatusCreated, w.Code, "public submission body: %s", w.Body.String())

		// --- Response carries a reference number (D-84), not a receipt number ---
		var env struct {
			Data struct {
				ReferenceNumber string `json:"reference_number"`
				Status          string `json:"status"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "body: %s", w.Body.String())
		assert.NotEmpty(t, env.Data.ReferenceNumber, "response must carry a reference number")
		assert.True(t, strings.HasPrefix(env.Data.ReferenceNumber, "REF-"),
			"reference number must be the REF- code, not a receipt number; got %q", env.Data.ReferenceNumber)
		assert.Equal(t, "pending_review", env.Data.Status)

		// --- The donation row: pending_review, source=flow_b, created_by=public-web ---
		var (
			donationID string
			status     string
			source     string
			createdBy  string
		)
		require.NoError(t, h.pool.QueryRow(h.ctx,
			`SELECT id::text, status, source, created_by::text FROM donations WHERE donor_name = $1`, donorName,
		).Scan(&donationID, &status, &source, &createdBy))
		assert.Equal(t, "pending_review", status)
		assert.Equal(t, "flow_b", source, "public submissions must be source=flow_b (D-77)")
		assert.Equal(t, donation.PublicWebUserID, createdBy, "created_by must be the seeded public-web user (D-76)")

		// --- Exactly one slip_attachments row ---
		var slipCount int
		require.NoError(t, h.pool.QueryRow(h.ctx,
			`SELECT count(*) FROM slip_attachments WHERE donation_id = $1 AND deleted_at IS NULL`, donationID,
		).Scan(&slipCount))
		assert.Equal(t, 1, slipCount, "exactly one slip row (D-80 mandatory slip)")

		// --- Exactly one ack_email outbox job carrying the donation id; NO issue_receipt ---
		var ackCount int
		require.NoError(t, h.pool.QueryRow(h.ctx,
			`SELECT count(*) FROM outbox_jobs WHERE job_type = 'ack_email' AND payload->>'donation_id' = $1`, donationID,
		).Scan(&ackCount))
		assert.Equal(t, 1, ackCount, "exactly one ack_email job (D-85)")

		var issueCount int
		require.NoError(t, h.pool.QueryRow(h.ctx,
			`SELECT count(*) FROM outbox_jobs WHERE job_type = 'issue_receipt' AND payload->>'donation_id' = $1`, donationID,
		).Scan(&issueCount))
		assert.Equal(t, 0, issueCount, "no issue_receipt job at submit (receipts only at approval, D-84)")

		// --- Exactly one in-tx audit row, actor = the public-web UUID (Pitfall 1) ---
		var auditCount int
		require.NoError(t, h.pool.QueryRow(h.ctx,
			`SELECT count(*) FROM audit_log WHERE actor_id = $1 AND action = 'donation.public_submit'`,
			donation.PublicWebUserID,
		).Scan(&auditCount))
		assert.Equal(t, 1, auditCount, "one audit row under the public-web UUID actor")
	})

	t.Run("BadMagicByteSlip_Rejected_NoRow", func(t *testing.T) {
		var before int
		require.NoError(t, h.pool.QueryRow(h.ctx, `SELECT count(*) FROM donations`).Scan(&before))

		const donorName = "นาย ทดสอบ ไฟล์ปลอม"
		// A text file renamed .jpg — magic-byte detection (not the extension) rejects it.
		badSlip := append([]byte("this is definitely not an image file, just plain text"), make([]byte, 512)...)
		w := h.doPublicSubmission(t, validPublicFields(donorName), "notreally.jpg", badSlip, "")
		require.Equal(t, http.StatusUnsupportedMediaType, w.Code,
			"a spoofed-extension text file must be rejected by magic-byte validation; body: %s", w.Body.String())
		assert.Contains(t, w.Body.String(), "unsupported_file_type")

		// No donation row was created — the slip is validated BEFORE any DB write (Pitfall 4).
		var after int
		require.NoError(t, h.pool.QueryRow(h.ctx, `SELECT count(*) FROM donations`).Scan(&after))
		assert.Equal(t, before, after, "a rejected slip must create NO donation row")

		var orphan int
		require.NoError(t, h.pool.QueryRow(h.ctx,
			`SELECT count(*) FROM donations WHERE donor_name = $1`, donorName).Scan(&orphan))
		assert.Equal(t, 0, orphan, "no donation row for the rejected submission")
	})

	t.Run("MissingSlip_Rejected_NoRow", func(t *testing.T) {
		var before int
		require.NoError(t, h.pool.QueryRow(h.ctx, `SELECT count(*) FROM donations`).Scan(&before))

		const donorName = "นาย ทดสอบ ไม่มีสลิป"
		w := h.doPublicSubmission(t, validPublicFields(donorName), "", nil, "")
		require.Equal(t, http.StatusBadRequest, w.Code,
			"a missing slip must be rejected (D-80 mandatory); body: %s", w.Body.String())
		assert.Contains(t, w.Body.String(), "slip_required")

		var after int
		require.NoError(t, h.pool.QueryRow(h.ctx, `SELECT count(*) FROM donations`).Scan(&after))
		assert.Equal(t, before, after, "a missing-slip submission must create NO donation row")
	})

	t.Run("RateLimit_ExceedingThreshold_429", func(t *testing.T) {
		// A dedicated client IP gets its own fresh token bucket (burst =
		// e2ePublicRateBurst, near-zero refill). Send exactly `burst` requests to
		// exhaust it (each 400 slip_required, but still consuming a rate token since
		// PerIP runs BEFORE the handler), then one more must be 429 rate_limited.
		const ip = "203.0.113.7:9999"
		fields := validPublicFields("นาย ทดสอบ Rate Limit")
		for i := 0; i < e2ePublicRateBurst; i++ {
			w := h.doPublicSubmission(t, fields, "", nil, ip)
			require.NotEqual(t, http.StatusTooManyRequests, w.Code,
				"request %d within the burst must pass the rate limiter; body: %s", i+1, w.Body.String())
		}
		w := h.doPublicSubmission(t, fields, "", nil, ip)
		require.Equal(t, http.StatusTooManyRequests, w.Code,
			"the request past the per-IP burst must be 429; body: %s", w.Body.String())
		assert.Contains(t, w.Body.String(), "rate_limited")
	})

	t.Run("SpoofedXFF_SharesRemoteAddrBucket_NotBypassed", func(t *testing.T) {
		// SEC-06-FLAG2 regression: router.SetTrustedProxies(nil) (main.go)
		// must make gin's c.ClientIP() IGNORE an attacker-supplied
		// X-Forwarded-For on a direct connection and always return
		// RemoteAddr. A dedicated RemoteAddr gets its own fresh token
		// bucket (burst = e2ePublicRateBurst). Send burst requests that
		// share ONE RemoteAddr but each carry a DIFFERENT spoofed XFF value
		// — before the fix, gin's default trust-all-proxies config would
		// have honored each spoofed XFF as the "real" client IP, handing
		// every request a fresh, never-throttled bucket and none of them
		// would ever hit 429.
		const ip = "198.51.100.42:9999"
		fields := validPublicFields("นาย ทดสอบ Spoof XFF")
		for i := 0; i < e2ePublicRateBurst; i++ {
			spoofedXFF := fmt.Sprintf("10.0.0.%d", i+1)
			w := h.doPublicSubmissionXFF(t, fields, ip, spoofedXFF)
			require.NotEqual(t, http.StatusTooManyRequests, w.Code,
				"request %d (spoofed XFF=%s, shared RemoteAddr) within the burst must pass; body: %s",
				i+1, spoofedXFF, w.Body.String())
		}
		// One more request, SAME RemoteAddr, yet another distinct spoofed
		// XFF — must be 429: proves the spoofed header did NOT create a
		// fresh per-header bucket and RemoteAddr governs.
		w := h.doPublicSubmissionXFF(t, fields, ip, "10.0.0.99")
		require.Equal(t, http.StatusTooManyRequests, w.Code,
			"the request past the burst must be 429 despite a fresh spoofed XFF value — RemoteAddr must govern the rate-limit bucket; body: %s",
			w.Body.String())
		assert.Contains(t, w.Body.String(), "rate_limited")
	})

	t.Run("OversizedBody_Rejected_NoRow_NoHang", func(t *testing.T) {
		// SEC-06-FLAG1 regression: main.go's bodyLimitMiddleware wraps the
		// request body in http.MaxBytesReader(11<<20) BEFORE
		// captchaMW.VerifyTurnstile()'s own c.PostForm call would otherwise
		// force gin to fully parse (and, for >32MB parts, disk-spill) the
		// entire oversized multipart body during its own token read — no
		// valid CAPTCHA solve required to trigger that spill pre-fix.
		//
		// PRODUCTION divergence (confirmed, not a test gap — see 260717-spx
		// plan context): in production, an oversized body ->
		// ParseMultipartForm fails inside VerifyTurnstile -> c.PostForm
		// returns "" -> the REAL Turnstile verifier rejects the empty token
		// -> 400 {"error":"captcha_failed"}, UNCHANGED shape. This E2E
		// harness wires fakeCaptchaVerifier (e2e_test.go), whose Verify()
		// ALWAYS returns nil regardless of token value, so in-harness the
		// (now-failed) parse still lets the fake verifier PASS and the
		// request reaches the handler, which then rejects on the
		// missing/unreadable slip. This subtest therefore asserts the DoS
		// property that holds under BOTH paths — a bounded 4xx rejection,
		// no donation row, and no hang — NOT the specific captcha_failed
		// error shape (which only production's real verifier produces).
		var before int
		require.NoError(t, h.pool.QueryRow(h.ctx, `SELECT count(*) FROM donations`).Scan(&before))

		const ip = "203.0.113.55:9999" // fresh RemoteAddr, isolated from other subtests' buckets
		const donorName = "นาย ทดสอบ Oversized Body"

		start := time.Now()
		w := h.doPublicSubmission(t, validPublicFields(donorName), "oversized.png", oversizedSlipBytes(), ip)
		elapsed := time.Since(start)

		assert.Less(t, elapsed, 10*time.Second,
			"an oversized body must be rejected promptly by the body-limit middleware, not hang; took %s", elapsed)
		assert.GreaterOrEqual(t, w.Code, http.StatusBadRequest,
			"an oversized body must be rejected with a 4xx status (bounded rejection); body: %s", w.Body.String())
		assert.Less(t, w.Code, http.StatusInternalServerError,
			"an oversized body must not 5xx; body: %s", w.Body.String())

		var after int
		require.NoError(t, h.pool.QueryRow(h.ctx, `SELECT count(*) FROM donations`).Scan(&after))
		assert.Equal(t, before, after, "an oversized body must create NO donation row")

		var orphan int
		require.NoError(t, h.pool.QueryRow(h.ctx,
			`SELECT count(*) FROM donations WHERE donor_name = $1`, donorName).Scan(&orphan))
		assert.Equal(t, 0, orphan, "no donation row for the oversized-body submission")
	})
}
