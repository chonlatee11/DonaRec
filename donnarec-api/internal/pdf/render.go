// Package pdf renders donation receipt PDFs from admin-configurable HTML templates
// (receipt_template_config.template_html/template_html_en) via a sandboxed remote
// headless-Chromium instance (chromium.go, docker/chrome.Dockerfile, 04-02).
//
// Design decisions realized here:
//
//	D-58: the admin's stored template HTML is trusted-but-risky (XSS/SSRF axis, since it
//	      is executed by a real browser engine — mitigated in chromium.go's render
//	      sandbox), while donor-supplied field data substituted into it (DonorName, etc.)
//	      is untrusted end-user input on a DIFFERENT axis (markup-injection) — Render
//	      protects against that second axis via Go's stdlib html/template contextual
//	      autoescaping. The admin template string is parsed and executed AS-IS; it is
//	      NEVER wrapped in template.HTML(...), which would disable autoescaping entirely
//	      and defeat the one layer that protects against donor-field injection (T-04-09).
//	D-56: LetterheadData/SealData/SignatureData/WatermarkData are pre-built,
//	      server-controlled base64 data: URIs (see DataURI) — the Go app fetches the
//	      underlying image bytes from MinIO itself (network access it legitimately has);
//	      Chromium never fetches them, closing an SSRF surface at the source rather than
//	      relying solely on the render sandbox to catch it (04-RESEARCH.md Pitfall 3).
//	FontFaceCSS embeds the TH Sarabun New font (once sourced — assets/fonts/README.md)
//	      as a base64 @font-face data: URI, injected once per render, never derived from
//	      donor or admin input.
//
// LetterheadData/SealData/SignatureData/WatermarkData are typed template.URL, and
// FontFaceCSS is typed template.CSS, so html/template treats them as pre-vetted, safe
// content in their respective (URL / CSS) contexts instead of applying content-type
// sanitization that would otherwise mangle a legitimate data: URI into "#ZgotmplZ".
// This is safe ONLY because these values are assembled by this package from
// server-controlled bytes — never from donor or admin free-text input.
package pdf

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
)

// ReceiptData is the template data structure supplied to Render. Field names match the
// placeholders already used by the seeded receipt_template_config templates
// (migrations/000011_receipt_template_config.up.sql): {{.WatermarkData}},
// {{.LetterheadData}}, {{.ReceiptNo}}, {{.IssueDate}}, {{.DonorName}}, {{.Amount}},
// {{.Section6Text}}, {{.SignatureData}}.
type ReceiptData struct {
	// DonorName is untrusted end-user input — autoescaped by html/template.
	DonorName string
	// ReceiptNo is the frozen, formatted receipt number (D-42 snapshot).
	ReceiptNo string
	// Amount is the frozen donation amount, pre-formatted for display.
	Amount string
	// IssueDate is the frozen issue date, pre-formatted for display.
	IssueDate string
	// Section6Text is the tax-deduction wording (FR-24), admin-configured per language.
	Section6Text string
	// DeductionMultiplier is the global 1x/2x election (D-59), e.g. "1x" or "2x".
	DeductionMultiplier string
	// Language is the frozen donor_language ("th" or "en", D-55).
	Language string

	// LetterheadData is a pre-built base64 data: URI (see DataURI) — server-controlled,
	// safe for the URL context without triggering html/template's URL sanitizer.
	LetterheadData template.URL
	// SealData is a pre-built base64 data: URI for the hospital seal image.
	SealData template.URL
	// SignatureData is a pre-built base64 data: URI for the authorized signature image.
	SignatureData template.URL
	// WatermarkData is a pre-built base64 data: URI for the watermark overlay image.
	WatermarkData template.URL

	// FontFaceCSS is a pre-built @font-face CSS block (see FontFaceCSS) embedding the
	// TH Sarabun New font as a base64 data: URI — server-controlled, safe for the CSS
	// context without triggering html/template's CSS sanitizer.
	FontFaceCSS template.CSS
}

// Render executes templateHTML (the admin's stored, trusted-but-risky template string)
// against data using Go's stdlib html/template, whose contextual autoescaping
// automatically neutralizes donor-supplied field injection (T-04-09) based on where each
// field appears in the markup (text node, attribute, URL, CSS, etc.).
//
// templateHTML is parsed and executed exactly as received — it must NEVER be pre-wrapped
// in template.HTML(...) by a caller, which would disable autoescaping entirely.
//
// The returned string is a complete, self-contained HTML document ready to be handed to
// (*Renderer).RenderPDF via page.SetDocumentContent — it contains no external resource
// references (all images are already-inlined data: URIs) and requires no network access
// to render correctly.
func Render(templateHTML string, data ReceiptData) (string, error) {
	tmpl, err := template.New("receipt").Parse(templateHTML)
	if err != nil {
		return "", fmt.Errorf("pdf: parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("pdf: execute template: %w", err)
	}

	return buf.String(), nil
}

// DataURI encodes raw image bytes as a base64 data: URI with the given MIME type (e.g.
// "image/png"). Used to inline caller-supplied letterhead/seal/signature/watermark
// images directly into the assembled HTML so Chromium never needs to fetch them over
// the network (04-RESEARCH.md Pitfall 3 — closes an SSRF surface at the source, not just
// at the render sandbox).
//
// Returned as template.URL (not a plain string) so html/template treats it as pre-vetted
// safe content in a URL context (e.g. <img src="{{...}}">) instead of rejecting it as
// "#ZgotmplZ" the way it would an arbitrary untyped string.
func DataURI(mimeType string, data []byte) template.URL {
	return template.URL(fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data)))
}

// FontFaceCSS assembles a CSS @font-face block embedding fontData as a base64 data: URI
// under the given fontFamily name. Intended for the TH Sarabun New font (once sourced,
// assets/fonts/README.md) — the identical font bytes are embedded here AND (per D-61)
// referenced by the admin settings live-preview iframe, so both stay visually
// consistent. This is a server-controlled asset, never derived from donor or admin
// free-text input.
//
// Returned as template.CSS (not a plain string) so html/template treats it as pre-vetted
// safe content in a CSS context (e.g. <style>{{...}}</style>) instead of rejecting it.
func FontFaceCSS(fontFamily, mimeType string, fontData []byte) template.CSS {
	dataURI := DataURI(mimeType, fontData)
	return template.CSS(fmt.Sprintf(
		`@font-face { font-family: '%s'; src: url(%s) format('truetype'); }`,
		fontFamily, dataURI,
	))
}
