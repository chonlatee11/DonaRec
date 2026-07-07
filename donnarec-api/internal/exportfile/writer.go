// Package exportfile provides stream-only .xlsx/.csv writers shared by every
// Phase 5 export/report endpoint (FR-30/FR-32, D-74).
//
// Both writers accept ONLY an io.Writer — never a filesystem path — so
// plaintext-PII export data is never written to disk anywhere in the process
// (05-RESEARCH.md Pattern 1/2, D-74). Callers stream directly to the HTTP
// http.ResponseWriter after any DB transaction has already committed
// (05-RESEARCH.md Pitfall 3: never hold a DB tx open across a slow workbook
// build).
package exportfile

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/xuri/excelize/v2"
)

// utf8BOM is the 3-byte UTF-8 byte-order-mark written before any CSV content
// so Excel reliably detects UTF-8 (and therefore renders Thai text) instead
// of guessing a legacy code page (05-RESEARCH.md Pattern 2).
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// StreamXLSX builds a single-sheet workbook entirely in memory
// (excelize.NewFile + SetCellValue) and writes it directly to w — no on-disk
// temp file is ever created anywhere in this function (D-74). headers becomes row 1;
// each entry in rows becomes one subsequent row, in order. Thai text is
// handled natively by excelize (UTF-8 throughout OOXML), so header/cell
// values round-trip verbatim.
func StreamXLSX(w io.Writer, sheetName string, headers []string, rows [][]string) error {
	f := excelize.NewFile()
	defer f.Close() //nolint:errcheck // best-effort cleanup of the in-memory workbook

	const defaultSheet = "Sheet1"
	if sheetName == "" {
		sheetName = defaultSheet
	}
	if sheetName != defaultSheet {
		if err := f.SetSheetName(defaultSheet, sheetName); err != nil {
			return fmt.Errorf("exportfile: rename sheet: %w", err)
		}
	}

	if len(headers) > 0 {
		if err := f.SetSheetRow(sheetName, "A1", &headers); err != nil {
			return fmt.Errorf("exportfile: write header row: %w", err)
		}
	}

	for i, row := range rows {
		cellRef, err := excelize.CoordinatesToCellName(1, i+2) // +2: row 1 is the header
		if err != nil {
			return fmt.Errorf("exportfile: compute cell reference: %w", err)
		}
		rowCopy := row
		if err := f.SetSheetRow(sheetName, cellRef, &rowCopy); err != nil {
			return fmt.Errorf("exportfile: write data row %d: %w", i, err)
		}
	}

	if err := f.Write(w); err != nil {
		return fmt.Errorf("exportfile: stream xlsx: %w", err)
	}
	return nil
}

// StreamCSV writes the UTF-8 BOM, then a header row, then one row per entry
// in rows, directly to w via encoding/csv — no filesystem path is ever
// touched (D-74). The BOM MUST be written before csv.NewWriter touches w
// (05-RESEARCH.md Pattern 2) — this is unrelated to encoding/csv's Reader-side
// BOM-handling behavior (golang/go#33887), which only concerns parsing.
func StreamCSV(w io.Writer, headers []string, rows [][]string) error {
	if _, err := w.Write(utf8BOM); err != nil {
		return fmt.Errorf("exportfile: write BOM: %w", err)
	}

	cw := csv.NewWriter(w)
	if len(headers) > 0 {
		if err := cw.Write(headers); err != nil {
			return fmt.Errorf("exportfile: write csv header: %w", err)
		}
	}
	for i, row := range rows {
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("exportfile: write csv row %d: %w", i, err)
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("exportfile: flush csv: %w", err)
	}
	return nil
}

// SetDownloadHeaders sets Content-Type and a Content-Disposition header that
// carries BOTH an ASCII-safe filename="<asciiName>" fallback AND the RFC
// 5987/8187 extended filename*=UTF-8”<percent-encoded utf8Name> parameter —
// raw Thai bytes inside a bare filename="..." parameter are not valid per
// RFC 6266/2616 grammar (05-RESEARCH.md Pitfall 2). Every modern browser
// prefers filename* when present; older/non-browser HTTP clients fall back
// to the ASCII filename.
func SetDownloadHeaders(w http.ResponseWriter, contentType, asciiName, utf8Name string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, asciiName, url.QueryEscape(utf8Name)))
}
