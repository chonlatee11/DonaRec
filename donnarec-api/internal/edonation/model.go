// Package edonation — model.go
//
// Request/response Go structs for the e-Donation export slice (FR-30, plan 05-02).
// ExportRow is intentionally NOT the sqlc-generated SearchIssuedForExportRow — the
// service layer decrypts donor_tax_id_enc/dek into a plaintext NationalID here
// (D-64), matching the API/DB boundary discipline internal/donation/model.go
// documents ("never expose raw DB rows to callers").
package edonation

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

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

// KeyedRequest is the input DTO for Service.SetKeyed — bulk or per-row
// mark/unmark of "คีย์เข้า e-Donation แล้ว" (FR-31/D-67). DonationIDs are
// validated as well-formed UUID strings and converted to pgtype.UUID at the
// HANDLER boundary, BEFORE this DTO is ever constructed (T-05-04-SQLI:
// malformed IDs must 4xx before the ANY($1::uuid[]) query ever runs, never a
// 500). Keyed=true marks; Keyed=false unmarks (D-67).
type KeyedRequest struct {
	DonationIDs []pgtype.UUID
	Keyed       bool
}

// AgingRow is one unkeyed issued donation, bucketed by its e-Donation keying
// deadline (FR-31/D-68). ApprovedAt (never DonatedAt or an "issued_at" —
// donations has NO issued_at column) is the D-68 aging base date; Deadline is
// computeDeadline(ApprovedAt); Bucket is computeBucket(ApprovedAt, now,
// near_due_days from edonation_config).
type AgingRow struct {
	ID               string
	DonorName        string
	ReceiptFormatted string
	ApprovedAt       time.Time
	Deadline         time.Time
	Bucket           AgingBucket
	Keyed            bool
}

// AgingResult is the full aging view response: bucketed rows (ordered by
// ApprovedAt, oldest first — inherited from SearchUnkeyedIssued's ORDER BY)
// plus per-bucket counts (not_due/near_due/overdue) for summary cards.
type AgingResult struct {
	Rows   []AgingRow
	Counts map[AgingBucket]int
}
