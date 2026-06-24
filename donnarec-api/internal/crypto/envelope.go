package crypto

import (
	"context"
	"crypto/rand"
	"fmt"
)

// EncryptField encrypts a PII field value using envelope encryption.
//
// Envelope encryption process:
//  1. Generate a fresh random 32-byte Data Encryption Key (DEK).
//  2. Encrypt plaintext with the DEK via AES-256-GCM.
//  3. Wrap (encrypt) the DEK with the KEK via KeyProvider.WrapKey.
//  4. Return ciphertext + wrappedDEK — store both alongside each other in DB.
//     The plaintext DEK is never returned, never stored, never logged.
//
// Anti-pattern avoided: the DEK is freshly generated per field, per call.
// Reusing a DEK across fields or calls would break forward secrecy.
//
// Storage contract:
//   - DB column for field: stores ciphertext (returned as first return value).
//   - DB column for DEK:   stores wrappedDEK (returned as second return value).
//   - The KEK is held only in the app process (from DONAREC_KEK env) — never in DB.
//
// Phase 3 usage: call EncryptField when writing donor national/tax ID to DB.
// Call DecryptField when reading and the caller is authorized to see plaintext.
// Non-authorized callers receive only the masked value (see internal/pii/mask.go).
func EncryptField(ctx context.Context, kp KeyProvider, plaintext []byte) (ciphertext, wrappedDEK []byte, err error) {
	// Step 1: generate a fresh random 32-byte DEK
	dek := make([]byte, 32)
	if _, err = rand.Read(dek); err != nil {
		return nil, nil, fmt.Errorf("EncryptField: generate DEK: %w", err)
	}
	// Zero the DEK from stack memory after use (best-effort; GC may still hold copies)
	defer func() { clearBytes(dek) }()

	// Step 2: encrypt plaintext with DEK
	ciphertext, err = Encrypt(dek, plaintext)
	if err != nil {
		return nil, nil, fmt.Errorf("EncryptField: encrypt plaintext: %w", err)
	}

	// Step 3: wrap DEK with KEK via KeyProvider
	wrappedDEK, err = kp.WrapKey(ctx, dek)
	if err != nil {
		return nil, nil, fmt.Errorf("EncryptField: wrap DEK: %w", err)
	}

	return ciphertext, wrappedDEK, nil
}

// DecryptField decrypts a PII field value that was encrypted via EncryptField.
//
// Decryption process:
//  1. Unwrap the DEK using KeyProvider.UnwrapKey (KEK decrypts wrappedDEK).
//  2. Decrypt ciphertext using the unwrapped DEK via AES-256-GCM.
//  3. Return plaintext — the unwrapped DEK is zeroed after use.
//
// Authorization note:
//
//	The caller is responsible for verifying that the requesting user is authorized
//	to receive plaintext PII before calling DecryptField.
//	Use pii.CanRevealFull(claims) for the role gate (D-10).
//	All reveals must be audited via the 01-02 PII-reveal audit path (D-13).
//	This function does not perform authorization checks itself — it only decrypts.
func DecryptField(ctx context.Context, kp KeyProvider, ciphertext, wrappedDEK []byte) ([]byte, error) {
	// Step 1: unwrap DEK using KEK
	dek, err := kp.UnwrapKey(ctx, wrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("DecryptField: unwrap DEK: %w", err)
	}
	defer func() { clearBytes(dek) }()

	// Step 2: decrypt ciphertext with DEK
	plaintext, err := Decrypt(dek, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("DecryptField: decrypt: %w", err)
	}

	return plaintext, nil
}

// clearBytes zeroes a byte slice in memory.
// This is best-effort; the Go GC may copy slices before this runs.
// For MVP this provides reasonable DEK hygiene without requiring CGo.
func clearBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
