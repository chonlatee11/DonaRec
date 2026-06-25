// Package pii_test tests PII masking and role-gated reveal decision logic.
package pii_test

import (
	"testing"

	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/pii"
	"github.com/stretchr/testify/assert"
)

// TestMaskNationalID validates that MaskNationalID reveals exactly the last 4 characters
// and masks all preceding characters (D-11).
func TestMaskNationalID(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLast4 string
		wantLen   int
	}{
		{
			name:      "standard 13-digit Thai national ID",
			input:     "1234567890123",
			wantLast4: "0123",
			wantLen:   len("x-xxxx-xxxxx-x0-123"), // matches the mask format
		},
		{
			name:      "13-digit ID ending in 0000",
			input:     "1234567890000",
			wantLast4: "0000",
		},
		{
			name:      "13-digit ID all same digit",
			input:     "1111111111111",
			wantLast4: "1111",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pii.MaskNationalID(tc.input)

			// Must not be empty
			assert.NotEmpty(t, got)

			// Last 4 characters of masked output must be the last 4 of input
			if len(got) >= 4 {
				assert.Equal(t, tc.wantLast4, got[len(got)-4:],
					"last 4 chars of masked output must match last 4 of input")
			}

			// The full input must not appear verbatim in the masked output
			assert.NotEqual(t, tc.input, got, "masked output must differ from input")

			// Check the first characters are masked (not the original digits)
			// The first char of input is '1' — it must be masked in output
			if len(tc.input) > 4 && len(got) > 0 {
				assert.NotEqual(t, string(tc.input[0]), string(got[0]),
					"first character must be masked (not a visible digit)")
			}
		})
	}

	t.Run("empty input handled safely", func(t *testing.T) {
		got := pii.MaskNationalID("")
		// Must not panic; result should be safe (empty or all-mask)
		assert.NotEqual(t, "", got, "empty input should return a safe placeholder")
	})

	t.Run("short input (< 10 chars) is fully masked (WR-04)", func(t *testing.T) {
		// Values shorter than a plausible ID must not leak their last 4 chars —
		// they are fully masked so no original digit is exposed.
		got := pii.MaskNationalID("123")
		assert.Equal(t, "xxx", got, "short value must be fully masked")
		assert.NotContains(t, got, "1", "no original digit may appear")
	})

	t.Run("exactly 4 chars — fully masked, not revealed (WR-04)", func(t *testing.T) {
		// Revealing all 4 chars of a 4-char value would leak 100% of it; the
		// last-4 rule only applies once the value is at least minRevealLen long.
		got := pii.MaskNationalID("1234")
		assert.Equal(t, "xxxx", got, "4-char value must be fully masked, not revealed")
	})

	t.Run("10-char value reveals last 4 (WR-04 boundary)", func(t *testing.T) {
		got := pii.MaskNationalID("0123456789")
		assert.Equal(t, "xxxxxx6789", got, "at minRevealLen, mask all but last 4")
	})
}

// TestCanRevealFull validates role-gated reveal decision (D-10):
// Admin → true, Checker → true, Maker-only → false.
func TestCanRevealFull(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
		want  bool
	}{
		{
			name:  "admin can reveal (D-10)",
			roles: []string{auth.RoleAdmin},
			want:  true,
		},
		{
			name:  "checker can reveal (D-10)",
			roles: []string{auth.RoleChecker},
			want:  true,
		},
		{
			name:  "maker cannot reveal (D-10)",
			roles: []string{auth.RoleMaker},
			want:  false,
		},
		{
			name:  "maker+checker can reveal (multi-role D-02)",
			roles: []string{auth.RoleMaker, auth.RoleChecker},
			want:  true,
		},
		{
			name:  "no roles — cannot reveal",
			roles: []string{},
			want:  false,
		},
		{
			name:  "unknown role — cannot reveal",
			roles: []string{"superuser"},
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claims := auth.KeycloakClaims{
				Subject: "test-subject",
				Email:   "test@example.com",
				RealmAccess: auth.RealmRoles{
					Roles: tc.roles,
				},
			}
			got := pii.CanRevealFull(claims)
			assert.Equal(t, tc.want, got, "CanRevealFull mismatch for roles %v", tc.roles)
		})
	}
}
