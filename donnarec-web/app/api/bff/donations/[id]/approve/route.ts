import type { NextRequest } from "next/server";
import { bffForward } from "@/lib/bff";

/**
 * POST /api/bff/donations/:id/approve — BFF proxy for Checker approval.
 *
 * D-R1: server-side Bearer forward via bffForward. Go re-enforces RequireRoles
 * (checker|admin) + SoD (approver != creator) — this route is a proxy, not the
 * authorization authority (T-12-02). Go 403 (sod/insufficient_role) and 409
 * (status conflict — already actioned) pass through unchanged.
 */
export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
): Promise<Response> {
  const { id } = await params;
  return bffForward(request, `/api/donations/${id}/approve`);
}
