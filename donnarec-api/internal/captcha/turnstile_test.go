// internal/captcha/turnstile_test.go — RED tests for TurnstileVerifier against a
// fake HTTP server standing in for challenges.cloudflare.com/turnstile/v0/siteverify
// (06-PLAN-02 <feature> behavior spec). Black-box test package, mirrors the
// codebase's established `_test` convention (auth/rbac_test.go, mailer/dev_sender_test.go).
package captcha_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donnarec/donnarec-api/internal/captcha"
)

// TestTurnstileVerifier_Verify covers the four fail-closed behaviors specified
// in the plan: empty token (no network call), success=true, success=false, and
// a network/timeout error — every non-success path must return a non-nil
// error wrapping captcha.ErrCaptchaFailed (Pitfall 5 — never nil on failure).
func TestTurnstileVerifier_Verify(t *testing.T) {
	t.Run("empty token fails without any network call", func(t *testing.T) {
		called := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		}))
		defer srv.Close()

		v := captcha.NewTurnstileVerifierWithClient("test-secret", srv.URL, srv.Client())
		err := v.Verify(context.Background(), "", "1.2.3.4")

		require.Error(t, err)
		require.ErrorIs(t, err, captcha.ErrCaptchaFailed)
		assert.False(t, called, "siteverify must not be called for an empty token")
	})

	t.Run("server responds success:true returns nil", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "test-secret", r.FormValue("secret"))
			assert.Equal(t, "good-token", r.FormValue("response"))
			assert.Equal(t, "1.2.3.4", r.FormValue("remoteip"))
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		}))
		defer srv.Close()

		v := captcha.NewTurnstileVerifierWithClient("test-secret", srv.URL, srv.Client())
		err := v.Verify(context.Background(), "good-token", "1.2.3.4")

		assert.NoError(t, err)
	})

	t.Run("server responds success:false fails", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success":     false,
				"error-codes": []string{"invalid-input-response"},
			})
		}))
		defer srv.Close()

		v := captcha.NewTurnstileVerifierWithClient("test-secret", srv.URL, srv.Client())
		err := v.Verify(context.Background(), "bad-token", "1.2.3.4")

		require.Error(t, err)
		require.ErrorIs(t, err, captcha.ErrCaptchaFailed)
	})

	t.Run("network timeout is fail-closed, never nil", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(50 * time.Millisecond)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		}))
		defer srv.Close()

		client := &http.Client{Timeout: 5 * time.Millisecond}
		v := captcha.NewTurnstileVerifierWithClient("test-secret", srv.URL, client)
		err := v.Verify(context.Background(), "token", "1.2.3.4")

		require.Error(t, err)
		require.ErrorIs(t, err, captcha.ErrCaptchaFailed)
	})

	t.Run("closed server (transport error) is fail-closed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		srv.Close() // closed before any request reaches it

		v := captcha.NewTurnstileVerifierWithClient("test-secret", srv.URL, srv.Client())
		err := v.Verify(context.Background(), "token", "1.2.3.4")

		require.Error(t, err)
		require.ErrorIs(t, err, captcha.ErrCaptchaFailed)
	})
}
