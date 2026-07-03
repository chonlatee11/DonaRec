import type { NextRequest } from "next/server";
import { bffForward } from "@/lib/bff";

/**
 * POST /api/bff/donations/:id/reject — BFF proxy for Checker permanent reject.
 *
 * Body `{ reason }` (mandatory, FR-12) is forwarded as-is by bffForward. Go
 * re-enforces RequireRoles + SoD; 422 (missing_reason) and 409 (status
 * conflict) pass through unchanged (T-12-02).
 */
export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
): Promise<Response> {
  const { id } = await params;
  return bffForward(request, `/api/donations/${id}/reject`);
}
