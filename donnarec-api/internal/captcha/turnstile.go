package captcha

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// turnstileSiteverifyURL is Cloudflare's official server-side verification
// endpoint (developers.cloudflare.com/turnstile/get-started/server-side-validation/).
const turnstileSiteverifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// turnstileDefaultTimeout bounds how long a single siteverify call may hang.
// Without a timeout, a stalled/black-holed connection to Cloudflare could hang
// the calling request indefinitely — an availability risk on top of the
// deliberate fail-closed tradeoff (Pitfall 5).
const turnstileDefaultTimeout = 5 * time.Second

// TurnstileVerifier implements Verifier against Cloudflare Turnstile's
// server-side siteverify endpoint. secretKey is env-sourced only
// (TURNSTILE_SECRET_KEY, config.go) and is never DB-stored (T-06-08).
type TurnstileVerifier struct {
	secretKey string
	client    *http.Client
	verifyURL string
}

// NewTurnstileVerifier constructs a TurnstileVerifier for production use,
// pointed at Cloudflare's real siteverify endpoint with a bounded-timeout
// HTTP client.
func NewTurnstileVerifier(secretKey string) *TurnstileVerifier {
	return &TurnstileVerifier{
		secretKey: secretKey,
		client:    &http.Client{Timeout: turnstileDefaultTimeout},
		verifyURL: turnstileSiteverifyURL,
	}
}

// NewTurnstileVerifierWithClient constructs a TurnstileVerifier against a
// caller-supplied siteverify URL and HTTP client. This is the test seam
// (turnstile_test.go points verifyURL at a fake httptest.Server standing in
// for Cloudflare) and is also available to production wiring if a non-default
// client (custom timeout/transport) is ever needed.
func NewTurnstileVerifierWithClient(secretKey, verifyURL string, client *http.Client) *TurnstileVerifier {
	return &TurnstileVerifier{
		secretKey: secretKey,
		client:    client,
		verifyURL: verifyURL,
	}
}

// Verify implements Verifier. Fail-closed on every error path (Pitfall 5):
// empty token, request-build error, transport/timeout error, response-decode
// error, and provider success=false all return a non-nil error wrapping
// ErrCaptchaFailed. Only a genuine {"success":true} response returns nil.
func (v *TurnstileVerifier) Verify(ctx context.Context, token, remoteIP string) error {
	if token == "" {
		return fmt.Errorf("%w: empty token", ErrCaptchaFailed)
	}

	form := url.Values{
		"secret":   {v.secretKey},
		"response": {token},
		"remoteip": {remoteIP},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.verifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("%w: build siteverify request: %v", ErrCaptchaFailed, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.client.Do(req)
	if err != nil {
		// Network/timeout error — fail-closed, never nil (Pitfall 5).
		return fmt.Errorf("%w: siteverify request: %v", ErrCaptchaFailed, err)
	}
	defer resp.Body.Close()

	var result struct {
		Success    bool     `json:"success"`
		ErrorCodes []string `json:"error-codes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("%w: decode siteverify response: %v", ErrCaptchaFailed, err)
	}
	if !result.Success {
		return fmt.Errorf("%w: provider rejected token (codes=%v)", ErrCaptchaFailed, result.ErrorCodes)
	}
	return nil
}
