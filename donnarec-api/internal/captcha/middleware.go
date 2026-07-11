package captcha

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// TokenField is the multipart/form field name the public donation form
// submits the Turnstile widget token under. Agreed with plan 03's public
// handler and plan 06's TurnstileWidget.tsx frontend component.
const TokenField = "turnstile_token"

// Middleware wraps a Verifier and exposes it as gin.HandlerFunc factories.
// Constructor-injected (no global state, Pattern B) — mirrors DonationService's
// keyProvider-field injection style (service.go).
type Middleware struct {
	verifier Verifier
}

// NewMiddleware constructs a captcha Middleware around the given Verifier.
func NewMiddleware(v Verifier) *Middleware {
	return &Middleware{verifier: v}
}

// VerifyTurnstile returns a gin.HandlerFunc that reads the Turnstile token
// from the multipart form field TokenField, verifies it via the wrapped
// Verifier using c.ClientIP() as the remote IP, and aborts the request on
// failure.
//
// On failure it returns 400 with a distinct CAPTCHA-specific error shape
// {"error": "captcha_failed"} — deliberately NOT the 422 field-validation
// shape used elsewhere in this codebase, so the frontend can key off a
// distinct message (Pitfall 3, 06-RESEARCH.md). The raw token is read only
// here and never forwarded into any domain request struct.
func (m *Middleware) VerifyTurnstile() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.PostForm(TokenField)
		if err := m.verifier.Verify(c.Request.Context(), token, c.ClientIP()); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "captcha_failed",
			})
			return
		}
		c.Next()
	}
}
