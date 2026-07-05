// Package testutil provides shared test infrastructure for donnarec-api integration tests.
package testutil

import (
	"context"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/require"
	miniotc "github.com/testcontainers/testcontainers-go/modules/minio"
)

// minioImage pins the MinIO test image (mirrors SetupTestPostgres's "postgres:17"
// pinning convention — a fixed tag, not ":latest", for reproducible CI runs).
const minioImage = "minio/minio:RELEASE.2024-01-16T16-07-38Z"

// StartMinio starts a MinIO testcontainer, creates the given bucket, and returns
// connection details usable directly by storage.NewStorageClient (endpoint,
// accessKey, secretKey) — the worker (04-05) integration tests use this for a
// real (non-mocked) receipts-bucket object store, mirroring SetupTestPostgres/
// StartChrome's t.Helper/require.NoError/t.Cleanup shape.
//
// Usage in test files:
//
//	endpoint, accessKey, secretKey := testutil.StartMinio(t, "test-receipts")
//	store, err := storage.NewStorageClient(endpoint, accessKey, secretKey, "test-receipts", false)
func StartMinio(t *testing.T, bucket string) (endpoint, accessKey, secretKey string) {
	t.Helper()
	ctx := context.Background()

	container, err := miniotc.Run(ctx, minioImage)
	require.NoError(t, err, "failed to start minio container")

	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("warning: failed to terminate minio container: %v", err)
		}
	})

	endpoint, err = container.ConnectionString(ctx)
	require.NoError(t, err, "failed to get minio connection string")

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(container.Username, container.Password, ""),
		Secure: false,
	})
	require.NoError(t, err, "failed to create minio client for bucket setup")

	err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
	require.NoError(t, err, "failed to create minio bucket %q", bucket)

	return endpoint, container.Username, container.Password
}
