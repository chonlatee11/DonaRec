// Package pdf — sandbox security regression tests (D-58, Phase 4 Plan 03).
//
// Ports 04-RESEARCH.md's live-verified spike (JS-disable + network-block) into a
// permanent regression test so a future chromedp/CDP upgrade that silently weakens
// either mitigation fails CI loudly, rather than surfacing as a production incident.
//
// White-box (package pdf, not pdf_test): TestRenderSandboxSecurity_JSDisabled and
// TestRenderSandboxSecurity_NetworkBlocked call the unexported renderInSandbox helper
// directly so the test exercises the EXACT same CDP action sequence RenderPDF uses in
// production (fetch.Enable+FailRequest-all, emulation.SetScriptExecutionDisabled,
// page.SetDocumentContent) — there is only one place these three mitigations are wired,
// and this file is what proves it, per the threat register (T-04-07, T-04-08).
//
// References:
//
//	04-RESEARCH.md "Pattern 2: Sandboxed Chromium Render" (live-verified spike)
//	04-RESEARCH.md "Security Domain" — V12 File Handling (SSRF-adjacent)
//	04-03-PLAN.md <threat_model> T-04-07, T-04-08, T-04-09
package pdf

import (
	"context"
	"html/template"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/donnarec/donnarec-api/internal/testutil"
)

// TestRenderSandboxSecurity_JSDisabled proves T-04-07: an inline <script> in the
// rendered HTML must NOT execute. The fixture document has no <title> element, so if
// script execution were (mis)enabled, the inline script would overwrite document.title
// with a detectable marker string. With emulation.SetScriptExecutionDisabled(true) wired
// (renderInSandbox, chromium.go), the title must remain empty.
func TestRenderSandboxSecurity_JSDisabled(t *testing.T) {
	wsURL, cleanup := testutil.StartChrome(t)
	defer cleanup()

	html := `<!DOCTYPE html><html><head><meta charset="utf-8"></head><body>
<script>document.title = "XSS-EXECUTED-IF-YOU-SEE-THIS";</script>
</body></html>`

	var title string
	err := renderInSandbox(context.Background(), wsURL, zap.NewNop(), html,
		chromedp.Sleep(200*time.Millisecond),
		chromedp.Title(&title),
	)
	require.NoError(t, err, "sandboxed render must still succeed even with an inline <script> present")
	assert.Empty(t, title, "document.title must remain empty — inline <script> must never execute (T-04-07)")
}

// TestRenderSandboxSecurity_NetworkBlocked proves T-04-08: an external resource probe
// (simulating SSRF / tracking-pixel exfiltration via an admin-authored template) must
// never load. fetch.Enable + FailRequest-all (renderInSandbox, chromium.go) intercepts
// and fails every outbound request before it reaches the network, so the <img>'s
// naturalWidth must remain 0 and the render must still complete without error/timeout.
func TestRenderSandboxSecurity_NetworkBlocked(t *testing.T) {
	wsURL, cleanup := testutil.StartChrome(t)
	defer cleanup()

	html := `<!DOCTYPE html><html><head><meta charset="utf-8"></head><body>
<img id="probe" src="https://example.invalid/exfil-tracker.png?ssrf-probe">
</body></html>`

	var naturalWidth int
	err := renderInSandbox(context.Background(), wsURL, zap.NewNop(), html,
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Evaluate(`document.getElementById('probe').naturalWidth`, &naturalWidth),
	)
	require.NoError(t, err, "sandboxed render must still succeed even though the network probe is blocked")
	assert.Equal(t, 0, naturalWidth, "external image must never load — network must be fully blocked (T-04-08)")
}

// TestRenderSandboxSecurity_DonorFieldEscaped proves T-04-09: a donor-supplied field
// (untrusted end-user input, e.g. DonorName) that contains markup must be contextually
// escaped by html/template when assembled into the admin's (trusted-but-risky) template
// HTML — Render must NEVER wrap the whole admin template string in template.HTML(...),
// which would disable autoescaping entirely and defeat this protection.
//
// Does not require a live Chromium instance — this tests the pure HTML-assembly step.
func TestRenderSandboxSecurity_DonorFieldEscaped(t *testing.T) {
	tmpl := `<!DOCTYPE html><html><body><div class="donor">{{.DonorName}}</div></body></html>`

	data := ReceiptData{
		DonorName: `<script>alert('xss')</script>`,
		ReceiptNo: "2569/000001",
	}

	html, err := Render(tmpl, data)
	require.NoError(t, err)

	assert.NotContains(t, html, "<script>alert", "donor-supplied markup must not survive unescaped into the assembled HTML")
	assert.Contains(t, html, "&lt;script&gt;", "donor field must be HTML-entity-escaped by html/template's contextual autoescaping")
}

// TestDataURIAndFontFaceCSS is a lightweight unit test (no Chromium required) for the
// server-controlled asset-embedding helpers render.go provides: DataURI (used for
// letterhead/seal/signature/watermark images) and FontFaceCSS (TH Sarabun New, embedded
// once, server-controlled — never derived from donor or admin input).
func TestDataURIAndFontFaceCSS(t *testing.T) {
	uri := DataURI("image/png", []byte{0x89, 0x50, 0x4E, 0x47})
	assert.Equal(t, template.URL("data:image/png;base64,iVBORw=="), uri)

	css := FontFaceCSS("TH Sarabun New", "font/ttf", []byte{0x00, 0x01, 0x02, 0x03})
	assert.Contains(t, string(css), "@font-face")
	assert.Contains(t, string(css), "TH Sarabun New")
	assert.Contains(t, string(css), "data:font/ttf;base64,AAECAw==")
}
