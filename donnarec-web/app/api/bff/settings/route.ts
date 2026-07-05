import type { NextRequest } from "next/server";
import { bffForward } from "@/lib/bff";

/**
 * GET/PUT /api/bff/settings — BFF proxy for the Admin receipt-template/
 * compliance settings screen (Screen 6, D-58/D-59/NFR-09, plan 04-08).
 *
 * D-R1: server-side Bearer forward via bffForward. Go re-enforces
 * RequireRoles(admin) on the adminGroup route + Admin-only (T-04-25) — this
 * route is a thin proxy, not the authorization authority (T-12-02). Go 403
 * (insufficient_role) and 422 (invalid_template / invalid_number_format on
 * PUT) pass through unchanged.
 *
 * GET  -> GET /api/admin/settings   (merged template + number-format config)
 * PUT  -> PUT /api/admin/settings   ("save all tabs" in one request, D-58)
 */
export async function GET(request: NextRequest): Promise<Response> {
  return bffForward(request, "/api/admin/settings");
}

export async function PUT(request: NextRequest): Promise<Response> {
  return bffForward(request, "/api/admin/settings");
}
