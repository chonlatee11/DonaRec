import { getServerSession } from "next-auth/next";
import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";
import { authOptions } from "./auth";

/**
 * Go API base URL (server-side). Same source as lib/api.ts.
 * Set NEXT_PUBLIC_API_BASE_URL in the environment; defaults to local dev.
 */
const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8000";

/**
 * getBffToken — resolves the current server session's Keycloak access token.
 *
 * Exposed (03-13) so BFF routes that must COMPOSE multiple Go calls before
 * responding — e.g. reissue needs the original record's detail + an audited
 * PII reveal before it can build the Go ReissueDonationRequest body — can
 * fetch the token once and reuse it via goFetch, instead of going through the
 * generic single-call bffForward passthrough.
 */
export async function getBffToken(): Promise<string | null> {
  const session = await getServerSession(authOptions);
  return session?.accessToken ?? null;
}

/** Standard 401 JSON response — identical shape to bffForward's own gate. */
export function unauthenticatedResponse(): NextResponse {
  return NextResponse.json(
    { error: "unauthenticated", message: "ไม่พบเซสชัน — กรุณาเข้าสู่ระบบใหม่" },
    { status: 401 }
  );
}

/**
 * goFetch — low-level authenticated fetch to the Go API. Returns the raw
 * Response; callers decide how to parse it. Composition routes (reissue) need
 * the JSON body itself to build a follow-up request, not just a passthrough.
 */
export async function goFetch(
  token: string,
  goPath: string,
  init?: RequestInit
): Promise<Response> {
  return fetch(`${API_BASE_URL}${goPath}`, {
    ...init,
    headers: {
      ...(init?.headers as Record<string, string> | undefined),
      Authorization: `Bearer ${token}`,
    },
    cache: "no-store",
  });
}

/**
 * passthroughGoResponse — converts a raw Go Response into a NextResponse,
 * preserving status + body exactly like bffForward's own pass-through tail
 * (204 → empty body; JSON parsed when present; non-JSON wrapped as
 * {message:text}). Exposed so composition routes reuse identical handling.
 */
export async function passthroughGoResponse(
  goRes: Response
): Promise<NextResponse> {
  if (goRes.status === 204) {
    return new NextResponse(null, { status: 204 });
  }

  let payload: unknown = null;
  const text = await goRes.text();
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = { message: text };
    }
  }

  return NextResponse.json(payload, { status: goRes.status });
}

/**
 * bffForward — server-side proxy helper for the BFF Route Handlers.
 *
 * D-R1: TanStack Query (client) calls Next.js Route Handlers under
 * `app/api/bff/**`. Those handlers run ON THE SERVER, obtain the Keycloak
 * access token via getServerSession(authOptions), and forward it as a Bearer
 * to the Go API. The access token therefore NEVER reaches the browser — the
 * client only ever talks to the same-origin BFF route.
 *
 * Behaviour:
 *   - No server session / no access token → 401 JSON (client is unauthenticated).
 *   - Otherwise fetch `${API_BASE_URL}${goPath}` with Authorization: Bearer,
 *     forwarding the incoming method/body, and pass the Go response through
 *     unchanged (status + parsed JSON body).
 *
 * The Go API re-verifies the Bearer (RequireAuth) — the BFF is a proxy, not the
 * authorization authority (threat T-10-02).
 *
 * NOTE: this reads the incoming body via request.text() — safe for JSON
 * bodies (submit/cancel/approve/return/reject) but NOT for multipart file
 * uploads (binary bytes are not guaranteed valid UTF-8). The slip upload
 * route therefore does NOT use bffForward for its POST handler (see
 * app/api/bff/donations/[id]/slip/route.ts).
 */
export async function bffForward(
  request: NextRequest,
  goPath: string
): Promise<NextResponse> {
  const session = await getServerSession(authOptions);
  const accessToken = session?.accessToken;

  if (!accessToken) {
    return unauthenticatedResponse();
  }

  const method = request.method ?? "GET";

  const headers: Record<string, string> = {
    // T-10-01: token added server-side only; it is never serialized back to the
    // browser response body.
    Authorization: `Bearer ${accessToken}`,
  };

  // Forward a body only for methods that carry one.
  let body: string | undefined;
  if (method !== "GET" && method !== "HEAD") {
    body = await request.text();
    if (body) {
      headers["Content-Type"] =
        request.headers.get("content-type") ?? "application/json";
    }
  }

  let goRes: Response;
  try {
    goRes = await fetch(`${API_BASE_URL}${goPath}`, {
      method,
      headers,
      body,
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

  // Pass the Go response through: preserve status + parsed JSON body.
  return passthroughGoResponse(goRes);
}

// ---------------------------------------------------------------------------
// FE -> Go donor-field mapping (D-R3 remediation, 03-13)
// ---------------------------------------------------------------------------

/**
 * FeDonorFields — the shape the browser sends on create/update, and the shape
 * the reissue route composes server-side from the original record. Uses the
 * FE's own field names (national_id/address/email/note) — the browser never
 * needs to know the Go contract's field names.
 */
export interface FeDonorFields {
  donor_name: string;
  national_id: string;
  address: string;
  email?: string | null;
  amount: number;
  donated_at: string;
  note?: string | null;
  consent_given: boolean;
  consent_text_version?: string | null;
  consent_purpose?: string | null;
  /** D-55/FR-23: document language for PDF/email; omitted defaults to "th" server-side */
  donor_language?: "th" | "en" | null;
}

/**
 * mapFeDonorFieldsToGo — the field-name mapping this plan exists to add:
 * national_id -> donor_tax_id, address -> donor_address, email -> donor_email,
 * note -> notes. Used by the create (POST) and update (PUT) BFF routes, and
 * by the reissue route's server-side composition (D-50: reissue creates a
 * replacement draft carrying the original record's donor data — the reissue
 * dialog only collects a free-text reason per UI-SPEC, not corrected fields).
 */
export function mapFeDonorFieldsToGo(
  fields: FeDonorFields
): Record<string, unknown> {
  return {
    donor_name: fields.donor_name,
    donor_tax_id: fields.national_id,
    donor_address: fields.address,
    donor_email: fields.email ?? "",
    amount: fields.amount,
    donated_at: fields.donated_at,
    notes: fields.note ?? "",
    consent_given: fields.consent_given,
    consent_text_version: fields.consent_text_version ?? "",
    consent_purpose: fields.consent_purpose ?? "tax-receipt",
    ...(fields.donor_language ? { donor_language: fields.donor_language } : {}),
  };
}
