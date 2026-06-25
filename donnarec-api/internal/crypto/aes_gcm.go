package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
)

// Encrypt encrypts plaintext using AES-256-GCM authenticated encryption.
// The key must be exactly 16, 24, or 32 bytes (for AES-128, AES-192, or AES-256).
//
// Output format: nonce || ciphertext (nonce is gcm.NonceSize() bytes, prepended).
// The random nonce ensures that two encryptions of the same plaintext with the same
// key produce different ciphertext (nonce-uniqueness property — required for IND-CPA security).
//
// The GCM authentication tag is appended by cipher.AEAD.Seal and is verified
// on Decrypt — any tampering causes Decrypt to return an error.
//
// Do NOT use this function directly on PII fields from outside this package.
// Use EncryptField (envelope.go) which generates a fresh DEK per field and wraps it
// via KeyProvider. Calling Encrypt directly with a long-lived key violates the
// envelope model (Foundational Rule 5).
func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm init: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("nonce generation: %w", err)
	}
	// Seal appends ciphertext + auth tag after nonce.
	// gcm.Seal(dst, nonce, plaintext, additionalData)
	// dst = nonce → output is nonce || ciphertext+tag
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts a nonce-prefixed AES-256-GCM ciphertext produced by Encrypt.
//
// Returns an error if the ciphertext is too short, the nonce cannot be extracted,
// or the GCM authentication tag verification fails (indicating tampering or wrong key).
func Decrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm init: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short: got %d bytes, need at least %d", len(ciphertext), nonceSize)
	}
	nonce, data := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		// GCM Open fails on tampered ciphertext or wrong key — do not leak details
		return nil, fmt.Errorf("decryption failed (tampered ciphertext or wrong key)")
	}
	return plaintext, nil
}

// BlindIndex computes a deterministic HMAC-SHA256 of plaintext using indexKey.
//
// This is used to create searchable blind indexes for PII fields without
// exposing the plaintext value. The indexKey must be kept separate from the DEK
// and KEK — it is a dedicated index key loaded from a separate env var (Phase 3).
//
// Properties:
//   - Deterministic: same plaintext + indexKey always produces same output.
//   - Key-sensitive: different indexKey produces different output.
//   - One-way: the indexKey makes it computationally infeasible to reverse
//     the HMAC output to recover the plaintext.
//
// Output: 32 bytes (SHA-256 digest size).
//
// Phase 3 usage: store BlindIndex(nationalID, indexKey) alongside the encrypted
// national ID field. Query by BlindIndex value — never by ciphertext directly.
// Do NOT index the ciphertext column (ciphertext changes with each Encrypt call due
// to random nonce — indexing it would never match on lookup).
func BlindIndex(plaintext, indexKey []byte) []byte {
	mac := hmac.New(sha256.New, indexKey)
	mac.Write(plaintext)
	return mac.Sum(nil)
}
