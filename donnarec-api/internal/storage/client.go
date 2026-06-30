// Package storage provides the MinIO/S3-compatible object storage client for donation slip files.
// Full implementation (NewStorageClient, PutSlip, PresignedGet) is in plan 03-04.
//
// Wave 0 stub — defines the package and public sentinel errors so that:
//   - client_test.go (test scaffolds) can compile as package storage_test
//   - handler code in later plans can import the sentinel errors without circular deps
package storage

import "errors"

// ErrUnsupportedFileType is returned when a slip file has an unsupported MIME type.
// Magic-byte detection (not Content-Type header) is used to determine the actual type.
// Allowed types: image/jpeg, image/png, application/pdf (Pattern 4, RESEARCH.md).
var ErrUnsupportedFileType = errors.New("storage: unsupported file type — only image/jpeg, image/png, application/pdf are allowed")

// ErrFileTooLarge is returned when a slip file exceeds the maximum allowed size (10 MB).
var ErrFileTooLarge = errors.New("storage: file exceeds maximum allowed size of 10 MB")
