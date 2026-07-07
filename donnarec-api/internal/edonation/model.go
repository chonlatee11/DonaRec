// Package edonation — model.go
//
// Request/response Go structs for the e-Donation export slice (FR-30, plan 05-02).
// ExportRow is intentionally NOT the sqlc-generated SearchIssuedForExportRow — the
// service layer decrypts donor_tax_id_enc/dek into a plaintext NationalID here
// (D-64), matching the API/DB boundary discipline internal/donation/model.go
// documents ("never expose raw DB rows to callers").
package edonation

import "time"

// ExportFilter narrows the export source to an optional donated_at date range and/or
// an optional keyed-status boolean (D-66). All fields are optional — nil/empty means
// "no filter", mirroring donation.ListFilter's D-53 discipline. Format is validated
// against an allowlist (xlsx|csv) at the handler boundary, not here.
type ExportFilter struct {
	From        *time.Time // inclusive lower bound on donated_at
	To          *time.Time // inclusive upper bound on donated_at
	KeyedStatus *bool      // nil = both keyed and unkeyed; true/false = exact match
	Format      string     // "xlsx" | "csv" — handler-validated, informational here
}

// ExportRow is one decrypted, export-ready donation row.
//
// D-64 (load-bearing): NationalID holds the full plaintext 13-digit donor tax/
// national ID. It is populated by Service.Export ONLY after the audited decrypt +
// committed summary audit row — never before, never unaudited (T-05-02-UNAUDITED).
// cash_type_label is NOT a field here — it is a constant sourced from
// edonation_config (D-65) and merged in by xlsx.go/csv.go at write time, not
// per-row data.
type ExportRow struct {
	NationalID       string // decrypted 13-digit donor tax/national ID
	DonatedAt        string // "YYYY-MM-DD"
	ReceiptFormatted string
	DonorName        string
}
