import type { NextRequest } from "next/server";
import { bffForward } from "@/lib/bff";

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
