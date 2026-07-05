import { describe, it, expect, vi, beforeEach } from "vitest";
import { NextRequest } from "next/server";
import { buildPreviewRequest, type SettingsFormValues } from "@/lib/settings";

/**
 * BFF settings route-handler trust-boundary tests (04-08).
 *
 * Hermetic: no real network, no Docker. Mocks next-auth's getServerSession
 * (the token source) and the global fetch (the Go API call) so these tests
 * exercise ONLY the BFF proxy layer added in app/api/bff/settings/**,
 * mirroring app/api/bff/donations/__tests__/bff-routes.test.ts's pattern.
 *
 * Contract under test (consumes the Admin API built in 04-07):
 *   GET/PUT  /api/bff/settings              -> /api/admin/settings
 *   POST     /api/bff/settings/preview      -> /api/admin/settings/preview
 *   POST     /api/bff/settings/preview/pdf  -> /api/admin/settings/preview/pdf
 *                                              (binary application/pdf passthrough)
 *   POST     /api/bff/settings/images/:slot -> /api/admin/settings/images/:slot
 *                                              (multipart passthrough, T-04-24/T-04-25
 *                                              mitigation lives server-side in Go —
 *                                              this proxy is never the authorization
 *                                              authority, T-12-02 convention)
 */

const mockGetServerSession = vi.fn();
vi.mock("next-auth/next", () => ({
  getServerSession: (...args: unknown[]) => mockGetServerSession(...args),
}));

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

// Imported AFTER the mocks above so bffForward/goFetch pick up the mocked module.
const { GET: settingsGET, PUT: settingsPUT } = await import("../route");
const { POST: previewPOST } = await import("../preview/route");
const { POST: previewPdfPOST } = await import("../preview/pdf/route");
const { POST: imagesPOST } = await import("../images/[slot]/route");

function jsonResponse(body: unknown, status: number): Response {
  return new Response(body === null ? null : JSON.stringify(body), {
    status,
    headers: body === null ? {} : { "Content-Type": "application/json" },
  });
}

function makeRequest(
  method: string,
  url: string,
  body?: unknown
): NextRequest {
  return new NextRequest(url, {
    method,
    ...(body !== undefined
      ? {
          body: JSON.stringify(body),
          headers: { "Content-Type": "application/json" },
        }
      : {}),
  });
}

function makeSlotParams(slot: string): { params: Promise<{ slot: string }> } {
  return { params: Promise.resolve({ slot }) };
}

beforeEach(() => {
  mockGetServerSession.mockReset();
  mockFetch.mockReset();
});

describe("BFF settings routes — trust boundary", () => {
  it("forwards a Bearer token to the Go API on GET /api/bff/settings", async () => {
    mockGetServerSession.mockResolvedValue({ accessToken: "tok" });
    mockFetch.mockResolvedValue(
      jsonResponse({ data: { template_html: "<html></html>" } }, 200)
    );

    const req = makeRequest("GET", "http://localhost/api/bff/settings");
    const res = await settingsGET(req);

    expect(res.status).toBe(200);
    const [calledUrl, calledInit] = mockFetch.mock.calls[0] as [
      string,
      RequestInit
    ];
    expect(calledUrl).toContain("/api/admin/settings");
    expect(
      (calledInit.headers as Record<string, string>).Authorization
    ).toBe("Bearer tok");
  });

  it("forwards the request body + Bearer token on PUT /api/bff/settings", async () => {
    mockGetServerSession.mockResolvedValue({ accessToken: "tok" });
    mockFetch.mockResolvedValue(jsonResponse({ data: { saved: true } }, 200));

    const payload = { template_html: "<html>{{.ReceiptNo}}</html>", separator: "/" };
    const req = makeRequest("PUT", "http://localhost/api/bff/settings", payload);
    const res = await settingsPUT(req);

    expect(res.status).toBe(200);
    const [calledUrl, calledInit] = mockFetch.mock.calls[0] as [
      string,
      RequestInit
    ];
    expect(calledUrl).toContain("/api/admin/settings");
    expect(calledInit.body).toBe(JSON.stringify(payload));
  });

  it("passes through Go 422 (invalid_template) on PUT unchanged", async () => {
    mockGetServerSession.mockResolvedValue({ accessToken: "tok" });
    mockFetch.mockResolvedValue(jsonResponse({ error: "invalid_template" }, 422));

    const req = makeRequest("PUT", "http://localhost/api/bff/settings", {});
    const res = await settingsPUT(req);

    expect(res.status).toBe(422);
  });

  it("returns 401 and does NOT call the Go API when there is no session (GET)", async () => {
    mockGetServerSession.mockResolvedValue(null);

    const req = makeRequest("GET", "http://localhost/api/bff/settings");
    const res = await settingsGET(req);

    expect(res.status).toBe(401);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("forwards the preview request body to /api/admin/settings/preview", async () => {
    mockGetServerSession.mockResolvedValue({ accessToken: "tok" });
    mockFetch.mockResolvedValue(
      jsonResponse({ data: { html: "<html>preview</html>" } }, 200)
    );

    const payload = { template_html: "<html></html>", language: "th" };
    const req = makeRequest(
      "POST",
      "http://localhost/api/bff/settings/preview",
      payload
    );
    const res = await previewPOST(req);
    const body = (await res.json()) as { data: { html: string } };

    expect(res.status).toBe(200);
    expect(body.data.html).toBe("<html>preview</html>");
    const [calledUrl] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(calledUrl).toContain("/api/admin/settings/preview");
  });

  /**
   * FI-03: the D-61 "sample-data-only" preview guarantee is enforced at the
   * CLIENT boundary by buildPreviewRequest, which whitelists exactly the
   * PreviewRequest contract fields. The BFF /settings/preview route is an
   * intentional transparent proxy — it forwards the request body verbatim with
   * a server-attached Bearer token (T-12-02) and is NOT the authority for this
   * guarantee. The previous test built a payload that already omitted PII and
   * then asserted the pass-through proxy omitted it too — it would have passed
   * even if a caller DID include PII (false confidence). Retarget at the real
   * guard: buildPreviewRequest never emits a donor/PII field, even when one is
   * injected onto the settings object.
   */
  it("buildPreviewRequest strips donor/PII fields even if present on the settings object (D-61 client guard)", () => {
    const pollutedSettings = {
      template_html: "<html></html>",
      template_html_en: "<html></html>",
      section6_text_th: "",
      section6_text_en: "",
      deduction_multiplier: "1x",
      letterhead_object_key: null,
      seal_object_key: null,
      signature_object_key: null,
      watermark_object_key: null,
      separator: "/",
      running_no_padding: 6,
      year_format: "BE4",
      prefix: "",
      // Injected donor/PII fields that must NEVER reach a preview request:
      donation_id: "d-123",
      national_id: "1234567890123",
      donor_name: "สมชาย ใจดี",
    } as unknown as SettingsFormValues;

    const req = buildPreviewRequest(pollutedSettings, "th");

    expect(req).not.toHaveProperty("donation_id");
    expect(req).not.toHaveProperty("national_id");
    expect(req).not.toHaveProperty("donor_name");
    // Positive assertion: only the whitelisted PreviewRequest contract keys.
    expect(Object.keys(req).sort()).toEqual(
      [
        "deduction_multiplier",
        "language",
        "letterhead_object_key",
        "seal_object_key",
        "section6_text_en",
        "section6_text_th",
        "signature_object_key",
        "template_html",
        "template_html_en",
        "watermark_object_key",
      ].sort()
    );
  });

  it("passes through raw application/pdf bytes on POST preview/pdf", async () => {
    mockGetServerSession.mockResolvedValue({ accessToken: "tok" });
    const pdfBytes = new Uint8Array([0x25, 0x50, 0x44, 0x46]); // "%PDF"
    mockFetch.mockResolvedValue(
      new Response(pdfBytes, {
        status: 200,
        headers: { "Content-Type": "application/pdf" },
      })
    );

    const payload = { template_html: "<html></html>", language: "th" };
    const req = makeRequest(
      "POST",
      "http://localhost/api/bff/settings/preview/pdf",
      payload
    );
    const res = await previewPdfPOST(req);

    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toContain("application/pdf");
    const bytes = new Uint8Array(await res.arrayBuffer());
    expect(Array.from(bytes)).toEqual(Array.from(pdfBytes));
    const [calledUrl] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(calledUrl).toContain("/api/admin/settings/preview/pdf");
  });

  it("returns 401 and does NOT call the Go API on preview/pdf without a session", async () => {
    mockGetServerSession.mockResolvedValue(null);

    const req = makeRequest(
      "POST",
      "http://localhost/api/bff/settings/preview/pdf",
      { template_html: "<html></html>" }
    );
    const res = await previewPdfPOST(req);

    expect(res.status).toBe(401);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("forwards a multipart image upload to /api/admin/settings/images/:slot", async () => {
    mockGetServerSession.mockResolvedValue({ accessToken: "tok" });
    mockFetch.mockResolvedValue(
      jsonResponse({ data: { slot: "signature", object_key: "settings/signature/abc.png" } }, 200)
    );

    const formData = new FormData();
    const file = new File([new Uint8Array([1, 2, 3])], "sig.png", {
      type: "image/png",
    });
    formData.set("file", file);

    const req = new NextRequest(
      "http://localhost/api/bff/settings/images/signature",
      { method: "POST", body: formData }
    );
    const res = await imagesPOST(req, makeSlotParams("signature"));
    const body = (await res.json()) as { data: { object_key: string } };

    expect(res.status).toBe(200);
    expect(body.data.object_key).toBe("settings/signature/abc.png");
    const [calledUrl, calledInit] = mockFetch.mock.calls[0] as [
      string,
      RequestInit
    ];
    expect(calledUrl).toContain("/api/admin/settings/images/signature");
    expect(
      (calledInit.headers as Record<string, string>).Authorization
    ).toBe("Bearer tok");
  });

  it("passes through Go 415 (unsupported_file_type) on image upload unchanged", async () => {
    mockGetServerSession.mockResolvedValue({ accessToken: "tok" });
    mockFetch.mockResolvedValue(
      jsonResponse({ error: "unsupported_file_type" }, 415)
    );

    const formData = new FormData();
    formData.set(
      "file",
      new File([new Uint8Array([1])], "x.pdf", { type: "application/pdf" })
    );
    const req = new NextRequest(
      "http://localhost/api/bff/settings/images/letterhead",
      { method: "POST", body: formData }
    );
    const res = await imagesPOST(req, makeSlotParams("letterhead"));

    expect(res.status).toBe(415);
  });

  it("returns 401 and does NOT call the Go API on image upload without a session", async () => {
    mockGetServerSession.mockResolvedValue(null);

    const formData = new FormData();
    formData.set("file", new File([new Uint8Array([1])], "x.png"));
    const req = new NextRequest(
      "http://localhost/api/bff/settings/images/seal",
      { method: "POST", body: formData }
    );
    const res = await imagesPOST(req, makeSlotParams("seal"));

    expect(res.status).toBe(401);
    expect(mockFetch).not.toHaveBeenCalled();
  });
});
