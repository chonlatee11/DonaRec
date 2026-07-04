// Package settings_test — tests for SettingsService (Phase 4, plan 04-07, Task 1).
//
// Pure-logic tests (invalid template, invalid number format, invalid image slot) use no
// database — mirrors internal/receiptno/format_test.go's dependency-free style.
// The save/get round-trip test uses a real Postgres testcontainer (testutil.SetupTestPostgres),
// mirroring internal/receiptno/allocator_test.go's `testing.Short()`-gated integration shape.
package settings_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/settings"
	"github.com/donnarec/donnarec-api/internal/testutil"
)

// validSettings returns a ReceiptSettings value that passes all validation.
func validSettings() settings.ReceiptSettings {
	return settings.ReceiptSettings{
		TemplateHTML:        `<html><body>{{.DonorName}} {{.ReceiptNo}}</body></html>`,
		TemplateHTMLEn:      `<html><body>{{.DonorName}} {{.ReceiptNo}}</body></html>`,
		Section6TextTh:      "หัก ณ ที่จ่าย 1 เท่า",
		Section6TextEn:      "1x tax deduction",
		DeductionMultiplier: "1x",
		Separator:           "/",
		RunningNoPadding:    6,
		YearFormat:          "BE4",
		Prefix:              "",
	}
}

// TestSaveSettings_InvalidTemplate_Rejected verifies that a template string which fails
// html/template.Parse is rejected with ErrInvalidTemplate BEFORE any DB write — proven by
// passing db.New(nil) (a Queries with a nil DBTX): if SaveSettings touched the DB before
// validating, this would panic instead of returning cleanly.
func TestSaveSettings_InvalidTemplate_Rejected(t *testing.T) {
	t.Parallel()

	svc := settings.NewSettingsService(nil, db.New(nil), nil, zap.NewNop())

	t.Run("thai_template_invalid", func(t *testing.T) {
		input := validSettings()
		input.TemplateHTML = `{{.DonorName` // malformed action, unparseable
		err := svc.SaveSettings(context.Background(), input, pgtype.UUID{})
		require.ErrorIs(t, err, settings.ErrInvalidTemplate)
	})

	t.Run("english_template_invalid", func(t *testing.T) {
		input := validSettings()
		input.TemplateHTMLEn = `{{if}}` // malformed action, unparseable
		err := svc.SaveSettings(context.Background(), input, pgtype.UUID{})
		require.ErrorIs(t, err, settings.ErrInvalidTemplate)
	})
}

// TestSaveSettings_InvalidNumberFormat_Rejected verifies that a separator/prefix
// containing characters outside the safe allowlist (mirrors
// internal/receiptno/format.go's configCharAllowlist) is rejected with
// ErrInvalidNumberFormat before any DB write — same nil-DBTX proof technique as above.
func TestSaveSettings_InvalidNumberFormat_Rejected(t *testing.T) {
	t.Parallel()

	svc := settings.NewSettingsService(nil, db.New(nil), nil, zap.NewNop())

	t.Run("dangerous_separator", func(t *testing.T) {
		input := validSettings()
		input.Separator = `<script>`
		err := svc.SaveSettings(context.Background(), input, pgtype.UUID{})
		require.ErrorIs(t, err, settings.ErrInvalidNumberFormat)
	})

	t.Run("dangerous_prefix", func(t *testing.T) {
		input := validSettings()
		input.Prefix = `"><img src=x>`
		err := svc.SaveSettings(context.Background(), input, pgtype.UUID{})
		require.ErrorIs(t, err, settings.ErrInvalidNumberFormat)
	})
}

// TestSaveTemplateImage_InvalidSlot_Rejected verifies that an unknown image slot name is
// rejected with ErrInvalidImageSlot BEFORE the receiptsStore is ever touched — proven by
// passing a nil ReceiptsStore (would nil-panic if called).
func TestSaveTemplateImage_InvalidSlot_Rejected(t *testing.T) {
	t.Parallel()

	svc := settings.NewSettingsService(nil, db.New(nil), nil, zap.NewNop())

	_, err := svc.SaveTemplateImage(context.Background(), "not-a-real-slot", nil, 0, pgtype.UUID{})
	require.ErrorIs(t, err, settings.ErrInvalidImageSlot)
}

// TestSettingsService_SaveAndGet_RoundTrip proves SaveSettings + GetSettings round-trip
// through a real Postgres instance: both receipt_template_config (04-01 seed) and
// receipt_number_config (Phase 2 seed) are read back with the newly-saved values,
// including updated_by set to the caller-supplied users.id.
func TestSettingsService_SaveAndGet_RoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}
	t.Parallel()

	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)
	ctx := context.Background()

	// A real users row so updated_by carries a plausible users.id (no FK constraint on
	// the column, but this proves the value round-trips as a real UUID either way).
	adminRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:           "admin-settings-test@example.com",
		DisplayName:     "Admin Settings Test",
		KeycloakSubject: "admin-settings-test-subject",
	})
	require.NoError(t, err)

	svc := settings.NewSettingsService(pool, queries, nil, zap.NewNop())

	// GetSettings must succeed against the 04-01/Phase-2 seeded rows before any save.
	initial, err := svc.GetSettings(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, initial.TemplateHTML, "the 04-01-seeded Thai template must be non-empty before any admin edit")

	input := validSettings()
	input.TemplateHTML = `<html><body>ROUND TRIP {{.DonorName}}</body></html>`
	input.Section6TextTh = "ทดสอบ round trip"
	input.DeductionMultiplier = "2x"
	input.Separator = "-"
	input.RunningNoPadding = 8
	input.YearFormat = "CE4"
	input.Prefix = "RT"

	err = svc.SaveSettings(ctx, input, adminRow.ID)
	require.NoError(t, err)

	after, err := svc.GetSettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, input.TemplateHTML, after.TemplateHTML)
	assert.Equal(t, input.Section6TextTh, after.Section6TextTh)
	assert.Equal(t, "2x", after.DeductionMultiplier)
	assert.Equal(t, "-", after.Separator)
	assert.Equal(t, 8, after.RunningNoPadding)
	assert.Equal(t, "CE4", after.YearFormat)
	assert.Equal(t, "RT", after.Prefix)
	assert.Equal(t, adminRow.ID.String(), after.UpdatedBy, "updated_by must be the caller-supplied users.id")
}

// TestSaveSettings_PartialFailureRollsBackBothWrites proves WR-07's fix
// (04-REVIEW.md): UpdateReceiptTemplateConfig and UpdateReceiptNumberConfig
// are written inside ONE transaction, so a failure on the SECOND write rolls
// back the FIRST too — no partial save.
//
// year_format has no app-level validation in SaveSettings (only
// separator/prefix go through numberFormatCharAllowlist) — its DB CHECK
// constraint (year_format IN ('BE4','CE4'), migrations/000004) is the only
// backstop, making it a convenient way to force the SECOND write
// (UpdateReceiptNumberConfig) to fail while the FIRST write
// (UpdateReceiptTemplateConfig) would otherwise have already succeeded.
func TestSaveSettings_PartialFailureRollsBackBothWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}
	t.Parallel()

	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)
	ctx := context.Background()

	adminRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:           "admin-settings-partial-fail@example.com",
		DisplayName:     "Admin Settings Partial Fail Test",
		KeycloakSubject: "admin-settings-partial-fail-subject",
	})
	require.NoError(t, err)

	svc := settings.NewSettingsService(pool, queries, nil, zap.NewNop())

	before, err := svc.GetSettings(ctx)
	require.NoError(t, err)

	input := validSettings()
	input.TemplateHTML = `<html><body>SHOULD NOT PERSIST {{.DonorName}}</body></html>`
	input.YearFormat = "XX99" // violates the DB CHECK constraint — forces the SECOND write to fail

	err = svc.SaveSettings(ctx, input, adminRow.ID)
	require.Error(t, err, "an invalid year_format must be rejected (via the DB CHECK constraint)")

	after, err := svc.GetSettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, before.TemplateHTML, after.TemplateHTML,
		"the template write must be rolled back — no partial save when the number-format write fails")
	assert.Equal(t, before.YearFormat, after.YearFormat,
		"year_format itself must be unchanged after the rejected save")
}
