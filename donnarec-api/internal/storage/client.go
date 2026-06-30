// Package storage provides the MinIO/S3-compatible object storage client for donation slip files.
//
// Design decisions realized here:
//
//	D-48: slip attachment is optional in Flow A — cash/no-slip donations fully supported
//	D-54: soft-delete only (deleted_at); files retained in object storage; audited on remove
//	T-03-14: magic-byte validation — content type determined from actual bytes, not filename/header
//	T-03-15: 10 MB size cap via declared-size check + io.LimitReader defense-in-depth in PutSlip
//	T-03-16: presigned URLs with 15-min TTL; object key contains UUID (not publicly guessable)
//
// ValidateSlip is exported so unit tests can exercise the validation path without a live MinIO.
// PutSlip calls validateSlip internally, then streams to MinIO only if validation succeeds.
package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// maxSlipSize is the maximum allowed slip file size: 10 MB (T-03-15, A5 in RESEARCH.md).
const maxSlipSize = 10 << 20

// allowedMIMETypes is the set of content types accepted for slip files.
// Checked against magic-byte detection — not the caller-provided Content-Type header.
var allowedMIMETypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"application/pdf": true,
}

// ErrUnsupportedFileType is returned when a slip file has an unsupported MIME type.
// Magic-byte detection (not Content-Type header) is used to determine the actual type.
// Allowed types: image/jpeg, image/png, application/pdf (Pattern 4, RESEARCH.md, T-03-14).
var ErrUnsupportedFileType = errors.New("storage: unsupported file type — only image/jpeg, image/png, application/pdf are allowed")

// ErrFileTooLarge is returned when a slip file exceeds the maximum allowed size (10 MB).
var ErrFileTooLarge = errors.New("storage: file exceeds maximum allowed size of 10 MB")

// StorageClient wraps a MinIO client with the donnarec slip-storage contract.
// Use NewStorageClient to construct.
type StorageClient struct {
	client *minio.Client
	bucket string
}

// NewStorageClient constructs a StorageClient that connects to the given MinIO/S3 endpoint.
// Mirrors the panic-guard constructor style from audit.NewAuditService.
//
//	endpoint:  host:port of the MinIO server (e.g. "localhost:9000")
//	accessKey: MINIO_ACCESS_KEY / MINIO_ROOT_USER
//	secretKey: MINIO_SECRET_KEY / MINIO_ROOT_PASSWORD
//	bucket:    target bucket name (e.g. "donnarec-slips")
//	secure:    true for HTTPS (prod), false for HTTP (dev)
func NewStorageClient(endpoint, accessKey, secretKey, bucket string, secure bool) (*StorageClient, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: minio client init: %w", err)
	}
	return &StorageClient{client: client, bucket: bucket}, nil
}

// validateSlip checks declared size and magic bytes of the provided reader.
// Returns the detected MIME type and the consumed header bytes (for reassembly via
// io.MultiReader). Does NOT consume the full stream — only the first 512 bytes.
//
// Size check (T-03-15): rejects if declared size exceeds maxSlipSize (10 MB).
// MIME check  (T-03-14): rejects if magic bytes do not match the allowedMIMETypes set.
func validateSlip(r io.Reader, size int64) (*mimetype.MIME, []byte, error) {
	// Fast-reject by declared size (T-03-15). PutSlip also wraps r in io.LimitReader
	// as defense-in-depth — this first check avoids buffering anything unnecessarily.
	if size > maxSlipSize {
		return nil, nil, ErrFileTooLarge
	}

	// Buffer the first 512 bytes for magic-byte detection.
	// io.ReadFull returns io.ErrUnexpectedEOF when the reader has fewer than 512 bytes
	// (small files) — that is fine; we detect on whatever was read.
	buf := make([]byte, 512)
	n, err := io.ReadFull(r, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, nil, fmt.Errorf("storage: read slip header: %w", err)
	}

	detected := mimetype.Detect(buf[:n])

	if !allowedMIMETypes[detected.String()] {
		return nil, nil, ErrUnsupportedFileType
	}

	return detected, buf[:n], nil
}

// ValidateSlip validates the MIME type (via magic bytes) and declared size of a slip.
// Returns the detected MIME type, the consumed header bytes (for io.MultiReader reassembly),
// and any error.
//
// Exported for unit testing without a live MinIO client. PutSlip calls this internally.
func ValidateSlip(r io.Reader, size int64) (*mimetype.MIME, []byte, error) {
	return validateSlip(r, size)
}

// PutSlip validates and uploads a slip file to MinIO/S3.
// It calls validateSlip first (magic-byte + size check), then reassembles the reader
// (prepending consumed header bytes) before streaming to MinIO.
//
// objectKey format: "slips/{donationID}/{uuid}{ext}"
// The UUID in the key prevents guessing (T-03-16).
// donationID is included for logical grouping (Phase 6 reuse path).
//
// Returns (objectKey, mimeType, error). mimeType is the magic-byte-detected MIME string
// (e.g. "image/jpeg") needed by the caller to persist in the slip_attachments record.
func (s *StorageClient) PutSlip(ctx context.Context, r io.Reader, size int64, donationID string) (string, string, error) {
	detected, head, err := validateSlip(r, size)
	if err != nil {
		return "", "", err
	}

	// Reassemble: prepend the header bytes consumed during detection (Pattern 4, RESEARCH.md).
	// Also wrap remaining reader in LimitReader as defense-in-depth against a lying size (T-03-15).
	remaining := io.LimitReader(r, maxSlipSize-int64(len(head))+1)
	combined := io.MultiReader(bytes.NewReader(head), remaining)

	objectKey := fmt.Sprintf("slips/%s/%s%s", donationID, uuid.NewString(), detected.Extension())

	_, err = s.client.PutObject(ctx, s.bucket, objectKey, combined, size, minio.PutObjectOptions{
		ContentType: detected.String(),
	})
	if err != nil {
		return "", "", fmt.Errorf("storage: put slip object: %w", err)
	}

	return objectKey, detected.String(), nil
}

// PresignedGet returns a short-lived presigned URL for reading an object (T-03-16).
// TTL should be 15 minutes for UI display (UI-SPEC Screen 5).
// The URL is time-limited and the key is not publicly guessable (contains UUID).
func (s *StorageClient) PresignedGet(ctx context.Context, objectKey string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, s.bucket, objectKey, ttl, nil)
	if err != nil {
		return "", fmt.Errorf("storage: presign slip get: %w", err)
	}
	return u.String(), nil
}
