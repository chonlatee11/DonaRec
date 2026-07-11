import type { NextRequest } from "next/server";
import { bffForward } from "@/lib/bff";

/**
 * GET/PUT /api/bff/edonation-config — BFF proxy for the Admin-only
 * e-Donation field-mapping/cash-type-label/near-due-days config route
 * (D-75/NFR-09, plan 05-02/05-07).
 *
 * D-R1: bffForward forwards the Keycloak Bearer server-side to Go
 * GET/PUT /api/admin/edonation-config, passing the
 * `{data:{field_mapping,cash_type_label,near_due_days,updated_at,updated_by}}`
 * envelope through unchanged.
 *
 * Go's adminGroup.Use(RequireRoles(RoleAdmin)) is the real authority — this
 * route is only ever called from EdonationConfigTab, which is only rendered
 * inside the already-Admin-gated /admin/settings route
 * (app/admin/settings/page.tsx's isAdminViewer() redirect). A non-Admin
 * caller that somehow reached this route still gets a 403 from Go.
 */
export async function GET(request: NextRequest): Promise<Response> {
  return bffForward(request, "/api/admin/edonation-config");
}

export async function PUT(request: NextRequest): Promise<Response> {
  return bffForward(request, "/api/admin/edonation-config");
}
