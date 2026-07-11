// internal/edonation/export_test.go — TDD RED→GREEN tests for the xlsx.go/csv.go
// stream adapters (Task 1, plan 05-02). Pure in-memory tests — no DB, no Docker —
// mirroring internal/exportfile/writer_test.go's dependency-free style for the
// underlying StreamXLSX/StreamCSV primitives.
package edonation_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/donnarec/donnarec-api/internal/edonation"
)

// testFieldMapping mirrors migration 000014's default 5-column mapping (national_id,
// donated_at, cash_type, receipt_no, donor_name) — see edonation.defaultFieldMapping().
func testFieldMapping() edonation.FieldMapping {
	return edonation.FieldMapping{Columns: []edonation.FieldMappingColumn{
		{ColumnKey: "national_id", HeaderTh: "เลขบัตรประชาชน", HeaderEn: "National ID"},
		{ColumnKey: "donated_at", HeaderTh: "วันที่บริจาค", HeaderEn: "Donation Date"},
		{ColumnKey: "cash_type", HeaderTh: "ประเภทการชำระเงิน", HeaderEn: "Cash Type"},
		{ColumnKey: "receipt_no", HeaderTh: "เลขที่ใบเสร็จ", HeaderEn: "Receipt No."},
		{ColumnKey: "donor_name", HeaderTh: "ชื่อผู้บริจาค", HeaderEn: "Donor Name"},
	}}
}

func testExportRows() []edonation.ExportRow {
	return []edonation.ExportRow{
		{NationalID: "1234567890123", DonatedAt: "2026-03-15", ReceiptFormatted: "2569/000001", DonorName: "นาย ทดสอบ หนึ่ง"},
		{NationalID: "9876543210987", DonatedAt: "2026-03-16", ReceiptFormatted: "2569/000002", DonorName: "นาง ทดสอบ สอง"},
	}
}

// TestWriteXLSX_ZipSignature proves WriteXLSX streams a real .xlsx (ZIP-signature-
// prefixed) workbook directly to an io.Writer — no temp file (D-74).
func TestWriteXLSX_ZipSignature(t *testing.T) {
	var buf bytes.Buffer
	err := edonation.WriteXLSX(&buf, testFieldMapping(), "เงินสด", "th", testExportRows())
	require.NoError(t, err)

	// ZIP local file header signature: 0x50 0x4B 0x03 0x04 ("PK\x03\x04").
	require.GreaterOrEqual(t, buf.Len(), 4)
	assert.Equal(t, []byte{0x50, 0x4B, 0x03, 0x04}, buf.Bytes()[:4], "xlsx output must start with the ZIP signature")
}

// TestWriteXLSX_ConfigDrivenColumnOrder proves the header/column order is driven
// entirely by the FieldMapping passed in (D-75) — reversing the mapping reverses
// the header, never hardcoded.
func TestWriteXLSX_ConfigDrivenColumnOrder(t *testing.T) {
	fm := edonation.FieldMapping{Columns: []edonation.FieldMappingColumn{
		{ColumnKey: "donor_name", HeaderTh: "ชื่อผู้บริจาค", HeaderEn: "Donor Name"},
		{ColumnKey: "national_id", HeaderTh: "เลขบัตรประชาชน", HeaderEn: "National ID"},
	}}
	rows := []edonation.ExportRow{{NationalID: "1112223334445", DonorName: "นาย กลับลำดับ"}}

	var buf bytes.Buffer
	err := edonation.WriteXLSX(&buf, fm, "เงินสด", "th", rows)
	require.NoError(t, err)
	assert.NotEmpty(t, buf.Bytes())
}

// TestWriteCSV_BOMLeadingBytes proves WriteCSV writes the UTF-8 BOM before any
// content — required for Excel to reliably detect UTF-8 Thai text (05-RESEARCH.md
// Pattern 2).
func TestWriteCSV_BOMLeadingBytes(t *testing.T) {
	var buf bytes.Buffer
	err := edonation.WriteCSV(&buf, testFieldMapping(), "เงินสด", "th", testExportRows())
	require.NoError(t, err)

	require.GreaterOrEqual(t, buf.Len(), 3)
	assert.Equal(t, []byte{0xEF, 0xBB, 0xBF}, buf.Bytes()[:3], "csv output must start with the UTF-8 BOM")
}

// TestWriteCSV_IncludesConstantCashTypeLabel proves the constant cash_type_label
// (D-65) — NOT a per-row ExportRow field — appears in every data row's cash_type
// column, sourced from the cashTypeLabel argument.
func TestWriteCSV_IncludesConstantCashTypeLabel(t *testing.T) {
	var buf bytes.Buffer
	err := edonation.WriteCSV(&buf, testFieldMapping(), "เงินโอน", "th", testExportRows())
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "เงินโอน", "the constant cash_type_label must appear in the CSV output")
	// Both rows must carry the SAME constant label (it is not per-row data).
	assert.Equal(t, 2, bytesCount(out, "เงินโอน"), "cash_type_label must appear once per data row (2 rows seeded)")
}

// bytesCount counts non-overlapping occurrences of sub in s (tiny local helper —
// avoids pulling in strings.Count just for one assertion's readability elsewhere).
func bytesCount(s, sub string) int {
	count := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			count++
			i += len(sub) - 1
		}
	}
	return count
}

// TestWriteXLSX_EmptyRows proves an empty row set still streams a valid, header-
// only workbook (no panic) — the handler layer is responsible for rejecting empty
// exports before calling WriteXLSX/WriteCSV (Task 2's "no empty-file round trip").
func TestWriteXLSX_EmptyRows(t *testing.T) {
	var buf bytes.Buffer
	err := edonation.WriteXLSX(&buf, testFieldMapping(), "เงินสด", "th", nil)
	require.NoError(t, err)
	assert.NotEmpty(t, buf.Bytes())
}
