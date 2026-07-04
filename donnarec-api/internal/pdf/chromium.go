// Package pdf — chromium.go wires the sandboxed remote-Chromium render pipeline (D-58).
//
// Design decisions realized here:
//
//	D-58: three-layer defense-in-depth against the admin-supplied-HTML template-injection
//	      / stored-XSS / SSRF surface:
//	        (1) network-isolated `chrome` sidecar service (docker-compose.yml, 04-02) —
//	            no outbound network route at all, at the network layer.
//	        (2) fetch.Enable (catch-all pattern) + FailRequest on every paused request —
//	            CDP-level: 100% of outbound requests are intercepted and failed before
//	            reaching the network, even if layer (1) were ever misconfigured.
//	        (3) emulation.SetScriptExecutionDisabled(true) — stops ALL JavaScript
//	            execution the rendered page might attempt to run (inline <script>,
//	            event handlers, javascript: URLs).
//	      All three verified live in 04-RESEARCH.md's research session; (2) and (3) are
//	      proven as permanent regression tests in render_sandbox_security_test.go
//	      (T-04-07, T-04-08).
//	D-58 (continued): page.SetDocumentContent injects the fully self-contained HTML
//	      string directly — Chromium is NEVER told to navigate to a caller-influenced
//	      URL, which would itself require network reachability from the render
//	      container and reopen exactly the SSRF surface layers (1)/(2) close
//	      (04-RESEARCH.md Pitfall 3).
//	chromedp is pinned to v0.14.2 in go.mod (NOT @latest, which resolves to v0.15.1 and
//	      requires Go 1.26 — 04-RESEARCH.md Pitfall 2 / 04-02-SUMMARY.md).
//
// renderInSandbox is the single, unexported code path that establishes this sandbox
// (network-block + JS-disable + document-content injection); RenderPDF is the only
// production caller, appending a final page.PrintToPDF action. render_sandbox_security_test.go
// (same package) calls renderInSandbox directly with different trailing actions
// (chromedp.Title, chromedp.Evaluate) so the regression tests exercise the EXACT same
// sandbox setup code production PDF rendering uses — there is only one place these
// mitigations are wired, not a test-only reimplementation of them.
package pdf

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// renderTimeout bounds a single render (RenderPDF or a renderInSandbox regression test)
// end-to-end, including the sandbox setup and the trailing action(s) (04-RESEARCH.md
// Pattern 2).
const renderTimeout = 30 * time.Second

// documentSettleDelay is a short pause after page.SetDocumentContent to let layout
// settle before the trailing action (PrintToPDF, or a title/DOM check in tests) runs.
// There is no JavaScript and no webfonts-over-network to await (D-58), so a fixed short
// sleep is sufficient rather than an event-based wait (04-RESEARCH.md Pattern 2).
const documentSettleDelay = 300 * time.Millisecond

// Renderer drives a remote headless-Chromium instance over the Chrome DevTools Protocol
// (CDP) to turn self-contained HTML into PDF bytes. Use NewRenderer to construct.
type Renderer struct {
	wsURL string
}

// NewRenderer constructs a Renderer targeting the given Chrome DevTools Protocol
// WebSocket endpoint (e.g. config.WorkerConfig.ChromeWSURL — "ws://chrome:9222" in
// Docker — or a testutil.StartChrome(t) URL in tests).
//
// Mirrors storage.NewStorageClient's package-prefixed error-wrap constructor style.
func NewRenderer(chromeWSURL string) (*Renderer, error) {
	if chromeWSURL == "" {
		return nil, fmt.Errorf("pdf: chromedp remote allocator init: empty chrome websocket URL")
	}
	return &Renderer{wsURL: chromeWSURL}, nil
}

// RenderPDF renders selfContainedHTML to PDF bytes via the sandboxed remote Chromium
// instance. selfContainedHTML MUST already have all images/fonts inlined as base64
// data: URIs (render.go's Render/DataURI/FontFaceCSS) — RenderPDF itself performs zero
// HTTP fetches and grants the page zero JavaScript execution (D-58), verified by
// render_sandbox_security_test.go.
func (r *Renderer) RenderPDF(ctx context.Context, selfContainedHTML string) ([]byte, error) {
	var pdfBuf []byte

	printAction := chromedp.ActionFunc(func(ctx context.Context) error {
		buf, _, err := page.PrintToPDF().WithPrintBackground(true).Do(ctx)
		pdfBuf = buf
		return err
	})

	if err := renderInSandbox(ctx, r.wsURL, selfContainedHTML, printAction); err != nil {
		return nil, fmt.Errorf("pdf: render pdf: %w", err)
	}

	return pdfBuf, nil
}

// renderInSandbox connects to the chrome sidecar at wsURL, establishes the D-58 render
// sandbox (network-block via fetch.Enable+FailRequest-all, JS-disable via
// emulation.SetScriptExecutionDisabled, document-content injection via
// page.SetDocumentContent — never a URL navigation), lets layout settle, then runs
// extraActions before the CDP context is torn down.
//
// This is the single code path both RenderPDF (production, trailing action =
// page.PrintToPDF) and render_sandbox_security_test.go's regression tests (trailing
// action = chromedp.Title / chromedp.Evaluate) go through — there is only one place the
// three D-58 mitigations are wired.
func renderInSandbox(ctx context.Context, wsURL, html string, extraActions ...chromedp.Action) error {
	allocCtx, cancel := chromedp.NewRemoteAllocator(ctx, wsURL)
	defer cancel()

	cctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()

	cctx, cancelTO := context.WithTimeout(cctx, renderTimeout)
	defer cancelTO()

	actions := []chromedp.Action{
		// Layer 2: block ALL outbound network — every request is paused, then failed,
		// before it ever reaches the network (T-04-08).
		fetch.Enable().WithPatterns([]*fetch.RequestPattern{{URLPattern: "*"}}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			chromedp.ListenTarget(ctx, func(ev interface{}) {
				if paused, ok := ev.(*fetch.EventRequestPaused); ok {
					go func() {
						_ = chromedp.Run(ctx, fetch.FailRequest(paused.RequestID, "Failed"))
					}()
				}
			})
			return nil
		}),
		// Layer 3: disable JavaScript execution entirely (T-04-07).
		emulation.SetScriptExecutionDisabled(true),
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			ft, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return err
			}
			// Inject HTML directly — Chromium never performs a navigation/fetch to
			// obtain the template (Layer 1 support: no legitimate reason for the
			// sidecar to need network at all).
			return page.SetDocumentContent(ft.Frame.ID, html).Do(ctx)
		}),
		chromedp.Sleep(documentSettleDelay),
	}
	actions = append(actions, extraActions...)

	return chromedp.Run(cctx, actions...)
}
