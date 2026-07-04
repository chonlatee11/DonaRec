// Package worker — white-box unit test for fetchTemplateImageSoft (WR-06,
// 04-REVIEW.md): a fetch failure on a non-critical branding image
// (letterhead/seal/signature/watermark) must fail OPEN (log + empty image),
// never propagate an error that would fail the entire receipt render.
package worker

import (
	"context"
	"errors"
	"html/template"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// failingStore is a ReceiptsStore whose GetObject always errors — simulates a
// transient MinIO blip fetching a branding asset.
type failingStore struct{}

func (failingStore) GetObject(ctx context.Context, objectKey string) ([]byte, error) {
	return nil, errors.New("failingStore: simulated transient object-storage error")
}

func (failingStore) PutObject(ctx context.Context, objectKey string, data []byte, contentType string) error {
	return nil
}

func TestFetchTemplateImageSoft_FailsOpenAndLogsOnStoreError(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	w := &Worker{receiptsStore: failingStore{}, logger: zap.New(core)}

	key := "watermark-object-key.png"
	uri := w.fetchTemplateImageSoft(context.Background(), &key, "watermark")

	assert.Equal(t, template.URL(""), uri, "a failed branding-image fetch must fail open — empty image, not an error")

	entries := observed.All()
	require.Len(t, entries, 1, "the fetch failure must be logged, not silently discarded")
	assert.Equal(t, "watermark", entries[0].ContextMap()["image"])
}

func TestFetchTemplateImageSoft_NilObjectKey_NoStoreCallNoLog(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	w := &Worker{receiptsStore: failingStore{}, logger: zap.New(core)}

	uri := w.fetchTemplateImageSoft(context.Background(), nil, "seal")

	assert.Equal(t, template.URL(""), uri)
	assert.Empty(t, observed.All(), "an unset object key is the normal 'no asset uploaded yet' case — not a failure worth logging")
}
