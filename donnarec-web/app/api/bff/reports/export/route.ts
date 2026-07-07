import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";
import { getBffToken, goFetch, unauthenticatedResponse } from "@/lib/bff";

/**
 * GET /api/bff/reports/export — BFF proxy for the PII-free report export
 * stream (FR-32/D-70, plan 05-05/05-07).
 *
 * D-R1: server-side Bearer forward (getBffToken/goFetch) — same discipline
 * as every other BFF route. The access token never reaches the browser.
 *
 * Does NOT use bffForward/passthroughGoResponse for the response: Go's
 * report Export handler (internal/report/handler.go) streams raw
 * `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet` or
 * `text/csv` bytes via `c.Writer` (not the usual `{data:...}` JSON envelope)
 * — passthroughGoResponse's `res.text()` + `JSON.parse` path would corrupt
 * the binary xlsx bytes, exactly like
 * app/api/bff/edonation/export/route.ts's documented reason for bypassing
 * bffForward on the response side. This route reads `arrayBuffer()` and
 * forwards the bytes + Content-Type + Content-Disposition verbatim.
 *
 * D-70: unlike the e-Donation export (Screen 7), this export contains ZERO
 * PII and requires no confirmation dialog on the client side, and the Go
 * handler writes no audit_log row for it.
 */
export async function GET(request: NextRequest): Promise<Response> {
  const token = await getBffToken();
  if (!token) return unauthenticatedResponse();

  const qs = request.nextUrl.search; // includes leading "?" or ""

  let goRes: Response;
  try {
    goRes = await goFetch(token, `/api/reports/export${qs}`);
  } catch {
    return NextResponse.json(
      {
        error: "network",
        message:
          "ไม่สามารถเชื่อมต่อระบบได้ — กรุณาตรวจสอบการเชื่อมต่อและลองอีกครั้ง",
      },
      { status: 502 }
    );
  }

  const contentType = goRes.headers.get("content-type") ?? "";
  const isBinaryExport =
    contentType.includes("spreadsheetml") || contentType.includes("text/csv");

  if (isBinaryExport) {
    const bytes = await goRes.arrayBuffer();
    const headers: Record<string, string> = { "Content-Type": contentType };
    const disposition = goRes.headers.get("content-disposition");
    if (disposition) headers["Content-Disposition"] = disposition;
    return new NextResponse(bytes, { status: goRes.status, headers });
  }

  // Non-binary response (400/500) — Go's usual JSON error envelope.
  const text = await goRes.text();
  let payload: unknown = null;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = { message: text };
    }
  }
  return NextResponse.json(payload, { status: goRes.status });
}
