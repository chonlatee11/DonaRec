import type { NextRequest } from "next/server";
import { bffForward } from "@/lib/bff";

/**
 * POST /api/bff/donations/:id/cancel — BFF proxy for voiding an issued
 * receipt (FR-19, D-47, Checker/Admin only).
 *
 * D-R1: server-side Bearer forward via bffForward. The client body
 * `{reason, rd_confirmation_reason?}` already matches the Go
 * CancelDonationRequest field names exactly — no mapping needed. Go
 * re-enforces the Checker/Admin route guard (T-13-03) and the receipt number
 * is RETAINED on the cancelled record (no gap — load-bearing invariant, this
 * proxy does not touch that logic). Go 409 (edonation_keyed_confirmation_
 * required — D-51) and 422 (reason_required) pass through unchanged.
 */
export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
): Promise<Response> {
  const { id } = await params;
  return bffForward(request, `/api/donations/${id}/cancel`);
}
