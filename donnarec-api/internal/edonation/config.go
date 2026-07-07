// Package edonation provides the config-driven e-Donation export field mapping
// (D-75), plus the shared read/write accessor for edonation_config
// (cash-type label D-65, aging threshold D-68). This is the substrate later
// export/keyed/aging/report slices (05-02+) build on — column order/names are
// NEVER hardcoded in Go, only ever derived from FieldMapping (05-01-PLAN.md
// prohibitions).
package edonation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/jackc/pgx/v5/pgtype"
)

// FieldMappingColumn is one ordered column of the e-Donation export: a stable
// identifier (ColumnKey, used to look up the actual value from an export row)
// plus its Thai/English display header (D-75).
type FieldMappingColumn struct {
	ColumnKey string `json:"column_key"`
	HeaderTh  string `json:"header_th"`
	HeaderEn  string `json:"header_en"`
}

// FieldMapping is the config-driven, ordered set of export columns (D-75) —
// the SINGLE source of column order and header text for every e-Donation
// export. Never hardcode column order/names in Go; always go through this
// type, sourced from edonation_config.field_mapping.
type FieldMapping struct {
	Columns []FieldMappingColumn
}

// defaultFieldMapping mirrors migration 000014_edonation_config.up.sql's
// seeded default column order — used ONLY as a Go-side fallback if
// field_mapping is ever an empty JSON array (e.g. an admin clears it via the
// settings UI), so an export can never silently produce a zero-column file.
// This is a fail-safe, not a hardcoded override: whenever a real mapping is
// present in the config, D-75 still governs and this fallback is never used.
func defaultFieldMapping() FieldMapping {
	return FieldMapping{Columns: []FieldMappingColumn{
		{ColumnKey: "national_id", HeaderTh: "เลขบัตรประชาชน/เลขผู้เสียภาษี", HeaderEn: "National ID"},
		{ColumnKey: "donated_at", HeaderTh: "วันที่บริจาค", HeaderEn: "Donation Date"},
		{ColumnKey: "cash_type", HeaderTh: "ประเภทการชำระเงิน", HeaderEn: "Cash Type"},
		{ColumnKey: "receipt_no", HeaderTh: "เลขที่ใบเสร็จ", HeaderEn: "Receipt No."},
		{ColumnKey: "donor_name", HeaderTh: "ชื่อผู้บริจาค", HeaderEn: "Donor Name"},
	}}
}

// DecodeFieldMapping parses the edonation_config.field_mapping JSONB column
// into a FieldMapping. Nil/empty bytes and an empty JSON array both fall back
// to defaultFieldMapping() (see its doc comment) — every other input is
// decoded and used exactly as configured (D-75).
func DecodeFieldMapping(raw []byte) (FieldMapping, error) {
	if len(raw) == 0 {
		return defaultFieldMapping(), nil
	}
	var cols []FieldMappingColumn
	if err := json.Unmarshal(raw, &cols); err != nil {
		return FieldMapping{}, fmt.Errorf("edonation: decode field_mapping: %w", err)
	}
	if len(cols) == 0 {
		return defaultFieldMapping(), nil
	}
	return FieldMapping{Columns: cols}, nil
}

// HeaderRow returns the ordered header labels for locale ("en" for English;
// anything else, including "", defaults to Thai) — the order is derived
// entirely from m.Columns (D-75), never hardcoded.
func (m FieldMapping) HeaderRow(locale string) []string {
	headers := make([]string, len(m.Columns))
	for i, c := range m.Columns {
		if locale == "en" {
			headers[i] = c.HeaderEn
		} else {
			headers[i] = c.HeaderTh
		}
	}
	return headers
}

// RowValues maps a column_key -> value row into the SAME config-driven column
// order HeaderRow uses (D-75). A plain map (rather than a concrete ExportRow
// type owned by a later plan, e.g. 05-02's export slice) keeps this substrate
// package free of a forward dependency on downstream DTOs — callers build the
// map from their own row type. Missing keys map to the empty string.
func (m FieldMapping) RowValues(row map[string]string) []string {
	values := make([]string, len(m.Columns))
	for i, c := range m.Columns {
		values[i] = row[c.ColumnKey]
	}
	return values
}

// Config is both the e-Donation export config value (field mapping,
// cash-type label D-65, aging threshold D-68) AND the accessor that reads/
// writes it from edonation_config — mirrors settings.SettingsService's
// single-row config-service shape, merged into one type per this plan's
// GetConfig(ctx) (Config, error) / UpdateConfig(ctx, Config, pgtype.UUID)
// error contract.
type Config struct {
	FieldMapping  FieldMapping
	CashTypeLabel string
	NearDueDays   int
	UpdatedAt     time.Time
	UpdatedBy     string

	queries *db.Queries
}

// NewConfig constructs a Config accessor. Panics if queries is nil — a
// programming-error guard, mirroring settings.NewSettingsService's
// constructor style.
func NewConfig(queries *db.Queries) *Config {
	if queries == nil {
		panic("edonation.NewConfig: queries must not be nil")
	}
	return &Config{queries: queries}
}

// GetConfig reads the single edonation_config row and decodes field_mapping
// into a typed FieldMapping (D-75).
func (c *Config) GetConfig(ctx context.Context) (Config, error) {
	row, err := c.queries.GetEdonationConfig(ctx)
	if err != nil {
		return Config{}, fmt.Errorf("edonation: get config: %w", err)
	}

	fm, err := DecodeFieldMapping(row.FieldMapping)
	if err != nil {
		return Config{}, err
	}

	return Config{
		FieldMapping:  fm,
		CashTypeLabel: row.CashTypeLabel,
		NearDueDays:   int(row.NearDueDays),
		UpdatedAt:     row.UpdatedAt.Time,
		UpdatedBy:     row.UpdatedBy.String(),
	}, nil
}

// UpdateConfig persists cfg's field mapping / cash-type label / near-due-days
// (Admin-only; RBAC gating happens at the handler/route level in a later
// plan). updatedBy MUST be the acting admin's resolved users.id
// (auth.ResolveAppUser), never the raw Keycloak subject — same discipline as
// settings.SaveSettings.
func (c *Config) UpdateConfig(ctx context.Context, cfg Config, updatedBy pgtype.UUID) error {
	mappingJSON, err := json.Marshal(cfg.FieldMapping.Columns)
	if err != nil {
		return fmt.Errorf("edonation: encode field_mapping: %w", err)
	}

	if err := c.queries.UpdateEdonationConfig(ctx, db.UpdateEdonationConfigParams{
		FieldMapping:  mappingJSON,
		CashTypeLabel: cfg.CashTypeLabel,
		NearDueDays:   int32(cfg.NearDueDays),
		UpdatedBy:     updatedBy,
	}); err != nil {
		return fmt.Errorf("edonation: update config: %w", err)
	}
	return nil
}
