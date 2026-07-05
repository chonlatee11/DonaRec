import { describe, it, expect, vi, beforeEach } from "vitest";
import { NextRequest } from "next/server";

/**
 * BFF route-handler trust-boundary tests (03-12).
 *
 * Hermetic: no real network, no Docker. Mocks next-auth's getServerSession
 * (the token source) and the global fetch (the Go API call) so these tests
 * exercise ONLY the BFF proxy layer added in app/api/bff/donations/[id]/**.
 *
 * Asserts the exact trust-boundary contract from the plan's threat register
 * (T-12-01..T-12-04):
 *   1. Bearer forwarding — bffForward attaches `Authorization: Bearer <token>`.
 *   2. 401 + no Go call when there is no server session.
 *   3. PII field mapping — Go `donor_tax_id` → FE `national_id`.
 *   4. slip_url composition — 200 from /:id/slip → the url; 404 → null.
 *   5. Token-leak guard — no route response body ever contains the access
 *      token string.
 */

const mockGetServerSession = vi.fn();
vi.mock("next-auth/next", () => ({
  getServerSession: (...args: unknown[]) => mockGetServerSession(...args),
}));

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

// Imported AFTER the mocks above so bffForward picks up the mocked module.
const { GET: detailGET } = await import("../[id]/route");
const { GET: piiGET } = await import("../[id]/pii/route");
const { POST: approvePOST } = await import("../[id]/approve/route");
const { POST: returnPOST } = await import("../[id]/return/route");
const { POST: rejectPOST } = await import("../[id]/reject/route");
const { POST: resendPOST } = await import("../[id]/resend/route");
const { GET: receiptPdfGET } = await import("../[id]/receipt-pdf/route");

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

function makeParams(id: string): { params: Promise<{ id: string }> } {
  return { params: Promise.resolve({ id }) };
}

beforeEach(() => {
  mockGetServerSession.mockReset();
  mockFetch.mockReset();
});

describe("BFF donation [id] routes — trust boundary", () => {
  it("forwards a Bearer token to the Go API on approve", async () => {
    mockGetServerSession.mockResolvedValue({ accessToken: "tok" });
    mockFetch.mockResolvedValue(jsonResponse({ data: { id: "1" } }, 200));

    const req = makeRequest(
      "POST",
      "http://localhost/api/bff/donations/1/approve"
    );
    const res = await approvePOST(req, makeParams("1"));

    expect(res.status).toBe(200);
    expect(mockFetch).toHaveBeenCalledTimes(1);
    const [calledUrl, calledInit] = mockFetch.mock.calls[0] as [
      string,
      RequestInit
    ];
    expect(calledUrl).toContain("/api/donations/1/approve");
    expect(
      (calledInit.headers as Record<string, string>).Authorization
    ).toBe("Bearer tok");
  });

  it("forwards the reason body + Bearer token on return", async () => {
    mockGetServerSession.mockResolvedValue({ accessToken: "tok" });
    mockFetch.mockResolvedValue(jsonResponse({ data: { id: "1" } }, 200));

    const req = makeRequest(
      "POST",
      "http://localhost/api/bff/donations/1/return",
      { reason: "needs correction" }
    );
    const res = await returnPOST(req, makeParams("1"));

    expect(res.status).toBe(200);
    const [calledUrl, calledInit] = mockFetch.mock.calls[0] as [
      string,
      RequestInit
    ];
    expect(calledUrl).toContain("/api/donations/1/return");
    expect(calledInit.body).toBe(JSON.stringify({ reason: "needs correction" }));
  });

  it("passes through Go 422 (missing_reason) on reject unchanged", async () => {
    mockGetServerSession.mockResolvedValue({ accessToken: "tok" });
    mockFetch.mockResolvedValue(
      jsonResponse({ code: "MISSING_REASON", message: "กรุณาระบุเหตุผล" }, 422)
    );

    const req = makeRequest(
      "POST",
      "http://localhost/api/bff/donations/1/reject",
      { reason: "" }
    );
    const res = await rejectPOST(req, makeParams("1"));

    expect(res.status).toBe(422);
  });

  it("returns 401 and does NOT call the Go API when there is no session", async () => {
    mockGetServerSession.mockResolvedValue(null);

    const req = makeRequest("GET", "http://localhost/api/bff/donations/1");
    const res = await detailGET(req, makeParams("1"));

    expect(res.status).toBe(401);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("maps Go donor_tax_id to FE national_id on PII reveal", async () => {
    mockGetServerSession.mockResolvedValue({ accessToken: "tok" });
    mockFetch.mockResolvedValue(
      jsonResponse(
        { data: { donation_id: "1", donor_tax_id: "1234567890123" } },
        200
      )
    );

    const req = makeRequest(
      "GET",
      "http://localhost/api/bff/donations/1/pii"
    );
    const res = await piiGET(req, makeParams("1"));
    const body = (await res.json()) as { data: { national_id: string } };

    expect(res.status).toBe(200);
    expect(body.data.national_id).toBe("1234567890123");
    // The Go field name must not leak through unmapped.
    expect(JSON.stringify(body)).not.toContain("donor_tax_id");
  });

  it("composes slip_url from the Go url when /:id/slip returns 200", async () => {
    mockGetServerSession.mockResolvedValue({ accessToken: "tok" });
    mockFetch
      .mockResolvedValueOnce(
        jsonResponse({ data: { id: "1", status: "issued" } }, 200)
      )
      .mockResolvedValueOnce(
        jsonResponse(
          { data: { url: "https://minio.local/slip.pdf", expires_in_seconds: 900 } },
          200
        )
      );

    const req = makeRequest("GET", "http://localhost/api/bff/donations/1");
    const res = await detailGET(req, makeParams("1"));
    const body = (await res.json()) as { data: { slip_url: string | null } };

    expect(res.status).toBe(200);
    expect(body.data.slip_url).toBe("https://minio.local/slip.pdf");
    expect(mockFetch).toHaveBeenCalledTimes(2);
  });

  it("composes slip_url = null when /:id/slip returns 404 (no active slip)", async () => {
    mockGetServerSession.mockResolvedValue({ accessToken: "tok" });
    mockFetch
      .mockResolvedValueOnce(
        jsonResponse({ data: { id: "1", status: "draft" } }, 200)
      )
      .mockResolvedValueOnce(jsonResponse(null, 404));

    const req = makeRequest("GET", "http://localhost/api/bff/donations/1");
    const res = await detailGET(req, makeParams("1"));
    const body = (await res.json()) as { data: { slip_url: string | null } };

    expect(res.status).toBe(200);
    expect(body.data.slip_url).toBeNull();
  });

  it("forwards a Bearer token to the Go API on resend (plan 04-06)", async () => {
    mockGetServerSession.mockResolvedValue({ accessToken: "tok" });
    mockFetch.mockResolvedValue(
      jsonResponse({ data: { donation_id: "1", status: "resend_enqueued" } }, 200)
    );

    const req = makeRequest(
      "POST",
      "http://localhost/api/bff/donations/1/resend"
    );
    const res = await resendPOST(req, makeParams("1"));

    expect(res.status).toBe(200);
    const [calledUrl, calledInit] = mockFetch.mock.calls[0] as [
      string,
      RequestInit
    ];
    expect(calledUrl).toContain("/api/donations/1/resend");
    expect(
      (calledInit.headers as Record<string, string>).Authorization
    ).toBe("Bearer tok");
  });

  it("passes through Go 409 receipt_not_ready on resend unchanged (plan 04-06)", async () => {
    mockGetServerSession.mockResolvedValue({ accessToken: "tok" });
    mockFetch.mockResolvedValue(jsonResponse({ error: "receipt_not_ready" }, 409));

    const req = makeRequest(
      "POST",
      "http://localhost/api/bff/donations/1/resend"
    );
    const res = await resendPOST(req, makeParams("1"));

    expect(res.status).toBe(409);
  });

  it("forwards a Bearer token to the Go API on receipt-pdf download (plan 04-06)", async () => {
    mockGetServerSession.mockResolvedValue({ accessToken: "tok" });
    mockFetch.mockResolvedValue(
      jsonResponse({ data: { url: "https://minio.local/receipt.pdf" } }, 200)
    );

    const req = makeRequest(
      "GET",
      "http://localhost/api/bff/donations/1/receipt-pdf"
    );
    const res = await receiptPdfGET(req, makeParams("1"));
    const body = (await res.json()) as { data: { url: string } };

    expect(res.status).toBe(200);
    expect(body.data.url).toBe("https://minio.local/receipt.pdf");
    const [calledUrl, calledInit] = mockFetch.mock.calls[0] as [
      string,
      RequestInit
    ];
    expect(calledUrl).toContain("/api/donations/1/receipt-pdf");
    expect(
      (calledInit.headers as Record<string, string>).Authorization
    ).toBe("Bearer tok");
  });

  it("returns 401 and does NOT call the Go API on receipt-pdf without a session (plan 04-06)", async () => {
    mockGetServerSession.mockResolvedValue(null);

    const req = makeRequest(
      "GET",
      "http://localhost/api/bff/donations/1/receipt-pdf"
    );
    const res = await receiptPdfGET(req, makeParams("1"));

    expect(res.status).toBe(401);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("never leaks the access token into any route response body", async () => {
    const secret = "super-secret-access-token-xyz";
    mockGetServerSession.mockResolvedValue({ accessToken: secret });
    mockFetch
      .mockResolvedValueOnce(jsonResponse({ data: { id: "1" } }, 200))
      .mockResolvedValueOnce(jsonResponse(null, 404));

    const req = makeRequest("GET", "http://localhost/api/bff/donations/1");
    const res = await detailGET(req, makeParams("1"));
    const text = await res.text();

    expect(text).not.toContain(secret);
    expect(text).not.toContain("Bearer");
    expect(text).not.toContain("accessToken");
  });
});
