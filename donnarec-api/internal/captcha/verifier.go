// Package captcha defines the swappable server-side CAPTCHA verification seam
// (D-82, FR-04) that guards the public donation submission endpoint (the first
// unauthenticated HTTP seam in this codebase — Phase 6, D-78).
//
// Design decisions realized here:
//
//	D-82: Verifier is a narrow interface, not a concrete provider — mirrors the
//	      established mailer.EmailSender / worker.PDFRenderer "define the seam
//	      here" convention (06-PATTERNS.md). Only implementation shipped this
//	      phase is TurnstileVerifier (Cloudflare Turnstile).
//	T-06-06: server-side siteverify is the ONLY authoritative signal — a client-
//	      asserted "captcha success" flag is never trusted (Anti-Pattern,
//	      06-RESEARCH.md).
//	Pitfall 5 (06-RESEARCH.md): every failure path is fail-closed. Verify never
//	      returns nil on a network/timeout/decode error — an unreachable or
//	      misbehaving CAPTCHA provider rejects the submission rather than
//	      silently letting it through.
package captcha

import (
	"context"
	"errors"
)

// ErrCaptchaFailed is the sentinel returned (directly or wrapped via %w) by
// every Verify failure path: empty token, provider success=false, or a
// transport/decode error talking to the provider. Callers should check with
// errors.Is(err, ErrCaptchaFailed).
var ErrCaptchaFailed = errors.New("captcha: verification failed")

// Verifier is the narrow interface for server-side CAPTCHA token verification.
// Constructor-injected into the middleware — no global state (Pattern B,
// service.go convention).
type Verifier interface {
	// Verify checks token against the CAPTCHA provider, forwarding remoteIP
	// as additional signal. Returns nil ONLY on a genuine provider-confirmed
	// success; every other outcome (empty token, provider rejection,
	// network/timeout/decode error) returns a non-nil error wrapping
	// ErrCaptchaFailed (fail-closed — Pitfall 5).
	Verify(ctx context.Context, token, remoteIP string) error
}
