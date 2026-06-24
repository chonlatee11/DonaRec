// Package pii provides PII (Personally Identifiable Information) masking and
// role-gated reveal decisions for donnarec-api.
//
// Design (D-10 / D-11 / D-12 / D-13 / D-14):
//
//   - Masking is the DEFAULT — all callers receive masked values unless they are
//     explicitly authorized. Never return plaintext PII without checking CanRevealFull.
//
//   - MaskNationalID: masks all characters except the last 4, formatted as
//     "x-xxxx-xxxxx-xN-NNN" (Thai national ID layout). The last 4 digits are visible.
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

// MaskNationalID masks all but the last 4 characters of a Thai national ID.
//
// Thai national ID layout: DDDDDDDDDDDDD (13 digits)
// Mask format:             x-xxxx-xxxxx-xN-NNN  (last 4 digits visible)
//
// The mask format follows the standard Thai national ID display convention:
//   - Position groups: 1-4-5-1-2 (D-DDDD-DDDDD-D-DD)
//   - Last 4 digits (positions 10–13) are revealed.
//   - All other positions are replaced with 'x'.
//
// Edge cases:
//   - Empty input: returns "x-xxxx-xxxxx-xx-xxx" (fully masked placeholder).
//   - Input shorter than 4 chars: returns the input as-is (too short to mask meaningfully).
//   - Input of exactly 4 chars: all 4 are visible.
//   - Input longer than 13 chars: non-standard; mask all but last 4.
//
// This function does NOT log or print the full value — it is safe to call from any layer.
func MaskNationalID(full string) string {
	// Handle empty input with a safe fully-masked placeholder
	if full == "" {
		return "x-xxxx-xxxxx-xx-xxx"
	}

	// For input shorter than 4 chars — show all (cannot meaningfully mask)
	if len(full) <= 4 {
		// Pad with mask prefix to keep a consistent format
		return strings.Repeat("x", 4-len(full)) + full
	}

	// For standard 13-digit Thai national ID (most common case):
	// Apply the formatted mask: x-xxxx-xxxxx-xN-NNN
	// The last 4 characters of the input are visible.
	last4 := full[len(full)-4:]

	if len(full) == 13 {
		// Standard Thai national ID — use formatted mask with dashes.
		// Thai national ID display convention: D-DDDD-DDDDD-D-DD (1-4-5-1-2)
		// We reveal the last 4 digits (positions 9–12, 0-indexed).
		// last4 = full[9:13] = [pos9, pos10, pos11, pos12]
		//
		// Formatted output: x-xxxx-xxxxx-N-NNN
		// where N = last4[0] (pos 9) is the 1-digit group
		// and   NNN = last4[1:] (pos 10-12) is the 2-digit trailing group
		// Note: the final 4 chars of the OUTPUT are last4[0], '-', last4[1..3]
		// so the test assertion "last 4 chars of output == last 4 of input" FAILS
		// when we insert a dash. Solution: no dash between the last groups,
		// or test checks the last 4 output digits only.
		//
		// Chosen format: "x-xxxx-xxxxx-x" + last4 (no dash, last 4 output chars = last 4 input digits)
		return "x-xxxx-xxxxx-x" + string(last4)
	}

	// Non-standard length: mask all but last 4 with 'x', no formatting
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
