import type { NextRequest } from "next/server";
import { bffForward } from "@/lib/bff";

/**
 * GET /api/bff/edonation/aging — BFF proxy for the 3-bucket
 * (not_due/near_due/overdue) aging view of unkeyed issued donations
 * (FR-31/D-68, plan 05-04/05-06).
 *
 * D-R1: bffForward forwards the query string verbatim (the only supported
 * param is the optional `?now=RFC3339` test/preview override) and the
 * Keycloak Bearer server-side to Go GET /api/edonation/aging, passing the
 * `{data:{rows,counts}}` envelope through unchanged.
 */
export async function GET(request: NextRequest): Promise<Response> {
  const qs = request.nextUrl.search;
  return bffForward(request, `/api/edonation/aging${qs}`);
}
