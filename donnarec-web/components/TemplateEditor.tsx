"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Loader2 } from "lucide-react";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { debounce } from "@/lib/debounce";
import { createLatestGuard } from "@/lib/latest-response";
import {
  buildPreviewRequest,
  fetchPreviewHTML,
  fetchPreviewPDFBlob,
  settingsErrorMessage,
  type PreviewRequest,
  type SettingsFormValues,
} from "@/lib/settings";

// ---------------------------------------------------------------------------
// TemplateEditorFields — Tab 1 left-column content: the two HTML textareas
// (th/en). UI-SPEC Screen 6 Tab 1: font-mono 14px/400/line-height 1.6, min
// height 480px, helper text listing available template variables.
// ---------------------------------------------------------------------------

interface TemplateEditorFieldsProps {
  templateHtml: string;
  onTemplateHtmlChange: (value: string) => void;
  templateHtmlEn: string;
  onTemplateHtmlEnChange: (value: string) => void;
  disabled?: boolean;
}

export function TemplateEditorFields({
  templateHtml,
  onTemplateHtmlChange,
  templateHtmlEn,
  onTemplateHtmlEnChange,
  disabled,
}: TemplateEditorFieldsProps) {
  const t = useTranslations("settings.template");

  const monoTextareaClass =
    "min-h-[480px] font-mono text-[14px] leading-[1.6] whitespace-pre-wrap";

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-2">
        <Label htmlFor="template-html-th">{t("labelTh")}</Label>
        <p id="template-html-helper" className="text-[14px] text-slate-600">
          {t("helperText")}
        </p>
        <Textarea
          id="template-html-th"
          aria-describedby="template-html-helper"
          className={monoTextareaClass}
          value={templateHtml}
          onChange={(e) => onTemplateHtmlChange(e.target.value)}
          disabled={disabled}
          spellCheck={false}
        />
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="template-html-en">{t("labelEn")}</Label>
        <Textarea
          id="template-html-en"
          aria-describedby="template-html-helper"
          className={monoTextareaClass}
          value={templateHtmlEn}
          onChange={(e) => onTemplateHtmlEnChange(e.target.value)}
          disabled={disabled}
          spellCheck={false}
        />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// TemplateLivePreview — persistent right-column preview pane (visible
// regardless of which of the four tabs is active, UI-SPEC Screen 6: "the
// right live-preview pane always renders the full receipt using current
// in-memory edits + all other tabs' last-saved values").
//
// T-04-24 mitigation: the iframe uses sandbox="allow-same-origin" ONLY (no
// allow-scripts, no allow-popups) — content is injected via the `srcDoc`
// attribute (no script execution required to apply CSS/markup). T-04-26
// mitigation: preview always renders sample/mock data via
// buildPreviewRequest/fetchPreviewHTML — no donation id or donor field is
// ever constructed or sent from this component.
// ---------------------------------------------------------------------------

interface TemplateLivePreviewProps {
  formValues: SettingsFormValues;
  disabled?: boolean;
}

/**
 * TH Sarabun New @font-face injected into the sandboxed preview (D-61,
 * 04-UI-SPEC.md "New font requirement"). Falls back to the Google-Fonts
 * "Sarabun" family (already used for app chrome, see app/layout.tsx) when the
 * licensed .woff2 file has not been sourced yet at public/fonts/ (see
 * public/fonts/README.md — same open item as donnarec-api/assets/fonts/README.md).
 */
const PREVIEW_FONT_FACE = `<style>
  @font-face {
    font-family: 'THSarabunNewPreview';
    src: url('/fonts/THSarabunNew.woff2') format('woff2');
    font-weight: normal;
    font-style: normal;
  }
  body, * { font-family: 'THSarabunNewPreview', 'Sarabun', sans-serif; }
</style>`;

/**
 * FW-02: restrictive CSP enforced inside the sandboxed preview document. The
 * iframe uses `sandbox="allow-same-origin"` with no `allow-scripts` (JS — the
 * primary XSS vector — is already dead), but passive sub-resources
 * (`<img>`/`<link>`/CSS `url()`) could still issue same-origin GETs carrying
 * the admin's cookies. This closes the D-58 "no network" half:
 *   - default-src 'none'          — block everything not explicitly allowed
 *   - img-src data:               — inline images only, no network fetches
 *   - style-src 'unsafe-inline' 'self' — the injected <style> + same-origin CSS
 *   - font-src 'self' data:       — keep TH Sarabun (/fonts/*.woff2) resolving
 */
const PREVIEW_CSP_META = `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline' 'self'; font-src 'self' data:">`;

const PREVIEW_HEAD_INJECTION = PREVIEW_CSP_META + PREVIEW_FONT_FACE;

/**
 * Injects the CSP meta + preview font-face block right after <head> (or
 * prepends it if no <head> tag is present). The CSP <meta> is placed first so
 * it applies before any resource-loading markup in the document.
 */
function withPreviewFontFace(html: string): string {
  const headIndex = html.toLowerCase().indexOf("<head>");
  if (headIndex !== -1) {
    return (
      html.slice(0, headIndex + "<head>".length) +
      PREVIEW_HEAD_INJECTION +
      html.slice(headIndex + "<head>".length)
    );
  }
  return PREVIEW_HEAD_INJECTION + html;
}

export function TemplateLivePreview({ formValues, disabled }: TemplateLivePreviewProps) {
  const t = useTranslations("settings.template");

  const [language, setLanguage] = useState<"th" | "en">("th");
  const [mode, setMode] = useState<"html" | "pdf">("html");

  const [html, setHtml] = useState<string>("");
  const [htmlLoading, setHtmlLoading] = useState(false);
  const [htmlError, setHtmlError] = useState<string | null>(null);
  const [hasFetchedOnce, setHasFetchedOnce] = useState(false);

  const [pdfBlobUrl, setPdfBlobUrl] = useState<string | null>(null);
  const [pdfLoading, setPdfLoading] = useState(false);
  const [pdfError, setPdfError] = useState<string | null>(null);

  const debouncedFetchRef = useRef<
    (((req: PreviewRequest) => void) & { cancel: () => void }) | null
  >(null);

  // WR-04 fix (04-REVIEW.md): guards against an out-of-order response — the
  // network gives no guarantee responses resolve in request order, so
  // without this an OLDER in-flight fetch that resolves LAST could silently
  // overwrite a newer, still-current preview with stale HTML.
  const previewGuardRef = useRef(createLatestGuard());

  // Stable dependency key so the debounced fetch re-fires only when a value
  // that actually affects the rendered receipt changes (D-61 "no heavy
  // re-render every keystroke" — the debounce utility itself coalesces
  // rapid changes within 400ms).
  const payloadKey = JSON.stringify({
    template_html: formValues.template_html,
    template_html_en: formValues.template_html_en,
    section6_text_th: formValues.section6_text_th,
    section6_text_en: formValues.section6_text_en,
    deduction_multiplier: formValues.deduction_multiplier,
    letterhead_object_key: formValues.letterhead_object_key,
    seal_object_key: formValues.seal_object_key,
    signature_object_key: formValues.signature_object_key,
    watermark_object_key: formValues.watermark_object_key,
    language,
  });

  useEffect(() => {
    if (!debouncedFetchRef.current) {
      debouncedFetchRef.current = debounce((req: PreviewRequest) => {
        const requestId = previewGuardRef.current.next();
        setHtmlLoading(true);
        setHtmlError(null);
        fetchPreviewHTML(req)
          .then((result) => {
            if (!previewGuardRef.current.isCurrent(requestId)) return; // stale — a newer request superseded this one
            setHtml(result);
            setHasFetchedOnce(true);
          })
          .catch((err) => {
            if (!previewGuardRef.current.isCurrent(requestId)) return;
            setHtmlError(settingsErrorMessage(err));
          })
          .finally(() => {
            if (previewGuardRef.current.isCurrent(requestId)) setHtmlLoading(false);
          });
      }, 400);
    }
    return () => {
      debouncedFetchRef.current?.cancel();
    };
    // Only set up the debounced function + cleanup once — setState setters
    // are referentially stable, so there is nothing else to depend on here.
  }, []);

  useEffect(() => {
    if (disabled) return;
    debouncedFetchRef.current?.(buildPreviewRequest(formValues, language));
    // payloadKey captures every value that should re-trigger the debounce.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [payloadKey, disabled]);

  // Revoke the previous PDF object URL on unmount / replacement to avoid leaks.
  useEffect(() => {
    return () => {
      if (pdfBlobUrl) URL.revokeObjectURL(pdfBlobUrl);
    };
  }, [pdfBlobUrl]);

  async function handleRenderRealPdf() {
    setPdfLoading(true);
    setPdfError(null);
    try {
      const blob = await fetchPreviewPDFBlob(buildPreviewRequest(formValues, language));
      const url = URL.createObjectURL(blob);
      setPdfBlobUrl((prev) => {
        if (prev) URL.revokeObjectURL(prev);
        return url;
      });
    } catch (err) {
      setPdfError(err instanceof Error ? err.message : t("renderPdfFailed"));
    } finally {
      setPdfLoading(false);
    }
  }

  const srcDoc = html ? withPreviewFontFace(html) : "";

  return (
    <div className="flex flex-col gap-3 rounded-lg border border-slate-200 bg-white p-4">
      {/* Sample-data language toggle (D-61 "sample fixture for both donor_language branches") */}
      <div className="flex items-center justify-between gap-2">
        <div className="flex gap-1">
          <Button
            type="button"
            size="sm"
            variant={language === "th" ? "default" : "outline"}
            className={language === "th" ? "bg-blue-600 text-white hover:bg-blue-700" : ""}
            onClick={() => setLanguage("th")}
          >
            {t("previewLanguageTh")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant={language === "en" ? "default" : "outline"}
            className={language === "en" ? "bg-blue-600 text-white hover:bg-blue-700" : ""}
            onClick={() => setLanguage("en")}
          >
            {t("previewLanguageEn")}
          </Button>
        </div>

        {/* Preview-mode segmented control (2-option toggle, not a Tabs navigation) */}
        <div className="flex gap-1">
          <Button
            type="button"
            size="sm"
            variant={mode === "html" ? "default" : "outline"}
            className={mode === "html" ? "bg-blue-600 text-white hover:bg-blue-700" : ""}
            onClick={() => setMode("html")}
          >
            {t("previewModeHtml")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant={mode === "pdf" ? "default" : "outline"}
            className={mode === "pdf" ? "bg-blue-600 text-white hover:bg-blue-700" : ""}
            onClick={() => {
              setMode("pdf");
              void handleRenderRealPdf();
            }}
          >
            {t("previewModePdf")}
          </Button>
        </div>
      </div>

      {/* Non-dismissible info banner (D-58 security mandate, made user-visible per UI-SPEC) */}
      <div className="rounded-md bg-blue-50 px-3 py-2 text-[14px] text-blue-700">
        {t("infoBanner")}
      </div>

      {mode === "html" ? (
        <div className="relative min-h-[480px] overflow-hidden rounded-md border border-slate-200">
          {htmlLoading && (
            <div className="absolute right-2 top-2 z-10">
              <Loader2 className="h-4 w-4 animate-spin text-slate-400" aria-hidden="true" />
            </div>
          )}
          {htmlError ? (
            <p className="p-4 text-[14px] text-red-600" role="alert">
              {htmlError}
            </p>
          ) : !hasFetchedOnce && !html ? (
            <div className="flex h-full min-h-[480px] flex-col items-center justify-center gap-1 p-6 text-center">
              <p className="text-[14px] font-medium text-slate-700">
                {t("previewEmptyHeading")}
              </p>
              <p className="text-[14px] text-slate-500">{t("previewEmptyBody")}</p>
            </div>
          ) : (
            <iframe
              title={t("iframeTitle")}
              sandbox="allow-same-origin"
              srcDoc={srcDoc}
              className="h-[480px] w-full border-0 bg-white"
            />
          )}
        </div>
      ) : (
        <div className="relative min-h-[480px] rounded-md border border-slate-200">
          {pdfLoading ? (
            <div
              className="flex h-[480px] flex-col items-center justify-center gap-2"
              aria-busy="true"
            >
              <Loader2 className="h-6 w-6 animate-spin text-slate-400" />
              <p className="text-[14px] text-slate-500">{t("renderingPdf")}</p>
            </div>
          ) : pdfError ? (
            <p className="p-4 text-[14px] text-red-600" role="alert">
              {pdfError}
            </p>
          ) : pdfBlobUrl ? (
            <embed
              src={pdfBlobUrl}
              type="application/pdf"
              className="h-[480px] w-full"
            />
          ) : (
            <div className="flex h-[480px] flex-col items-center justify-center gap-1 p-6 text-center">
              <p className="text-[14px] text-slate-500">{t("previewEmptyBody")}</p>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
