import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";
import {
  bffForward,
  getBffToken,
  passthroughGoResponse,
  unauthenticatedResponse,
} from "@/lib/bff";

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8000";

/**
 * POST /api/bff/donations/:id/slip — BFF proxy for slip upload (T-13-01).
 *
 * Does NOT use bffForward for this method: bffForward reads the incoming
 * body via request.text(), which is UTF-8 decoding — safe for JSON bodies but
 * NOT safe for binary file bytes inside a multipart body (would corrupt
 * non-UTF-8 bytes, e.g. JPEG/PNG binary data). Instead this parses the
 * incoming multipart body with request.formData() (Next.js handles the
 * multipart decode natively) and re-posts a FRESH FormData to Go. Content-
 * Type is deliberately NOT set on the outgoing request — fetch generates its
 * own multipart boundary from the FormData body, exactly like
 * lib/api.ts's apiFetchFormData pattern.
 *
 * Magic-byte + 10MB validation is Go's authority (T-03-14/T-03-15); this
 * proxy does not inspect file contents. Go 413 (too large) / 415 (unsupported
 * type) / 409 (slip already exists) pass through unchanged.
 */
export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
): Promise<Response> {
  const { id } = await params;
  const token = await getBffToken();
  if (!token) return unauthenticatedResponse();

  let incoming: FormData;
  try {
    incoming = await request.formData();
  } catch {
    return NextResponse.json({ error: "invalid_request_body" }, { status: 400 });
  }

  const file = incoming.get("file");
  if (!(file instanceof Blob)) {
    return NextResponse.json(
      { error: "missing_file_field", detail: "multipart field 'file' is required" },
      { status: 400 }
    );
  }

  const outgoing = new FormData();
  const filename = file instanceof File ? file.name : "slip";
  outgoing.set("file", file, filename);

  let goRes: Response;
  try {
    goRes = await fetch(`${API_BASE_URL}/api/donations/${id}/slip`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${token}`,
        // Deliberately no Content-Type — fetch sets multipart/form-data + boundary.
      },
      body: outgoing,
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

  return passthroughGoResponse(goRes);
}

/**
 * GET /api/bff/donations/:id/slip — BFF proxy for the presigned slip URL
 * (T-03-16, 15-min TTL). 404 when no active slip (D-48 — cash donations may
 * have none).
 */
export async function GET(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
): Promise<Response> {
  const { id } = await params;
  return bffForward(request, `/api/donations/${id}/slip`);
}

/**
 * DELETE /api/bff/donations/:id/slip — BFF proxy for soft-deleting the active
 * slip (D-54 — the object is NEVER hard-deleted). Returns 204 on success.
 */
export async function DELETE(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
): Promise<Response> {
  const { id } = await params;
  return bffForward(request, `/api/donations/${id}/slip`);
}
