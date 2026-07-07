// Package edonation — csv.go
//
// WriteCSV mirrors xlsx.go's adapter shape for the secondary BOM-CSV export
// format — same config-driven column order (D-75), same constant cash_type_label
// merge (D-65), same stream-only discipline (D-74) via exportfile.StreamCSV.
package edonation

import (
	"io"

	"github.com/donnarec/donnarec-api/internal/exportfile"
)

// WriteCSV streams rows through fm's config-driven column order to w as a
// BOM-prefixed UTF-8 CSV (D-74: stream-only, no temp file).
func WriteCSV(w io.Writer, fm FieldMapping, cashTypeLabel, locale string, rows []ExportRow) error {
	headers, data := buildHeadersAndRows(fm, cashTypeLabel, locale, rows)
	return exportfile.StreamCSV(w, headers, data)
}
