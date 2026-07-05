// Package settings_test — BW-04 (04-REVIEW-PRESHIP.md) regression coverage: the
// full-settings "save all tabs" PUT (SaveSettings) must NOT write the brand-image
// object keys. Those keys are persisted out-of-band by the upload endpoint, so a
// SaveSettings body carrying stale/omitted image keys must leave the freshly
// uploaded assets untouched — never null or revert them.
package settings_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/settings"
	"github.com/donnarec/donnarec-api/internal/testutil"
)

// TestSaveSettings_DoesNotClobberUploadedImageKeys proves BW-04's fix: after a
// brand image has been uploaded (its object key persisted on the config row), a
// subsequent SaveSettings PUT whose body omits the image keys (they are nil on
// ReceiptSettings, as the real API never round-trips a raw upload through this
// struct) must leave the persisted image keys unchanged.
func TestSaveSettings_DoesNotClobberUploadedImageKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}

	ctx := context.Background()
	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)

	admin, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:           "admin-bw04@example.com",
		DisplayName:     "Admin BW04",
		KeycloakSubject: "admin-bw04-subject",
	})
	require.NoError(t, err)

	// Simulate assets uploaded out-of-band (the upload endpoint persisted keys).
	_, err = pool.Exec(ctx,
		`UPDATE receipt_template_config
		 SET letterhead_object_key = 'uploaded-letterhead',
		     seal_object_key = 'uploaded-seal',
		     signature_object_key = 'uploaded-signature',
		     watermark_object_key = 'uploaded-watermark'
		 WHERE id = true`)
	require.NoError(t, err)

	svc := settings.NewSettingsService(pool, queries, nil, zap.NewNop())

	// A "save all tabs" PUT that changes text fields but carries NO image keys.
	input := validSettings()
	input.TemplateHTML = `<html><body>EDITED {{.DonorName}}</body></html>`
	input.Section6TextTh = "แก้ไขข้อความ"
	require.Nil(t, input.LetterheadObjectKey, "the save body omits image keys (owned by the upload endpoint)")

	require.NoError(t, svc.SaveSettings(ctx, input, admin.ID))

	after, err := svc.GetSettings(ctx)
	require.NoError(t, err)

	// Text fields updated…
	assert.Equal(t, input.TemplateHTML, after.TemplateHTML)
	assert.Equal(t, "แก้ไขข้อความ", after.Section6TextTh)
	// …but image keys must survive the save untouched (BW-04).
	assert.Equal(t, "uploaded-letterhead", derefStr(after.LetterheadObjectKey))
	assert.Equal(t, "uploaded-seal", derefStr(after.SealObjectKey))
	assert.Equal(t, "uploaded-signature", derefStr(after.SignatureObjectKey))
	assert.Equal(t, "uploaded-watermark", derefStr(after.WatermarkObjectKey))
}
