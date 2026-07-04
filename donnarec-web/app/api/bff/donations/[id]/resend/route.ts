import type { NextRequest } from "next/server";
import { bffForward } from "@/lib/bff";

/**
 * POST /api/bff/donations/:id/resend — BFF proxy for staff-triggered receipt
 * email resend (D-56/D-57, FR-27, plan 04-06).
 *
 * D-R1: server-side Bearer forward via bffForward. Go re-enforces
 * RequireAnyRole(checker|admin) on the checkerGroup route + a service-layer
 * role check — this route is a thin proxy, not the authorization authority
 * (T-12-02). Go 403 (insufficient_role/forbidden), 409 (status_conflict /
 * receipt_not_ready — worker has not frozen the PDF yet), and 404 pass
 * through unchanged. Resend NEVER allocates a new receipt number or
 * re-renders the PDF — it only re-enqueues an outbox job.
 */
export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
): Promise<Response> {
  const { id } = await params;
  return bffForward(request, `/api/donations/${id}/resend`);
}
