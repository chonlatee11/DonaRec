import type { NextRequest } from "next/server";
import { bffForward } from "@/lib/bff";

/**
 * GET /api/bff/queue — authenticated BFF proxy for the pending-review queue
 * (Screen 11, FR-08).
 *
 * This is an (app) staff-only surface — it reuses bffForward (session-bound
 * Bearer forwarding, T-06-25), never the session-less public passthrough. The
 * Go API re-verifies the Bearer + the donationGroup role guard on GET
 * /api/donations; the BFF is a proxy, not the authorization authority.
 *
 * Contract (T-06-27 — the client cannot inject an arbitrary source into SQL):
 *   - `status` is PINNED to `pending_review` here; the caller cannot widen it.
 *   - `?source=` is mapped to plan 06-01's server-side allow-list filter:
 *       all           → omit `source` (both flow_a + flow_b)
 *       from-website   → source=flow_b
 *       staff-entered  → source=flow_a
 *     any other value is dropped (falls back to "all"); the Go handler
 *     independently rejects a non flow_a/flow_b value with 400 invalid_source.
 *   - `page` is passed through for 20-row pagination.
 */
export async function GET(request: NextRequest): Promise<Response> {
  const incoming = request.nextUrl.searchParams;

  const params = new URLSearchParams();
  // status is pinned server-side — the queue only ever shows pending_review.
  params.set("status", "pending_review");

  // Map the UI filter token to plan 01's flow_a/flow_b source narg.
  const sourceToken = incoming.get("source");
  if (sourceToken === "from-website") {
    params.set("source", "flow_b");
  } else if (sourceToken === "staff-entered") {
    params.set("source", "flow_a");
  }
  // "all" (or anything unrecognised) → omit `source` entirely.

  const page = incoming.get("page");
  if (page && page !== "1") {
    params.set("page", page);
  }

  return bffForward(request, `/api/donations?${params.toString()}`);
}
