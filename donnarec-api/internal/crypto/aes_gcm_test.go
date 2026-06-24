// Package crypto_test — additional tests for aes_gcm.go helpers
// (core round-trip / tamper / nonce tests live in keyprovider_test.go)
package crypto_test

import (
	"testing"

	"github.com/donnarec/donnarec-api/internal/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEncryptDecryptEmpty verifies that empty plaintext is handled gracefully.
func TestEncryptDecryptEmpty(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 0x10)
	}

	ct, err := crypto.Encrypt(key, []byte{})
	require.NoError(t, err, "Encrypt must accept empty plaintext")

	got, err := crypto.Decrypt(key, ct)
	require.NoError(t, err)
	assert.Equal(t, []byte{}, got, "Decrypt of encrypted empty plaintext must return empty slice")
}

// TestEncryptKeyLength verifies that Encrypt rejects keys that are not 16/24/32 bytes.
func TestEncryptKeyLength(t *testing.T) {
	badKey := make([]byte, 10) // not a valid AES key size
	_, err := crypto.Encrypt(badKey, []byte("test"))
	require.Error(t, err, "Encrypt must reject invalid AES key lengths")
}
