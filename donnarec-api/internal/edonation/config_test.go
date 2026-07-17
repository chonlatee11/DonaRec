// internal/edonation/config_test.go — tests for the Config accessor (Phase 5, plan 05-01, Task 2).
//
// Pure-logic tests (JSONB decode, HeaderRow ordering, default-mapping fallback) use no
// database — mirrors internal/settings/service_test.go's dependency-free style.
// The GetConfig/UpdateConfig round-trip tests use a real Postgres testcontainer
// (testutil.SetupTestPostgres), mirroring internal/settings/service_test.go's
// `testing.Short()`-gated integration shape.
package edonation_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/edonation"
	"github.com/donnarec/donnarec-api/internal/testutil"
)

// TestConfig_DecodeFieldMapping_JSONB proves a real edonation_config.field_mapping JSONB
// payload decodes into an ordered FieldMapping with column_key/header_th/header_en intact.
func TestConfig_DecodeFieldMapping_JSONB(t *testing.T) {
	t.Parallel()

	raw := []byte(`[
		{"column_key": "national_id", "header_th": "เลขบัตรประชาชน", "header_en": "National ID"},
		{"column_key": "donor_name",  "header_th": "ชื่อผู้บริจาค",   "header_en": "Donor Name"}
	]`)

	fm, err := edonation.DecodeFieldMapping(raw)
	require.NoError(t, err)
	require.Len(t, fm.Columns, 2)

	assert.Equal(t, "national_id", fm.Columns[0].ColumnKey)
	assert.Equal(t, "เลขบัตรประชาชน", fm.Columns[0].HeaderTh)
	assert.Equal(t, "National ID", fm.Columns[0].HeaderEn)

	assert.Equal(t, "donor_name", fm.Columns[1].ColumnKey)
	assert.Equal(t, "ชื่อผู้บริจาค", fm.Columns[1].HeaderTh)
	assert.Equal(t, "Donor Name", fm.Columns[1].HeaderEn)
}

// TestConfig_HeaderRow_Ordering proves HeaderRow is driven entirely by the FieldMapping's
// column order — reordering the underlying columns reorders the output, per locale.
func TestConfig_HeaderRow_Ordering(t *testing.T) {
	t.Parallel()

	fm := edonation.FieldMapping{Columns: []edonation.FieldMappingColumn{
		{ColumnKey: "a", HeaderTh: "หนึ่ง", HeaderEn: "One"},
		{ColumnKey: "b", HeaderTh: "สอง", HeaderEn: "Two"},
		{ColumnKey: "c", HeaderTh: "สาม", HeaderEn: "Three"},
	}}

	assert.Equal(t, []string{"หนึ่ง", "สอง", "สาม"}, fm.HeaderRow("th"))
	assert.Equal(t, []string{"One", "Two", "Three"}, fm.HeaderRow("en"))

	// Reversed mapping must reverse the header row too — proves ordering is
	// derived purely from Columns, not any hardcoded order.
	reversed := edonation.FieldMapping{Columns: []edonation.FieldMappingColumn{
		fm.Columns[2], fm.Columns[1], fm.Columns[0],
	}}
	assert.Equal(t, []string{"Three", "Two", "One"}, reversed.HeaderRow("en"))
}

// TestConfig_RowValues_FollowsColumnOrder proves RowValues maps a column_key->value row
// into the SAME order as HeaderRow, for a given FieldMapping.
func TestConfig_RowValues_FollowsColumnOrder(t *testing.T) {
	t.Parallel()

	fm := edonation.FieldMapping{Columns: []edonation.FieldMappingColumn{
		{ColumnKey: "receipt_no", HeaderTh: "เลขที่ใบเสร็จ", HeaderEn: "Receipt No."},
		{ColumnKey: "donor_name", HeaderTh: "ชื่อผู้บริจาค", HeaderEn: "Donor Name"},
	}}

	row := map[string]string{
		"donor_name": "นาย ตัวอย่าง ใจบุญ",
		"receipt_no": "2569/000001",
		"unused_key": "should not appear",
	}

	assert.Equal(t, []string{"2569/000001", "นาย ตัวอย่าง ใจบุญ"}, fm.RowValues(row))
}

// TestConfig_DecodeFieldMapping_EmptyFallsBackToDefault proves an empty JSONB array (or nil
// bytes) falls back to a non-empty default mapping — export must never silently produce a
// zero-column file even if an admin clears the config.
func TestConfig_DecodeFieldMapping_EmptyFallsBackToDefault(t *testing.T) {
	t.Parallel()

	t.Run("empty_array", func(t *testing.T) {
		fm, err := edonation.DecodeFieldMapping([]byte(`[]`))
		require.NoError(t, err)
		assert.NotEmpty(t, fm.Columns)
	})

	t.Run("nil_bytes", func(t *testing.T) {
		fm, err := edonation.DecodeFieldMapping(nil)
		require.NoError(t, err)
		assert.NotEmpty(t, fm.Columns)
	})
}

// TestConfig_GetConfig_RoundTrip proves GetConfig reads the 000014-seeded edonation_config
// row: near_due_days default 3 and a non-empty FieldMapping (the migration's seeded default
// column set), against a real Postgres instance.
func TestConfig_GetConfig_RoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}
	t.Parallel()

	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)
	ctx := context.Background()

	cfgAccessor := edonation.NewConfig(queries)

	cfg, err := cfgAccessor.GetConfig(ctx)
	require.NoError(t, err)

	assert.Equal(t, 3, cfg.NearDueDays, "migration 000014 seeds near_due_days default 3")
	assert.NotEmpty(t, cfg.FieldMapping.Columns, "migration 000014 seeds a non-empty default field_mapping")
	assert.NotEmpty(t, cfg.CashTypeLabel)
}

// TestConfig_UpdateConfig_ReordersHeaders proves FieldMapping.HeaderRow ordering is driven
// by the JSONB config end-to-end: after UpdateConfig persists a reordered mapping, a fresh
// GetConfig observes the new header order.
func TestConfig_UpdateConfig_ReordersHeaders(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration test in short mode")
	}
	t.Parallel()

	pool := testutil.SetupTestPostgres(t)
	queries := db.New(pool)
	ctx := context.Background()

	adminRow, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:           "admin-edonation-config-test@example.com",
		DisplayName:     "Admin e-Donation Config Test",
		KeycloakSubject: "admin-edonation-config-test-subject",
	})
	require.NoError(t, err)

	cfgAccessor := edonation.NewConfig(queries)

	newMapping := edonation.FieldMapping{Columns: []edonation.FieldMappingColumn{
		{ColumnKey: "donor_name", HeaderTh: "ชื่อผู้บริจาค", HeaderEn: "Donor Name"},
		{ColumnKey: "national_id", HeaderTh: "เลขบัตรประชาชน", HeaderEn: "National ID"},
	}}

	err = cfgAccessor.UpdateConfig(ctx, edonation.Config{
		FieldMapping:  newMapping,
		CashTypeLabel: "เงินสด",
		NearDueDays:   5,
	}, adminRow.ID)
	require.NoError(t, err)

	after, err := cfgAccessor.GetConfig(ctx)
	require.NoError(t, err)

	assert.Equal(t, []string{"Donor Name", "National ID"}, after.FieldMapping.HeaderRow("en"))
	assert.Equal(t, "เงินสด", after.CashTypeLabel)
	assert.Equal(t, 5, after.NearDueDays)
	assert.Equal(t, adminRow.ID.String(), after.UpdatedBy)
}
