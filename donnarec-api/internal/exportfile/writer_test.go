// internal/exportfile/writer_test.go
// RED tests (Nyquist gate, TDD): written BEFORE writer.go exists.
// Verifies StreamXLSX/StreamCSV/SetDownloadHeaders behavior against the
// 05-01-PLAN.md <behavior> block — stream-only (io.Writer), no temp file,
// Thai text round-trips, BOM-prefixed CSV, RFC 5987 Content-Disposition.
package exportfile_test

import (
	"bytes"
	"encoding/csv"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/donnarec/donnarec-api/internal/exportfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

// zipLocalFileSignature is the 4-byte magic number every valid .xlsx (a zip
// container) begins with — "PK\x03\x04" (ZIP local file header signature).
var zipLocalFileSignature = []byte{'P', 'K', 0x03, 0x04}

// utf8BOM is the 3-byte UTF-8 byte-order-mark Excel needs at the start of a
// CSV file to reliably detect UTF-8 (and therefore render Thai text) instead
// of falling back to a legacy code page.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func TestStreamXLSX_ZipSignature(t *testing.T) {
	buf := &bytes.Buffer{}

	err := exportfile.StreamXLSX(buf, "e-Donation", []string{"col1", "col2"}, [][]string{{"a", "b"}})
	require.NoError(t, err)

	require.True(t, buf.Len() > len(zipLocalFileSignature), "buffer too small to contain a zip signature")
	assert.Equal(t, zipLocalFileSignature, buf.Bytes()[:4], "xlsx output must start with the ZIP local-file signature")
}

func TestStreamXLSX_ThaiRoundTrip(t *testing.T) {
	buf := &bytes.Buffer{}

	headers := []string{"เลขบัตรประชาชน/เลขผู้เสียภาษี", "ชื่อผู้บริจาค"}
	rows := [][]string{
		{"1-2345-67890-12-3", "นาย ตัวอย่าง ใจบุญ"},
	}

	err := exportfile.StreamXLSX(buf, "e-Donation", headers, rows)
	require.NoError(t, err)

	// Round-trip: open the bytes we just wrote back with excelize and assert
	// the Thai header/cell text is byte-identical to the input — proves no
	// mangling occurred anywhere in the write path.
	f, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	defer f.Close()

	sheet := f.GetSheetName(0)
	assert.Equal(t, "e-Donation", sheet)

	headerCell, err := f.GetCellValue(sheet, "A1")
	require.NoError(t, err)
	assert.Equal(t, headers[0], headerCell)

	headerCell2, err := f.GetCellValue(sheet, "B1")
	require.NoError(t, err)
	assert.Equal(t, headers[1], headerCell2)

	dataCell, err := f.GetCellValue(sheet, "A2")
	require.NoError(t, err)
	assert.Equal(t, rows[0][0], dataCell)

	dataCell2, err := f.GetCellValue(sheet, "B2")
	require.NoError(t, err)
	assert.Equal(t, rows[0][1], dataCell2)
}

func TestStreamXLSX_IOWriterOnly(t *testing.T) {
	// A plain bytes.Buffer (not backed by any filesystem path) proves
	// StreamXLSX needs nothing but an io.Writer — no os.Create/TempFile.
	var buf bytes.Buffer
	err := exportfile.StreamXLSX(&buf, "Sheet1", []string{"h"}, [][]string{{"v"}})
	require.NoError(t, err)
	assert.Positive(t, buf.Len())
}

func TestStreamCSV_BOMLeadingBytes(t *testing.T) {
	buf := &bytes.Buffer{}

	err := exportfile.StreamCSV(buf, []string{"col1", "col2"}, [][]string{{"a", "b"}})
	require.NoError(t, err)

	require.True(t, buf.Len() >= 3, "buffer too small to contain a BOM")
	assert.Equal(t, utf8BOM, buf.Bytes()[:3], "csv output must start with the UTF-8 BOM")
}

func TestStreamCSV_ThaiTextPresent(t *testing.T) {
	buf := &bytes.Buffer{}

	headers := []string{"ชื่อผู้บริจาค", "จำนวนเงิน"}
	rows := [][]string{
		{"นาย ตัวอย่าง ใจบุญ", "1,500.00"},
	}

	err := exportfile.StreamCSV(buf, headers, rows)
	require.NoError(t, err)

	// Strip the BOM, then parse as CSV and assert Thai text round-trips
	// verbatim through encoding/csv.
	body := bytes.TrimPrefix(buf.Bytes(), utf8BOM)
	r := csv.NewReader(bytes.NewReader(body))
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, headers, records[0])
	assert.Equal(t, rows[0], records[1])
}

func TestStreamCSV_IOWriterOnly(t *testing.T) {
	var buf bytes.Buffer
	err := exportfile.StreamCSV(&buf, []string{"h"}, [][]string{{"v"}})
	require.NoError(t, err)
	assert.Positive(t, buf.Len())
}

func TestSetDownloadHeaders(t *testing.T) {
	w := httptest.NewRecorder()

	exportfile.SetDownloadHeaders(w, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"edonation_export.xlsx", "ส่งออก_e-donation.xlsx")

	assert.Equal(t, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", w.Header().Get("Content-Type"))

	disposition := w.Header().Get("Content-Disposition")
	require.NotEmpty(t, disposition)
	assert.True(t, strings.Contains(disposition, `filename="edonation_export.xlsx"`),
		"Content-Disposition must include an ASCII-safe filename fallback: %s", disposition)
	assert.True(t, strings.Contains(disposition, "filename*=UTF-8''"+url.QueryEscape("ส่งออก_e-donation.xlsx")),
		"Content-Disposition must include the RFC 5987 extended UTF-8 filename*: %s", disposition)
}
