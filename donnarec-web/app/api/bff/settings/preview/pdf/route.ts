import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";
import { getBffToken, goFetch, unauthenticatedResponse } from "@/lib/bff";

/**
 * POST /api/bff/settings/preview/pdf — BFF proxy for the "real PDF" preview
 * mode (D-61 hybrid preview strategy: fast sandboxed-HTML preview + an
 * on-demand accurate render through the SAME sandboxed Chromium pipeline as
 * production, plan 04-08).
 *
 * Does NOT use bffForward/passthroughGoResponse for the response: Go's
 * PreviewPDF handler (internal/settings/handler.go) returns raw
 * `application/pdf` bytes (`c.Data(...)`), not the usual `{data:...}` JSON
 * envelope — passthroughGoResponse's `res.text()` + `JSON.parse` path would
 * corrupt binary PDF bytes exactly like the slip upload route's documented
 * reason for bypassing bffForward on the REQUEST side (see
 * app/api/bff/donations/[id]/slip/route.ts). Here it's the RESPONSE side that
 * is binary, so this route reads `arrayBuffer()` and forwards the bytes +
 * Content-Type verbatim.
 *
 * On a Go error (e.g. invalid_template, 422), the response is the usual JSON
 * error envelope — passed through as JSON since it is not `application/pdf`.
 */
export async function POST(request: NextRequest): Promise<Response> {
  const token = await getBffToken();
  if (!token) return unauthenticatedResponse();

  let body: string;
  try {
    body = await request.text();
  } catch {
    return NextResponse.json({ error: "invalid_request_body" }, { status: 400 });
  }

  let goRes: Response;
  try {
    goRes = await goFetch(token, "/api/admin/settings/preview/pdf", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body,
    });
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
  if (contentType.includes("application/pdf")) {
    const bytes = await goRes.arrayBuffer();
    return new NextResponse(bytes, {
      status: goRes.status,
      headers: { "Content-Type": "application/pdf" },
    });
  }

  // Non-PDF response (e.g. 422 invalid_template, 500) — Go's usual JSON error envelope.
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
