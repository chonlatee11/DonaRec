import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";
import { bffForward } from "@/lib/bff";

/**
 * GET /api/bff/donations/:id/pii — BFF proxy for the audited PII reveal (Screen 4).
 *
 * D-R1: server-side Bearer forward via bffForward — the token never reaches
 * the browser. The Go endpoint is checker/admin-gated and writes an audit_log
 * entry on every call (T-12-01) — this proxy does not add or bypass that.
 *
 * Field mapping: Go returns `{ data: { donation_id, donor_tax_id } }`; the FE
 * contract expects `{ data: { national_id } }` — renamed here so the field
 * name matches lib/donations.ts DonationDetail conventions.
 */
export async function GET(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
): Promise<Response> {
  const { id } = await params;

  const goRes = await bffForward(request, `/api/donations/${id}/pii`);
  if (goRes.status !== 200) {
    // 401/403/404 — pass through Go's status + body unchanged.
    return goRes;
  }

  const body = (await goRes.json()) as {
    data?: { donation_id?: string; donor_tax_id?: string };
  };

  return NextResponse.json(
    { data: { national_id: body.data?.donor_tax_id ?? null } },
    { status: 200 }
  );
}
