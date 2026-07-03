import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";
import { bffForward } from "@/lib/bff";

/**
 * GET /api/bff/donations/:id — BFF proxy for the donation DETAIL screen (Screen 3).
 *
 * D-R1: server-side Bearer forward via bffForward; the access token never
 * reaches the browser.
 *
 * T-12-04: composes `slip_url` by ALSO calling Go GET /:id/slip server-side —
 * the browser never calls the presigned-URL endpoint directly. 200 → the
 * presigned url; 404 (no active slip, D-48) → null. Both calls reuse the same
 * incoming GET request (bffForward only reads the request body for
 * non-GET/HEAD methods, so calling it twice on a GET request is safe).
 */
export async function GET(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
): Promise<Response> {
  const { id } = await params;

  const detailRes = await bffForward(request, `/api/donations/${id}`);
  if (detailRes.status !== 200) {
    // 401 (no session), 403, 404, etc. — pass through unchanged. No slip call
    // is made when the detail fetch itself failed/was unauthenticated.
    return detailRes;
  }

  const detailBody = (await detailRes.json()) as { data?: Record<string, unknown> };
  const detail = detailBody.data ?? {};

  const slipRes = await bffForward(request, `/api/donations/${id}/slip`);
  let slipUrl: string | null = null;
  if (slipRes.status === 200) {
    const slipBody = (await slipRes.json()) as { data?: { url?: string } };
    slipUrl = slipBody.data?.url ?? null;
  }
  // 404 (no active slip) and any other non-200 → slipUrl stays null.

  return NextResponse.json(
    { data: { ...detail, slip_url: slipUrl } },
    { status: 200 }
  );
}
