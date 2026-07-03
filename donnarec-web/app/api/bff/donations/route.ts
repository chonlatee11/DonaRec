import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";
import {
  bffForward,
  getBffToken,
  goFetch,
  mapFeDonorFieldsToGo,
  passthroughGoResponse,
  unauthenticatedResponse,
  type FeDonorFields,
} from "@/lib/bff";

/**
 * GET /api/bff/donations — BFF proxy for the donation LIST (Screen 1).
 *
 * D-R1: This Route Handler runs server-side. It obtains the Keycloak access
 * token via getServerSession (inside bffForward) and forwards a Bearer to the
 * Go API GET /api/donations. The client (TanStack Query) only ever calls this
 * same-origin endpoint — the token never reaches the browser.
 *
 * D-R2: The Go API returns the pagination envelope
 *   { "data": { "items": [...], "total": N, "page": P, "per_page": 20 } }
 * which this handler passes through unchanged.
 *
 * D-53: only name/status/from/to/receipt_no/page are forwarded — the query
 * string is passed verbatim; PII (national ID) is never a search key.
 */
export async function GET(request: NextRequest): Promise<Response> {
  const qs = request.nextUrl.search; // includes leading "?" or ""
  return bffForward(request, `/api/donations${qs}`);
}

/**
 * POST /api/bff/donations — BFF proxy for creating a new draft (Screen 2, FR-07).
 *
 * D-R3 contract fix: the browser sends FE field names (national_id/address/
 * email/note); the Go CreateDonationRequest expects donor_tax_id/donor_address/
 * donor_email/notes. This route maps them via mapFeDonorFieldsToGo BEFORE
 * forwarding — the browser never needs to know the Go contract's field names.
 * Go 422 validation errors (e.g. malformed national ID) pass through unchanged.
 */
export async function POST(request: NextRequest): Promise<Response> {
  const token = await getBffToken();
  if (!token) return unauthenticatedResponse();

  let body: FeDonorFields;
  try {
    body = (await request.json()) as FeDonorFields;
  } catch {
    return NextResponse.json({ error: "invalid_request_body" }, { status: 400 });
  }

  const goRes = await goFetch(token, "/api/donations", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(mapFeDonorFieldsToGo(body)),
  });

  return passthroughGoResponse(goRes);
}
