// Package pii provides PII (Personally Identifiable Information) masking and
// role-gated reveal decisions for donnarec-api.
//
// Design (D-10 / D-11 / D-12 / D-13 / D-14):
//
//   - Masking is the DEFAULT — all callers receive masked values unless they are
//     explicitly authorized. Never return plaintext PII without checking CanRevealFull.
//
//   - MaskNationalID: for a plausible-length ID (>= 10 chars) masks all characters
//     except the last 4, formatted as "x-xxxx-xxxxx-xN-NNN" for the standard 13-digit
//     layout. Values shorter than 10 chars are fully masked (WR-04) so a short/partial
//     value never leaks a disproportionate fraction of its digits.
//
//   - CanRevealFull: role gate — Admin and Checker may see full PII (D-10).
//     Maker-role users receive only the masked value. Just-in-time reveal means
//     this function is called per-request, not cached per-session.
//
// PHASE BOUNDARY (D-14):
//
//	This package provides the mechanism only. Actual donor PII (national ID, tax ID)
//	is NOT stored in this phase. Phase 3 wires EncryptField / DecryptField from
//	internal/crypto onto donor records and uses MaskNationalID / CanRevealFull here.
//
// AUDIT REQUIREMENT (D-13):
//
//	Every call that reveals full PII MUST be audited via the 01-02 PII-reveal audit
//	path. The reveal endpoint (Phase 3) must:
//	  1. Call CanRevealFull(claims) → return 403 if false.
//	  2. Call crypto.DecryptField to get plaintext.
//	  3. Write an audit entry (action="pii.reveal") via the audit middleware/service.
//	  4. Return the plaintext to the authorized caller.
//	This package does NOT perform auditing itself — it is the gate + mask mechanism.
package pii

import (
	"strings"

	"github.com/donnarec/donnarec-api/internal/auth"
)

// maskChar is the character used to replace masked PII digits.
const maskChar = 'x'

// minRevealLen is the minimum input length for which the last-4 reveal rule
// applies. A plausible national/tax ID is at least 10 digits; anything shorter is
// fully masked so a short/partial value does not leak a disproportionate fraction
// of its digits (WR-04).
const minRevealLen = 10

// MaskNationalID masks all but the last 4 characters of a Thai national ID.
//
// Format spec:
//   - len == 13 (standard Thai national ID): returns "x-xxxx-xxxxx-x" + last4,
//     where the final 4 output characters equal the last 4 input digits.
//   - len >= minRevealLen (10..) and != 13: returns ("x" * (len-4)) + last4.
//   - len < minRevealLen: fully masked ("x" * len) — too short to reveal safely.
//   - empty input: returns the fully-masked 13-digit placeholder "x-xxxx-xxxxx-xx-xxx".
//
// This function does NOT log or print the full value — it is safe to call from any layer.
func MaskNationalID(full string) string {
	// Handle empty input with a safe fully-masked placeholder
	if full == "" {
		return "x-xxxx-xxxxx-xx-xxx"
	}

	// Too short to reveal last-4 without leaking a large fraction of the value —
	// mask everything (WR-04). Covers partial/malformed entries (len 1..9).
	if len(full) < minRevealLen {
		return strings.Repeat(string(maskChar), len(full))
	}

	last4 := full[len(full)-4:]

	if len(full) == 13 {
		// Standard Thai national ID. Display convention groups are 1-4-5-1-2;
		// we reveal only the last 4 digits. The chosen output keeps the final 4
		// output characters identical to the last 4 input digits (no dash before
		// them), so callers/tests can compare them directly.
		return "x-xxxx-xxxxx-x" + last4
	}

	// Non-standard length (>= minRevealLen): mask all but last 4, no formatting.
	masked := strings.Repeat(string(maskChar), len(full)-4)
	return masked + last4
}

// CanRevealFull returns true if the authenticated user is authorized to see the
// full (unmasked) national ID or other sensitive PII field.
//
// Authorization policy (D-10):
//   - RoleAdmin:   YES — administrators see full PII
//   - RoleChecker: YES — checkers need full PII to approve donation records
//   - RoleMaker:   NO  — makers see only masked value
//   - Any other:   NO
//
// Just-in-time reveal (D-12): this function is called per-request, never cached.
// A user whose role changes immediately loses (or gains) reveal access on the next request.
//
// IMPORTANT: Returning true from this function does NOT automatically audit the reveal.
// The caller MUST write an audit entry (action="pii.reveal") via the 01-02 audit path
// before returning plaintext PII to the client (D-13).
//
// Phase scope: this mechanism is defined in Phase 1 and wired to donor data in Phase 3.
func CanRevealFull(claims auth.KeycloakClaims) bool {
	// Admin can see all PII (D-10)
	if claims.HasRole(auth.RoleAdmin) {
		return true
	}
	// Checker can see PII to perform approval (D-10)
	if claims.HasRole(auth.RoleChecker) {
		return true
	}
	// Maker and all other roles: masked only
	return false
}
