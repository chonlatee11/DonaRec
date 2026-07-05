// Package pdf — golden-file visual-regression tests (SC#2, FR-20/21/22/23/24).
//
// Renders two fixed fixtures (Thai worst-case + English) through the full
// Render -> RenderPDF pipeline against the real, Thai-font-equipped chrome sidecar
// (testutil.StartChrome, 04-02), rasterizes the resulting PDF to PNG via the system
// `pdftoppm` binary, and compares byte-for-byte against a committed golden PNG.
//
// 04-RESEARCH.md verified this session that repeated renders of the same fixture
// through the pinned container produce byte-identical PNGs — so exact comparison is
// sufficient; no pixel-diff/tolerance library is used (see RESEARCH.md "Package
// Legitimacy Audit" — orisano/pixelmatch was evaluated and rejected).
//
// `go test ./internal/pdf/... -run TestRenderGolden -update` regenerates the goldens.
//
// TH Sarabun New is not yet sourced (assets/fonts/README.md, Assumption A3) — these
// fixtures deliberately rely on the fonts-thai-tlwg fallback (Waree) already baked into
// docker/chrome.Dockerfile, NOT a FontFaceCSS-embedded custom font, so the test suite
// does not depend on an unlicensed/unavailable asset. Thai shaping correctness (stacked
// tone marks) is what SC#2 requires; exact production typography is a tracked follow-up.
package pdf

import (
	"bytes"
	"context"
	"flag"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/donnarec/donnarec-api/internal/testutil"
)

var updateGolden = flag.Bool("update", false, "regenerate golden PNG fixtures instead of comparing against them")

// whitespaceRe matches any run of whitespace (spaces, tabs, newlines, form-feeds) —
// used to normalize pdftotext output before substring comparison (see
// assertContainsExtractedText).
var whitespaceRe = regexp.MustCompile(`\s+`)

// receiptFixtureTemplate is a self-contained receipt HTML template (own fixture, NOT
// the migration-seeded receipt_template_config.template_html — this file's fixtures
// exercise the full ReceiptData field surface, including FontFaceCSS/SealData, which
// the current DB seed does not yet reference).
const receiptFixtureTemplate = `<!DOCTYPE html>
<html lang="{{.Language}}">
<head>
<meta charset="UTF-8">
<style>
  body { font-family: 'Waree', 'Garuda', sans-serif; font-size: 16pt; position: relative; margin: 0; padding: 40px; }
  {{.FontFaceCSS}}
  .watermark { position: absolute; top: 35%; left: 15%; width: 50%; opacity: 0.12; z-index: -1; }
  .letterhead { width: 100%; max-height: 90px; margin-bottom: 12px; }
  .seal { width: 70px; }
  .signature { height: 60px; display: block; margin-top: 8px; }
  .field { margin: 4px 0; }
  .section6 { margin-top: 24px; font-size: 14pt; white-space: pre-wrap; }
</style>
</head>
<body>
  <img class="watermark" src="{{.WatermarkData}}" alt="">
  <img class="letterhead" src="{{.LetterheadData}}" alt="">
  <h1>{{.ReceiptNo}}</h1>
  <p class="field">{{.IssueDate}}</p>
  <p class="field">{{.DonorName}}</p>
  <p class="field">{{.Amount}}</p>
  <div class="section6">{{.Section6Text}}</div>
  <p class="field">{{.DeductionMultiplier}}</p>
  <img class="seal" src="{{.SealData}}" alt="">
  <img class="signature" src="{{.SignatureData}}" alt="">
</body>
</html>`

// thaiWorstCaseData returns fixed fixture data covering SC#2's required worst case:
// stacked tone marks (ก๊วยเตี๋ยว / ปั๊ม / ตั้งชื่อ) combined with Latin-leading mixed text.
func thaiWorstCaseData() ReceiptData {
	return ReceiptData{
		DonorName:           "iPhone ก๊วยเตี๋ยว ปั๊ม ตั้งชื่อ",
		ReceiptNo:           "2569/000001",
		Amount:              "1,000.00 บาท",
		IssueDate:           "4 กรกฎาคม 2569",
		Section6Text:        "เงินบริจาคนี้สามารถนำไปหักลดหย่อนภาษีเงินได้บุคคลธรรมดาได้ 2 เท่า ตามประมวลรัษฎากร มาตรา 47(7)",
		DeductionMultiplier: "2x",
		Language:            "th",
		LetterheadData:      DataURI("image/png", solidPNG(200, 60, 220, 230, 240)),
		SealData:            DataURI("image/png", solidPNG(70, 70, 200, 30, 30)),
		SignatureData:       DataURI("image/png", solidPNG(120, 50, 20, 20, 20)),
		WatermarkData:       DataURI("image/png", solidPNG(300, 300, 150, 150, 150)),
	}
}

// englishData returns fixed fixture data for the English receipt template variant.
func englishData() ReceiptData {
	return ReceiptData{
		DonorName:           "John Smith",
		ReceiptNo:           "2569/000002",
		Amount:              "1,000.00 THB",
		IssueDate:           "4 July 2026",
		Section6Text:        "This donation is eligible for a 2x tax deduction under the Revenue Code, Section 47(7).",
		DeductionMultiplier: "2x",
		Language:            "en",
		LetterheadData:      DataURI("image/png", solidPNG(200, 60, 220, 230, 240)),
		SealData:            DataURI("image/png", solidPNG(70, 70, 200, 30, 30)),
		SignatureData:       DataURI("image/png", solidPNG(120, 50, 20, 20, 20)),
		WatermarkData:       DataURI("image/png", solidPNG(300, 300, 150, 150, 150)),
	}
}

func TestRenderGolden_ThaiWorstCase(t *testing.T) {
	data := thaiWorstCaseData()
	pdfBytes := renderFixture(t, data)
	assertGoldenPNG(t, "testdata/thai_worst_case.golden.png", pdfBytes)
	assertContainsExtractedText(t, pdfBytes, data.Section6Text)
}

func TestRenderGolden_English(t *testing.T) {
	data := englishData()
	pdfBytes := renderFixture(t, data)
	assertGoldenPNG(t, "testdata/english.golden.png", pdfBytes)
	assertContainsExtractedText(t, pdfBytes, data.Section6Text)
}

// renderFixture assembles the fixture template + data via Render, then rasterizes it
// to PDF bytes via a Renderer dialed at a freshly-started chrome sidecar.
func renderFixture(t *testing.T, data ReceiptData) []byte {
	t.Helper()

	html, err := Render(receiptFixtureTemplate, data)
	require.NoError(t, err)

	wsURL, cleanup := testutil.StartChrome(t)
	defer cleanup()

	renderer, err := NewRenderer(wsURL)
	require.NoError(t, err)

	pdfBytes, err := renderer.RenderPDF(context.Background(), html)
	require.NoError(t, err)
	require.NotEmpty(t, pdfBytes)

	return pdfBytes
}

// assertGoldenPNG rasterizes pdfBytes via `pdftoppm` and compares the resulting PNG
// byte-for-byte against the committed golden fixture at goldenPath. Run with -update to
// (re)write the golden from the current render output.
func assertGoldenPNG(t *testing.T, goldenPath string, pdfBytes []byte) {
	t.Helper()

	pngBytes := rasterizeViaPdftoppm(t, pdfBytes)

	if *updateGolden {
		require.NoError(t, os.WriteFile(goldenPath, pngBytes, 0o644))
		return
	}

	golden, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "golden fixture missing — run with -update to generate it")

	if !bytes.Equal(golden, pngBytes) {
		actualPath := goldenPath[:len(goldenPath)-len(".golden.png")] + ".actual.png"
		_ = os.WriteFile(actualPath, pngBytes, 0o644)
		t.Fatalf("rendered PDF no longer matches golden PNG %s — see %s for the actual output", goldenPath, actualPath)
	}
}

// assertContainsExtractedText extracts text from pdfBytes via `pdftotext` and asserts
// the given substring (the fixture's §6 + deduction-multiplier statement, FR-24) is
// present in the extracted output. pdftotext hard-wraps long lines at the page width;
// for Thai text (which has no inter-word spaces) that wrap can land in the MIDDLE of a
// word with no whitespace inserted at all, so a single-space-collapse normalization is
// not sufficient. Both the extracted text and the wanted substring therefore have ALL
// whitespace stripped entirely before comparison — the assertion cares about textual
// content surviving the render, not pdftotext's incidental line-wrap points.
func assertContainsExtractedText(t *testing.T, pdfBytes []byte, want string) {
	t.Helper()

	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "receipt.pdf")
	require.NoError(t, os.WriteFile(pdfPath, pdfBytes, 0o644))

	out, err := exec.Command("pdftotext", "-enc", "UTF-8", pdfPath, "-").Output()
	require.NoError(t, err, "pdftotext extraction failed")

	got := whitespaceRe.ReplaceAllString(string(out), "")
	wantNormalized := whitespaceRe.ReplaceAllString(want, "")

	require.Contains(t, got, wantNormalized, "§6 + deduction-multiplier text must be present in the rendered PDF (FR-24)")
}

// solidPNG generates a deterministic, solid-color PNG of the given size — used as
// stand-in letterhead/seal/signature/watermark test fixture assets so this test suite
// does not depend on any external/binary image asset being present in the repo.
func solidPNG(w, h int, r, g, b uint8) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	c := color.RGBA{R: r, G: g, B: b, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err) // deterministic in-memory encode; cannot realistically fail
	}
	return buf.Bytes()
}

// rasterizeViaPdftoppm rasterizes the given PDF bytes to a single PNG via the system
// `pdftoppm` binary (poppler-utils) at 150 DPI. 04-RESEARCH.md verified this session
// that repeated renders of the same fixture through the pinned container + pinned fonts
// produce byte-identical PNG output, so no perceptual/tolerance diff is required.
func rasterizeViaPdftoppm(t *testing.T, pdfBytes []byte) []byte {
	t.Helper()

	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "receipt.pdf")
	require.NoError(t, os.WriteFile(pdfPath, pdfBytes, 0o644))

	outPrefix := filepath.Join(tmpDir, "receipt")
	cmd := exec.Command("pdftoppm", "-png", "-r", "150", "-singlefile", pdfPath, outPrefix)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "pdftoppm failed: %s", string(out))

	pngBytes, err := os.ReadFile(outPrefix + ".png")
	require.NoError(t, err)

	return pngBytes
}
