import type { NextRequest } from "next/server";
import { bffForward } from "@/lib/bff";

/**
 * POST /api/bff/settings/preview — BFF proxy for the 400ms-debounced sandboxed
 * HTML live preview (D-61, plan 04-08).
 *
 * D-R1: thin server-side Bearer forward via bffForward — Go re-enforces
 * Admin-only (adminGroup). The request body is the admin's CURRENT, UNSAVED
 * editor state (template_html/section6 text/images/language) assembled by
 * TemplateEditor's caller — it NEVER carries a donation id or any donor field
 * (D-61 mandate: preview uses sample/mock data only, T-04-26 mitigation).
 * Go returns { data: { html } }, passed through unchanged.
 */
export async function POST(request: NextRequest): Promise<Response> {
  return bffForward(request, "/api/admin/settings/preview");
}
