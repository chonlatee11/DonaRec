// Package settings_test — BW-03 (04-REVIEW-PRESHIP.md) regression coverage:
// SaveTemplateImage must persist a brand-image slot key with a single atomic
// per-column write, NOT a read-modify-write of the whole config row. Two
// near-simultaneous uploads to DIFFERENT slots must both survive — the old
// read-whole-row / mutate-one-slot / write-whole-row path races, and the later
// full-row writer clobbers the other slot's freshly-uploaded key (lost update).
package settings_test

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/settings"
	"github.com/donnarec/donnarec-api/internal/testutil"
)

// keyReturningStore is a fake ReceiptsStore whose PutTemplateImage returns a
// deterministic per-slot key ("key-<slot>") so the test can assert each slot
// ends with its own uploaded key and nothing clobbered a sibling.
type keyReturningStore struct{}

func (keyReturningStore) GetObject(_ context.Context, _ string) ([]byte, error) { return nil, nil }

func (keyReturningStore) PutTemplateImage(_ context.Context, _ io.Reader, _ int64, slot string) (string, string, error) {
	return "key-" + slot, "image/png", nil
}

// TestSaveTemplateImage_ConcurrentSlotUploads_NoLostUpdate launches one upload
// per slot concurrently, all released together, and asserts every slot ends
// with its own uploaded key. Under the pre-fix read-modify-write path this
// races and a sibling slot is reverted to its seeded initial key; the atomic
// single-column write never loses a sibling's value.
func TestSaveTemplateImage_ConcurrentSlotUploads_NoLostUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}

	ctx := context.Background()
	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)

	admin, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:           "admin-bw03@example.com",
		DisplayName:     "Admin BW03",
		KeycloakSubject: "admin-bw03-subject",
	})
	require.NoError(t, err)

	// Seed distinct initial keys so a lost update is observable as a revert.
	_, err = pool.Exec(ctx,
		`UPDATE receipt_template_config
		 SET letterhead_object_key = 'init-letterhead',
		     seal_object_key = 'init-seal',
		     signature_object_key = 'init-signature',
		     watermark_object_key = 'init-watermark'
		 WHERE id = true`)
	require.NoError(t, err)

	svc := settings.NewSettingsService(pool, queries, keyReturningStore{}, zap.NewNop())

	slots := []string{"letterhead", "seal", "signature", "watermark"}
	start := make(chan struct{})
	errs := make(chan error, len(slots))
	var wg sync.WaitGroup
	for _, slot := range slots {
		slot := slot
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, e := svc.SaveTemplateImage(ctx, slot, strings.NewReader("x"), 1, admin.ID)
			errs <- e
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}

	after, err := svc.GetSettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, "key-letterhead", derefStr(after.LetterheadObjectKey))
	assert.Equal(t, "key-seal", derefStr(after.SealObjectKey))
	assert.Equal(t, "key-signature", derefStr(after.SignatureObjectKey))
	assert.Equal(t, "key-watermark", derefStr(after.WatermarkObjectKey))
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
