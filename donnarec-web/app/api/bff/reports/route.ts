import type { NextRequest } from "next/server";
import { bffForward } from "@/lib/bff";

/**
 * GET /api/bff/reports — BFF proxy for the PII-free donation summary report
 * (FR-32/D-70/D-71, plan 05-05/05-07).
 *
 * D-R1: bffForward forwards the query string verbatim (from/to/group_by) and
 * the Keycloak Bearer server-side to Go GET /api/reports/summary, passing
 * the `{data:{total_amount,receipt_count,average_per_receipt,breakdown}}`
 * envelope through unchanged.
 *
 * D-71: the Go route this proxies (reportGroup) carries NO
 * RequireAnyRole/RequireRoles guard — every authenticated staff member
 * (Maker/Checker/Admin) reaches this handler.
 */
export async function GET(request: NextRequest): Promise<Response> {
  const qs = request.nextUrl.search; // includes leading "?" or ""
  return bffForward(request, `/api/reports/summary${qs}`);
}
