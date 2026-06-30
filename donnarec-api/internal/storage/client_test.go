// Package storage_test — black-box test scaffolds for the MinIO storage client (Wave 0).
//
// These tests define the contract for the three file-validation invariants
// that StorageClient.PutSlip must enforce (Pattern 4, RESEARCH.md):
//   - magic-byte detection (not Content-Type header or file extension)
//   - hard 10 MB size cap
//   - allowlist: only image/jpeg, image/png, application/pdf
//
// Full implementation (NewStorageClient, PutSlip, PresignedGet) is in plan 03-04.
// All tests below skip unconditionally — they are Wave 0 contracts, not yet implemented.
package storage_test

import "testing"

// TestMagicByteRejectsSpoofed verifies that PutSlip rejects files where the actual
// content (magic bytes) does not match an allowed MIME type, even when the filename
// extension or caller-provided Content-Type suggests it should be valid.
//
// Example scenario: a file renamed to "slip.jpg" that is actually a ZIP archive
// must be rejected with ErrUnsupportedFileType, not accepted based on the extension.
//
// Relies on github.com/gabriel-vasile/mimetype.Detect (already in go.mod).
// Requires MinIO testcontainer or mock. Skip with -short.
func TestMagicByteRejectsSpoofed(t *testing.T) {
	if testing.Short() {
		t.Skip("Wave 0 scaffold — implemented in plan 03-04 (storage client magic-byte validation)")
	}
	t.Skip("Wave 0 scaffold — implemented in plan 03-04 (storage client magic-byte validation)")
}

// TestSizeLimit verifies that PutSlip rejects files larger than 10 MB with ErrFileTooLarge.
// Files at exactly 10 MB must be accepted; files at 10 MB + 1 byte must be rejected.
//
// Requires MinIO testcontainer or mock. Skip with -short.
func TestSizeLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("Wave 0 scaffold — implemented in plan 03-04 (storage client size limit)")
	}
	t.Skip("Wave 0 scaffold — implemented in plan 03-04 (storage client size limit)")
}

// TestAllowedTypes verifies that PutSlip accepts exactly three MIME types:
//   - image/jpeg  (JPEG magic bytes: FF D8 FF)
//   - image/png   (PNG magic bytes: 89 50 4E 47)
//   - application/pdf (PDF magic bytes: 25 50 44 46)
//
// All other types (e.g. text/plain, image/gif, application/zip) must return ErrUnsupportedFileType.
//
// Requires MinIO testcontainer or mock. Skip with -short.
func TestAllowedTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("Wave 0 scaffold — implemented in plan 03-04 (storage client allowed MIME types)")
	}
	t.Skip("Wave 0 scaffold — implemented in plan 03-04 (storage client allowed MIME types)")
}
