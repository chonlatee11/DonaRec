import type { NextRequest } from "next/server";
import { bffForward } from "@/lib/bff";

/**
 * POST /api/bff/donations/:id/submit — BFF proxy for draft → pending_review
 * transition (FR-11, D-45).
 *
 * D-R1: server-side Bearer forward via bffForward. Go's row-level lock keeps
 * the gap-less counter path unaffected by this proxy layer. Go 409
 * (status_conflict — not currently in draft) passes through unchanged.
 */
export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
): Promise<Response> {
  const { id } = await params;
  return bffForward(request, `/api/donations/${id}/submit`);
}
