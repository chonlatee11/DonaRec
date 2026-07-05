// Package storage_test — black-box tests for template-image (brand asset) upload
// validation (Phase 4, plan 04-07, Task 1).
//
// Mirrors client_test.go's ValidateSlip test shape, but for the narrower
// image-only allowlist (image/jpeg, image/png — NO application/pdf) and the
// smaller 2 MB cap that applies to letterhead/seal/signature/watermark brand
// assets (D-58 magic-byte mitigation, reusing the proven validateSlip pattern
// per 04-RESEARCH.md "Reuse internal/storage's magic-byte validation").
//
// All tests use in-memory byte readers — no live MinIO required.
// ValidateTemplateImage is the exported validation path, unit-testable
// independently of PutTemplateImage (mirrors ValidateSlip/PutSlip).
package storage_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donnarec/donnarec-api/internal/storage"
)

// TestTemplateImageAllowedTypes verifies that JPEG and PNG pass validation.
func TestTemplateImageAllowedTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		data []byte
	}{
		{"jpeg", jpegBytes()},
		{"png", pngBytes()},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mime, head, err := storage.ValidateTemplateImage(bytes.NewReader(tc.data), int64(len(tc.data)))
			require.NoError(t, err, "allowed image type %q should pass validation without error", tc.name)
			assert.NotNil(t, mime, "expected a non-nil MIME value for allowed type %q", tc.name)
			assert.NotEmpty(t, head, "expected non-empty header bytes consumed by magic detection for %q", tc.name)
		})
	}
}

// TestTemplateImageRejectsPDF verifies that a PDF — allowed for slips — is
// REJECTED for template images. Brand assets are images only (letterhead/seal/
// signature/watermark are all rendered as <img> tags in the receipt template).
func TestTemplateImageRejectsPDF(t *testing.T) {
	t.Parallel()

	data := pdfBytes()
	_, _, err := storage.ValidateTemplateImage(bytes.NewReader(data), int64(len(data)))
	require.ErrorIs(t, err, storage.ErrUnsupportedTemplateImageType,
		"a PDF must be rejected for template image upload (images only)")
}

// TestTemplateImageRejectsSpoofed verifies magic-byte detection (not filename/
// Content-Type header) rejects a non-image file disguised with an image name.
func TestTemplateImageRejectsSpoofed(t *testing.T) {
	t.Parallel()

	spoofed := append([]byte("#!/bin/sh\necho 'not an image'\n"), make([]byte, 512)...)
	_, _, err := storage.ValidateTemplateImage(bytes.NewReader(spoofed), int64(len(spoofed)))
	require.ErrorIs(t, err, storage.ErrUnsupportedTemplateImageType,
		"a spoofed file type must be rejected with ErrUnsupportedTemplateImageType")
}

// TestTemplateImageSizeLimit verifies the 2 MB cap (distinct from the 10 MB
// slip cap) — a brand image over 2 MB must be rejected; at-limit is accepted.
func TestTemplateImageSizeLimit(t *testing.T) {
	t.Parallel()

	const maxTemplateImageSize = 2 << 20 // 2 MB — same constant as production code

	t.Run("over_limit_rejected", func(t *testing.T) {
		t.Parallel()
		tooBig := int64(maxTemplateImageSize + 1)
		_, _, err := storage.ValidateTemplateImage(bytes.NewReader(make([]byte, tooBig)), tooBig)
		require.ErrorIs(t, err, storage.ErrTemplateImageTooLarge,
			"file declared at %d bytes (> 2 MB) must be rejected with ErrTemplateImageTooLarge", tooBig)
	})

	t.Run("at_limit_accepted", func(t *testing.T) {
		t.Parallel()
		atLimit := int64(maxTemplateImageSize)
		base := pngBytes()
		data := append(base, make([]byte, int(atLimit)-len(base))...)
		_, _, err := storage.ValidateTemplateImage(bytes.NewReader(data), atLimit)
		require.NoError(t, err, "file at exactly 2 MB must not be rejected by size check")
	})
}
