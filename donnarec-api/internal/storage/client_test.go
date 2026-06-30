// Package storage_test — black-box tests for the MinIO storage client (plan 03-04).
//
// Tests validate the three file-validation invariants that ValidateSlip must enforce:
//   - magic-byte detection (not Content-Type header or file extension)
//   - hard 10 MB size cap
//   - allowlist: only image/jpeg, image/png, application/pdf
//
// All tests use in-memory byte readers — no live MinIO required.
// ValidateSlip is the exported validation path, unit-testable independently of PutSlip.
//
// References:
//
//	RESEARCH.md §"Pattern 4: Magic-Byte Slip Validation" (T-03-14, T-03-15)
//	03-04-PLAN.md §"Task 1" §<behavior>
package storage_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donnarec/donnarec-api/internal/storage"
)

// --- magic-byte helpers (minimal, deterministic) ---

// jpegBytes returns a minimal JPEG magic-byte prefix padded to 512 bytes.
// JPEG magic: FF D8 FF E0 (JFIF) — sufficient for mimetype.Detect to identify.
func jpegBytes() []byte {
	b := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01}
	return append(b, make([]byte, 512)...)
}

// pngBytes returns a minimal PNG magic-byte prefix padded to 512 bytes.
// PNG magic: 89 50 4E 47 0D 0A 1A 0A — the canonical PNG signature.
func pngBytes() []byte {
	b := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	return append(b, make([]byte, 512)...)
}

// pdfBytes returns a minimal PDF magic-byte prefix padded to 512 bytes.
// PDF magic: 25 50 44 46 2D (= "%PDF-").
func pdfBytes() []byte {
	b := []byte{'%', 'P', 'D', 'F', '-', '1', '.', '4', '\n'}
	return append(b, make([]byte, 512)...)
}

// TestAllowedTypes verifies that JPEG, PNG, and PDF files pass the magic-byte allowlist.
// Uses in-memory byte readers — no live MinIO required (03-04 §Task 1 §<behavior>).
//
// Invariants checked:
//   - ValidateSlip returns nil error for each allowed type
//   - A non-nil MIME value is returned
//   - Header bytes (consumed for magic detection) are non-empty
func TestAllowedTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		data []byte
	}{
		{"jpeg", jpegBytes()},
		{"png", pngBytes()},
		{"pdf", pdfBytes()},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mime, head, err := storage.ValidateSlip(bytes.NewReader(tc.data), int64(len(tc.data)))
			require.NoError(t, err, "allowed type %q should pass validation without error", tc.name)
			assert.NotNil(t, mime, "expected a non-nil MIME value for allowed type %q", tc.name)
			assert.NotEmpty(t, head, "expected non-empty header bytes consumed by magic detection for %q", tc.name)
		})
	}
}

// TestMagicByteRejectsSpoofed verifies that ValidateSlip rejects a file whose actual
// content does not match an allowed MIME type, regardless of the declared name/extension.
//
// Scenario: a shell script renamed to "slip.jpg" must be rejected.
// The caller declares a small size (not triggering ErrFileTooLarge first) so that the
// code path reaches the MIME-type check. Expected error: ErrUnsupportedFileType.
//
// T-03-14: magic bytes, not file extension or Content-Type header.
func TestMagicByteRejectsSpoofed(t *testing.T) {
	t.Parallel()

	// Plain text / shell script — clearly not an image or PDF.
	// Padded so ReadFull can read a full 512-byte header buffer.
	spoofed := append([]byte("#!/bin/sh\necho 'this is not a valid slip'\n"), make([]byte, 512)...)

	_, _, err := storage.ValidateSlip(bytes.NewReader(spoofed), int64(len(spoofed)))
	require.ErrorIs(t, err, storage.ErrUnsupportedFileType,
		"a spoofed file type must be rejected with ErrUnsupportedFileType")
}

// TestSizeLimit verifies that ValidateSlip rejects streams declared as larger than 10 MB.
//
// The 10 MB cap is 10 << 20 bytes. A stream with size 10<<20+1 must return ErrFileTooLarge.
// Files at exactly 10 MB (10<<20) should be accepted (boundary is exclusive on the large side).
//
// T-03-15: DoS protection — oversized uploads must not exhaust server memory/disk.
func TestSizeLimit(t *testing.T) {
	t.Parallel()

	const maxSlipSize = 10 << 20 // same constant as production code

	t.Run("over_limit_rejected", func(t *testing.T) {
		t.Parallel()
		tooBig := int64(maxSlipSize + 1)
		// Reader content doesn't matter for the declared-size check.
		_, _, err := storage.ValidateSlip(bytes.NewReader(make([]byte, tooBig)), tooBig)
		require.ErrorIs(t, err, storage.ErrFileTooLarge,
			"file declared at %d bytes (> 10 MB) must be rejected with ErrFileTooLarge", tooBig)
	})

	t.Run("at_limit_accepted", func(t *testing.T) {
		t.Parallel()
		// Exactly 10 MB of JPEG data — should be accepted (edge case boundary).
		atLimit := int64(maxSlipSize)
		base := jpegBytes()
		data := append(base, make([]byte, int(atLimit)-len(base))...)
		_, _, err := storage.ValidateSlip(bytes.NewReader(data), atLimit)
		require.NoError(t, err, "file at exactly 10 MB must not be rejected by size check")
	})
}
