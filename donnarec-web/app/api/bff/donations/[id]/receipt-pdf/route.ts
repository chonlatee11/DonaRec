import type { NextRequest } from "next/server";
import { bffForward } from "@/lib/bff";

/**
 * GET /api/bff/donations/:id/receipt-pdf — BFF proxy for the frozen receipt
 * PDF's short-lived (15-min) presigned download URL (FR-28, D-57 "staff
 * download always", plan 04-06).
 *
 * D-R1: server-side Bearer forward via bffForward — same presigned-URL proxy
 * shape as the slip GET route. Any staff role (Maker/Checker/Admin) may call
 * this; Go's donationGroup route guard (not checkerGroup) already allows all
 * three. 409 receipt_not_ready passes through unchanged when the outbox
 * worker (04-05) has not finished freezing the PDF yet.
 */
export async function GET(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
): Promise<Response> {
  const { id } = await params;
  return bffForward(request, `/api/donations/${id}/receipt-pdf`);
}
