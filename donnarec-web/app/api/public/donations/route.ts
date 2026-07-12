import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";
import { passthroughGoResponse } from "@/lib/bff";

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8000";

/**
 * POST /api/public/donations — SESSION-LESS passthrough for the public donation
 * form (Flow B, D-78 / plan 06-03).
 *
 * This is the ONE Next.js route in the codebase that must NOT authenticate:
 * there is no donor session. It deliberately resolves NO server session and
 * sends NO bearer credential (doing so would 401 every donor — threat
 * T-06-21). The upstream Go route group substitutes RequireAuth with per-IP
 * rate limiting + Cloudflare Turnstile verification, so this proxy is a pure
 * body-forwarder.
 *
 * Like the slip-upload BFF route it parses multipart with request.formData()
 * (NOT bffForward's request.text(), which would corrupt the binary slip bytes)
 * and re-posts a FRESH FormData — every field plus the slip file and the
 * turnstile_token — to Go. Content-Type is intentionally left unset so fetch
 * generates its own multipart boundary. The Go origin (API_BASE_URL) stays
 * server-side; the browser never learns it (T-06-24, no CORS introduced).
 */
export async function POST(request: NextRequest): Promise<Response> {
  let incoming: FormData;
  try {
    incoming = await request.formData();
  } catch {
    return NextResponse.json(
      { error: "invalid_request_body" },
      { status: 400 }
    );
  }

  // Re-build a fresh FormData, preserving every field (donor fields, consent,
  // donor_language, turnstile_token) and the slip file with its filename.
  const outgoing = new FormData();
  for (const [key, value] of incoming.entries()) {
    if (value instanceof File) {
      outgoing.set(key, value, value.name);
    } else {
      outgoing.set(key, value);
    }
  }

  let goRes: Response;
  try {
    goRes = await fetch(`${API_BASE_URL}/api/public/donations`, {
      method: "POST",
      // Deliberately NO bearer credential (D-78 — there is no donor session)
      // and NO Content-Type — fetch sets multipart/form-data + boundary.
      body: outgoing,
      cache: "no-store",
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
