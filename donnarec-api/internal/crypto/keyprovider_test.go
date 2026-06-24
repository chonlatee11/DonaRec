// Package crypto_test tests the crypto package.
package crypto_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/donnarec/donnarec-api/internal/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnvKeyProvider validates that EnvKeyProvider correctly reads DONAREC_KEK
// from the environment and implements WrapKey / UnwrapKey.
func TestEnvKeyProvider(t *testing.T) {
	// valid 32-byte hex KEK (64 hex chars)
	const validKEKHex = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

	t.Run("valid KEK wraps and unwraps DEK", func(t *testing.T) {
		t.Setenv("DONAREC_KEK", validKEKHex)

		kp, err := crypto.NewEnvKeyProvider()
		require.NoError(t, err)

		dek := make([]byte, 32)
		for i := range dek {
			dek[i] = byte(i + 1)
		}

		ctx := context.Background()
		wrapped, err := kp.WrapKey(ctx, dek)
		require.NoError(t, err)
		assert.NotEmpty(t, wrapped)
		// wrapped DEK must not be the plaintext DEK
		assert.NotEqual(t, dek, wrapped)

		unwrapped, err := kp.UnwrapKey(ctx, wrapped)
		require.NoError(t, err)
		assert.Equal(t, dek, unwrapped, "unwrapped DEK must equal original DEK")
	})

	t.Run("missing DONAREC_KEK returns error", func(t *testing.T) {
		os.Unsetenv("DONAREC_KEK")
		_, err := crypto.NewEnvKeyProvider()
		require.Error(t, err, "must error when DONAREC_KEK is not set")
	})

	t.Run("short DONAREC_KEK returns error", func(t *testing.T) {
		// Only 16 bytes (32 hex chars) — too short for AES-256
		t.Setenv("DONAREC_KEK", "0102030405060708090a0b0c0d0e0f10")
		_, err := crypto.NewEnvKeyProvider()
		require.Error(t, err, "must error when DONAREC_KEK is not 32 bytes")
	})

	t.Run("non-hex DONAREC_KEK returns error", func(t *testing.T) {
		t.Setenv("DONAREC_KEK", "not-a-hex-string-at-all-and-wrong-length")
		_, err := crypto.NewEnvKeyProvider()
		require.Error(t, err, "must error when DONAREC_KEK is not valid hex")
	})
}

// TestAESGCMRoundTrip validates Encrypt/Decrypt and nonce uniqueness / tamper detection.
func TestAESGCMRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}

	plaintext := []byte("ข้อมูลทดสอบ PDPA — national ID: 1234567890123")

	t.Run("encrypt then decrypt returns original plaintext", func(t *testing.T) {
		ct, err := crypto.Encrypt(key, plaintext)
		require.NoError(t, err)
		assert.NotEqual(t, plaintext, ct, "ciphertext must differ from plaintext")

		got, err := crypto.Decrypt(key, ct)
		require.NoError(t, err)
		assert.Equal(t, plaintext, got)
	})

	t.Run("two encryptions of same plaintext produce different ciphertext (random nonce)", func(t *testing.T) {
		ct1, err := crypto.Encrypt(key, plaintext)
		require.NoError(t, err)

		ct2, err := crypto.Encrypt(key, plaintext)
		require.NoError(t, err)

		assert.False(t, bytes.Equal(ct1, ct2), "each Encrypt call must use a fresh random nonce")
	})

	t.Run("tampered ciphertext fails Decrypt (GCM auth tag)", func(t *testing.T) {
		ct, err := crypto.Encrypt(key, plaintext)
		require.NoError(t, err)

		// Flip a byte in the middle of the ciphertext (after the nonce prefix)
		ct[len(ct)/2] ^= 0xFF

		_, err = crypto.Decrypt(key, ct)
		require.Error(t, err, "Decrypt must reject tampered ciphertext")
	})

	t.Run("wrong key fails Decrypt", func(t *testing.T) {
		ct, err := crypto.Encrypt(key, plaintext)
		require.NoError(t, err)

		wrongKey := make([]byte, 32)
		_, err = crypto.Decrypt(wrongKey, ct)
		require.Error(t, err, "Decrypt must fail with wrong key")
	})

	t.Run("truncated ciphertext fails Decrypt", func(t *testing.T) {
		ct, err := crypto.Encrypt(key, plaintext)
		require.NoError(t, err)

		_, err = crypto.Decrypt(key, ct[:5])
		require.Error(t, err, "Decrypt must fail on truncated ciphertext")
	})
}

// TestEnvelopeRoundTrip validates EncryptField / DecryptField.
func TestEnvelopeRoundTrip(t *testing.T) {
	const validKEKHex = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	t.Setenv("DONAREC_KEK", validKEKHex)

	kp, err := crypto.NewEnvKeyProvider()
	require.NoError(t, err)

	plaintext := []byte("1234567890123") // example national ID

	ctx := context.Background()

	t.Run("encrypt field then decrypt returns original plaintext", func(t *testing.T) {
		ct, wrappedDEK, err := crypto.EncryptField(ctx, kp, plaintext)
		require.NoError(t, err)
		assert.NotEmpty(t, ct)
		assert.NotEmpty(t, wrappedDEK)

		got, err := crypto.DecryptField(ctx, kp, ct, wrappedDEK)
		require.NoError(t, err)
		assert.Equal(t, plaintext, got)
	})

	t.Run("wrappedDEK is not the raw DEK (DEK is wrapped)", func(t *testing.T) {
		ct, wrappedDEK, err := crypto.EncryptField(ctx, kp, plaintext)
		require.NoError(t, err)
		_ = ct

		// Wrapped DEK should not equal the raw 32-byte DEK plaintext
		// (it's encrypted under KEK so it must be different from a bare 32-byte key)
		assert.NotEqual(t, make([]byte, 32), wrappedDEK,
			"wrappedDEK must not be a zero-value DEK (sanity check)")
	})

	t.Run("tampered ciphertext fails DecryptField", func(t *testing.T) {
		ct, wrappedDEK, err := crypto.EncryptField(ctx, kp, plaintext)
		require.NoError(t, err)

		ct[len(ct)/2] ^= 0xFF
		_, err = crypto.DecryptField(ctx, kp, ct, wrappedDEK)
		require.Error(t, err, "must reject tampered field ciphertext")
	})
}

// TestBlindIndex validates HMAC-SHA256 determinism and key-sensitivity.
func TestBlindIndex(t *testing.T) {
	indexKey := make([]byte, 32)
	for i := range indexKey {
		indexKey[i] = byte(i + 1)
	}

	plaintext := []byte("1234567890123")

	t.Run("same input and key produce same output (deterministic)", func(t *testing.T) {
		h1 := crypto.BlindIndex(plaintext, indexKey)
		h2 := crypto.BlindIndex(plaintext, indexKey)
		assert.Equal(t, h1, h2, "BlindIndex must be deterministic for same input+key")
	})

	t.Run("different plaintext produces different output", func(t *testing.T) {
		h1 := crypto.BlindIndex(plaintext, indexKey)
		h2 := crypto.BlindIndex([]byte("9999999999999"), indexKey)
		assert.NotEqual(t, h1, h2)
	})

	t.Run("different key produces different output", func(t *testing.T) {
		otherKey := make([]byte, 32)
		h1 := crypto.BlindIndex(plaintext, indexKey)
		h2 := crypto.BlindIndex(plaintext, otherKey)
		assert.NotEqual(t, h1, h2, "different index key must produce different BlindIndex output")
	})

	t.Run("output is 32 bytes (SHA-256 digest size)", func(t *testing.T) {
		h := crypto.BlindIndex(plaintext, indexKey)
		assert.Len(t, h, 32, "HMAC-SHA256 output must be 32 bytes")
	})
}
