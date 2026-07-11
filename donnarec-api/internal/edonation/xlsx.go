// Package edonation — xlsx.go
//
// WriteXLSX is a thin adapter: it maps []ExportRow through the config-driven
// FieldMapping column order (D-75) — including the constant cash_type_label
// (D-65), which is NOT a per-row field — into headers+[][]string, then delegates
// to exportfile.StreamXLSX for the actual in-memory workbook build + stream
// (D-74: no temp file). Deliberately does not import internal/db or hold any DB
// transaction — called by the handler AFTER Service.Export has already committed
// (05-RESEARCH.md Pitfall 3).
package edonation

import (
	"io"

	"github.com/donnarec/donnarec-api/internal/exportfile"
)

// rowToMap converts one ExportRow + the config's constant cash_type_label (D-65)
// into a column_key -> value map for FieldMapping.RowValues (D-75). Unknown
// column_keys configured by an admin simply resolve to "" (FieldMapping.RowValues'
// own missing-key behavior) — never a panic or a skipped row.
func rowToMap(row ExportRow, cashTypeLabel string) map[string]string {
	return map[string]string{
		"national_id": row.NationalID,
		"donated_at":  row.DonatedAt,
		"cash_type":   cashTypeLabel,
		"receipt_no":  row.ReceiptFormatted,
		"donor_name":  row.DonorName,
	}
}

// buildHeadersAndRows derives the header row and per-row values from fm's
// config-driven column order (D-75), shared by WriteXLSX and WriteCSV so both
// formats are byte-for-byte consistent in column order/content.
func buildHeadersAndRows(fm FieldMapping, cashTypeLabel, locale string, rows []ExportRow) ([]string, [][]string) {
	headers := fm.HeaderRow(locale)
	data := make([][]string, len(rows))
	for i, r := range rows {
		data[i] = fm.RowValues(rowToMap(r, cashTypeLabel))
	}
	return headers, data
}

// WriteXLSX streams rows through fm's config-driven column order to w as a
// single-sheet .xlsx workbook (D-74: stream-only, in-memory build — no temp file
// anywhere in this call chain).
func WriteXLSX(w io.Writer, fm FieldMapping, cashTypeLabel, locale string, rows []ExportRow) error {
	headers, data := buildHeadersAndRows(fm, cashTypeLabel, locale, rows)
	return exportfile.StreamXLSX(w, "e-Donation", headers, data)
}
