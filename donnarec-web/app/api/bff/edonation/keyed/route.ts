import type { NextRequest } from "next/server";
import { bffForward } from "@/lib/bff";

/**
 * POST /api/bff/edonation/keyed — BFF proxy for the bulk/per-row
 * "คีย์เข้า e-Donation แล้ว" mark/unmark mutation (FR-31/D-67, plan
 * 05-04/05-06).
 *
 * D-R1: bffForward obtains the Keycloak access token server-side and
 * forwards a Bearer to Go POST /api/edonation/keyed — the browser only ever
 * sends `{donation_ids, keyed}` to this same-origin route. Go's
 * RequireAnyRole(Checker,Admin) guard (05-02) is the real authority; the
 * validator-checked-UUID / issued-only-scope discipline lives entirely in
 * the Go handler/service (05-04) — this route is a pure passthrough.
 */
export async function POST(request: NextRequest): Promise<Response> {
  return bffForward(request, "/api/edonation/keyed");
}
