// Package crypto provides envelope encryption primitives for donnarec-api.
//
// Design:
//   - KeyProvider interface abstracts KMS operations (wrap/unwrap DEK).
//     Swap to CloudKMSProvider without changing any call site.
//   - EnvKeyProvider is the MVP implementation; KEK is read from DONAREC_KEK env var.
//   - AES-256-GCM (stdlib only) provides authenticated encryption.
//   - BlindIndex (HMAC-SHA256) enables deterministic searchable index without
//     exposing plaintext PII (Phase 3 search usage — this package provides the mechanism).
//
// BOUNDARY CONTRACT (Foundational Rule 5 — LOCKED after Phase 1 commit):
//
//	type KeyProvider interface {
//	    WrapKey(ctx context.Context, plaintextDEK []byte) ([]byte, error)
//	    UnwrapKey(ctx context.Context, wrappedDEK []byte) ([]byte, error)
//	}
//
// Do NOT change these signatures after Phase 1 is committed.
// To add capabilities, embed the interface in a new extended interface.
//
// Phase scope:
//   - Phase 1 (01-03): crypto boundary, key provider, AES-GCM, envelope, blind index
//   - Phase 3: EncryptField/DecryptField applied to donor national/tax ID
//   - Future: swap EnvKeyProvider for AWS KMS / GCP KMS / HashiCorp Vault provider
package crypto

import "context"

// KeyProvider abstracts Key Encryption Key (KEK) operations.
//
// MVP implementation: EnvKeyProvider reads KEK from DONAREC_KEK env var.
// Future implementation: CloudKMSProvider delegates to AWS KMS / GCP KMS / HashiCorp Vault.
//
// LOCKED BOUNDARY: This interface must not change after Phase 1 commit.
// If new KMS capabilities are needed, embed this interface in an extended interface
// rather than adding methods here. Call sites depend on this exact shape.
//
// Phase 3 will use this interface to encrypt donor national ID / tax ID before
// writing to PostgreSQL. DB stores only ciphertext + wrappedDEK — never plaintext.
type KeyProvider interface {
	// WrapKey encrypts a plaintext Data Encryption Key (DEK) using the Key Encryption Key (KEK).
	// Returns the wrapped (encrypted) DEK suitable for storage alongside ciphertext.
	// The DEK must never be stored or logged in plaintext.
	WrapKey(ctx context.Context, plaintextDEK []byte) ([]byte, error)

	// UnwrapKey decrypts a wrapped DEK using the Key Encryption Key (KEK).
	// Returns the plaintext DEK for use in a single decrypt operation.
	// The returned DEK must be zeroed from memory after use and must not be persisted.
	UnwrapKey(ctx context.Context, wrappedDEK []byte) ([]byte, error)
}
