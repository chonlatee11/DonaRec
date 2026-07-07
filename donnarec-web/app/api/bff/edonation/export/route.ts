import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";
import { getBffToken, goFetch, unauthenticatedResponse } from "@/lib/bff";

/**
 * GET /api/bff/edonation/export — BFF proxy for the audited, RBAC-gated
 * e-Donation export stream (FR-30, D-63/D-64/D-74, plan 05-02/05-06).
 *
 * D-R1: server-side Bearer forward (getBffToken/goFetch) — same discipline
 * as every other BFF route. The access token never reaches the browser
 * (T-05-06-TOKEN).
 *
 * Does NOT use bffForward/passthroughGoResponse for the response: Go's
 * Export handler (internal/edonation/handler.go) streams raw
 * `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet` or
 * `text/csv` bytes via `c.Writer` (not the usual `{data:...}` JSON envelope)
 * — passthroughGoResponse's `res.text()` + `JSON.parse` path would corrupt
 * the binary xlsx bytes exactly like
 * app/api/bff/settings/preview/pdf/route.ts's documented reason for
 * bypassing bffForward on the response side. This route reads
 * `arrayBuffer()` and forwards the bytes + Content-Type + Content-Disposition
 * verbatim (T-05-06-BINARY: no re-encode).
 *
 * On a Go error (403 forbidden, 404 no_records, 400 invalid_format, 500),
 * the response is the usual JSON error envelope — passed through as JSON
 * since it is not an xlsx/csv content type.
 */
export async function GET(request: NextRequest): Promise<Response> {
  const token = await getBffToken();
  if (!token) return unauthenticatedResponse();

  const qs = request.nextUrl.search; // includes leading "?" or ""

  let goRes: Response;
  try {
    goRes = await goFetch(token, `/api/edonation/export${qs}`);
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

  // Non-binary response (403/404/400/500) — Go's usual JSON error envelope.
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
