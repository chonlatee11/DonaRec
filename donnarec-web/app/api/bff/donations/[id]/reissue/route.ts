import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";
import {
  getBffToken,
  goFetch,
  mapFeDonorFieldsToGo,
  passthroughGoResponse,
  unauthenticatedResponse,
} from "@/lib/bff";

/**
 * POST /api/bff/donations/:id/reissue — BFF proxy + composition for Void &
 * Reissue (D-50, Checker/Admin only).
 *
 * The Go ReissueDonationRequest requires the FULL donor payload (donor_name/
 * donor_tax_id/donor_address/... — mirrors CreateDonationRequest) because it
 * both cancels the original AND creates the replacement draft in one call.
 * BUT the UI-SPEC's Void & Reissue dialog intentionally collects only a
 * free-text reason ("ระบุเหตุผลและข้อมูลที่ต้องแก้ไข") — NOT structured donor
 * fields — so the replacement draft starts as a copy of the original record
 * and gets corrected afterwards via the normal edit flow (same pattern as any
 * other draft). This route therefore composes the Go request server-side:
 *
 *   1. GET the original record's detail (non-PII donor fields).
 *   2. GET the original record's audited PII reveal (plaintext tax ID) —
 *      same Checker/Admin gate as reissue itself, so no privilege escalation.
 *   3. Merge with the client's {reason, rd_confirmation_reason?} and forward
 *      to Go POST /:id/reissue.
 *
 * Go 409 (edonation_keyed_confirmation_required) and 403 (forbidden/
 * insufficient_role) pass through unchanged at every step.
 */
export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
): Promise<Response> {
  const { id } = await params;
  const token = await getBffToken();
  if (!token) return unauthenticatedResponse();

  let clientBody: { reason?: string; rd_confirmation_reason?: string };
  try {
    clientBody = (await request.json()) as {
      reason?: string;
      rd_confirmation_reason?: string;
    };
  } catch {
    return NextResponse.json({ error: "invalid_request_body" }, { status: 400 });
  }

  // Step 1: original record detail (donor_name/address/email/amount/donated_at/note/consent).
  const detailRes = await goFetch(token, `/api/donations/${id}`);
  if (detailRes.status !== 200) {
    return passthroughGoResponse(detailRes);
  }
  const detailBody = (await detailRes.json()) as { data?: Record<string, unknown> };
  const detail = detailBody.data ?? {};

  // Step 2: audited plaintext PII reveal (Checker/Admin-gated — same as reissue).
  const piiRes = await goFetch(token, `/api/donations/${id}/pii`);
  if (piiRes.status !== 200) {
    // 403 here means the caller isn't Checker/Admin — the reissue route itself
    // would reject them too, so surfacing this 403 first is correct.
    return passthroughGoResponse(piiRes);
  }
  const piiBody = (await piiRes.json()) as { data?: { donor_tax_id?: string } };
  const donorTaxId = piiBody.data?.donor_tax_id ?? "";

  // Step 3: compose and forward.
  const amountRaw = detail.amount;
  const goBody = {
    reason: clientBody.reason ?? "",
    rd_confirmation_reason: clientBody.rd_confirmation_reason ?? "",
    ...mapFeDonorFieldsToGo({
      donor_name: (detail.donor_name as string | undefined) ?? "",
      national_id: donorTaxId,
      address: (detail.address as string | undefined) ?? "",
      email: (detail.email as string | null | undefined) ?? undefined,
      amount:
        typeof amountRaw === "string"
          ? parseFloat(amountRaw)
          : typeof amountRaw === "number"
          ? amountRaw
          : 0,
      donated_at: (detail.donated_at as string | undefined) ?? "",
      note: (detail.note as string | null | undefined) ?? undefined,
      consent_given: (detail.consent_given as boolean | undefined) ?? true,
      consent_text_version:
        (detail.consent_text_version as string | null | undefined) ?? undefined,
    }),
  };

  const goRes = await goFetch(token, `/api/donations/${id}/reissue`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(goBody),
  });

  return passthroughGoResponse(goRes);
}
