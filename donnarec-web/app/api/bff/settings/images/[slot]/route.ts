import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";
import { getBffToken, passthroughGoResponse, unauthenticatedResponse } from "@/lib/bff";

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8000";

/**
 * POST /api/bff/settings/images/:slot — BFF proxy for brand-image uploads
 * (letterhead/seal/signature/watermark, FR-20/21/22, D-58, plan 04-08).
 *
 * [Rule 2 — auto-added] Not explicitly named as a BFF route in the plan's
 * files_modified list, but 04-07 built POST /api/admin/settings/images/:slot
 * specifically so ImageUploadSlot could upload — without this BFF proxy, that
 * endpoint would be unreachable from the browser and the plan's own
 * must_haves truth ("brand images ... across four tabs") would be unmet, the
 * exact situation 04-07-SUMMARY.md documents for its own image endpoint.
 *
 * Does NOT use bffForward for this method: bffForward reads the incoming
 * body via request.text() (UTF-8 decoding), which is unsafe for binary file
 * bytes inside a multipart body — identical rationale to
 * app/api/bff/donations/[id]/slip/route.ts. This parses the incoming
 * multipart body with request.formData() and re-posts a FRESH FormData to
 * Go; Content-Type is deliberately NOT set so fetch generates its own
 * multipart boundary.
 *
 * Magic-byte + 2 MB cap validation is Go's authority (storage.PutTemplateImage,
 * 04-07); this proxy does not inspect file contents. Go 413 (file_too_large) /
 * 415 (unsupported_file_type) / 400 (invalid_image_slot) pass through
 * unchanged. Admin-only is enforced server-side (adminGroup); this proxy is
 * never the authorization authority (T-12-02).
 */
export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ slot: string }> }
): Promise<Response> {
  const { slot } = await params;
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
  const filename = file instanceof File ? file.name : "image";
  outgoing.set("file", file, filename);

  let goRes: Response;
  try {
    goRes = await fetch(`${API_BASE_URL}/api/admin/settings/images/${slot}`, {
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
