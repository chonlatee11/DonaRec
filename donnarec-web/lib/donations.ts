import { apiFetch, DonnaRecApiError } from "@/lib/api";
import type { ApiError } from "@/lib/api";

// ---------------------------------------------------------------------------
// Status type (keep in sync with StatusBadge DonationStatus)
// ---------------------------------------------------------------------------

export type DonationStatus =
  | "draft"
  | "pending_review"
  | "issued"
  | "rejected"
  | "cancelled";

// ---------------------------------------------------------------------------
// API response types
// ---------------------------------------------------------------------------

export interface ReviewHistoryEntry {
  id: string;
  action: "return" | "reject";
  reason: string;
  actor_name: string;
  acted_at: string; // ISO datetime
}

/** Summarised donation row used on the list screen (Screen 1) */
export interface DonationSummary {
  id: string;
  donated_at: string;            // ISO date (YYYY-MM-DD)
  donor_name: string;
  /**
   * D-R2 / 03-09: backend serialises monetary amounts as a numeric STRING
   * (e.g. "1500.00") to preserve exact decimal precision. Callers parse with
   * parseFloat/Number at the render boundary.
   */
  amount: string;
  status: DonationStatus;
  /** Non-null only for status=issued or status=cancelled */
  receipt_formatted: string | null;
  created_by: string;            // display name
  created_by_id: string;         // Keycloak UUID of creator
}

/** Full donation detail used on Screen 3 */
export interface DonationDetail extends DonationSummary {
  /** Server always returns masked value; plaintext available via /pii endpoint (03-06) */
  national_id_masked: string;    // format: x-xxxx-xxxxx-1234
  address: string;
  email: string | null;
  note: string | null;
  /** Presigned object-storage URL (short TTL) or null if no slip attached */
  slip_url: string | null;
  consent_at: string | null;     // ISO datetime or null
  review_history: ReviewHistoryEntry[];
  replaces: { id: string; receipt_formatted: string } | null;
  replaced_by: { id: string; receipt_formatted: string } | null;
  edonation_keyed: boolean;
  /**
   * Server-computed authorization flags (T-03-31: Go API is the authority).
   * UI hides/shows controls based on these; server re-enforces on every mutation.
   */
  viewer_is_creator: boolean;
  can_approve: boolean;
  can_return: boolean;
  can_reject: boolean;
  /** True for Checker and Admin roles (T-03-32: reveal is audited server-side) */
  can_reveal_pii: boolean;
}

export interface DonationListResponse {
  items: DonationSummary[];
  total: number;
  page: number;
  per_page: number;
}

// ---------------------------------------------------------------------------
// Filter types
// ---------------------------------------------------------------------------

export interface SearchFilter {
  name?: string;
  /**
   * D-53: search scope is name / date range / status / receipt_no ONLY.
   * national/tax ID is intentionally excluded to prevent PII search.
   */
  status?: DonationStatus | "";
  from?: string;       // YYYY-MM-DD
  to?: string;         // YYYY-MM-DD
  receipt_no?: string; // exact match (FR-10)
  page?: number;
}

// ---------------------------------------------------------------------------
// API functions (server-side — apiFetch requires server context via getServerSession)
// ---------------------------------------------------------------------------

/**
 * Build the donation list query string from a SearchFilter.
 * D-53: Only name / date range / status / receipt_no / page are emitted —
 * national/tax ID is intentionally never a search key (no PII search).
 */
export function buildDonationQuery(filter: SearchFilter): string {
  const params = new URLSearchParams();
  if (filter.name) params.set("name", filter.name);
  if (filter.status) params.set("status", filter.status);
  if (filter.from) params.set("from", filter.from);
  if (filter.to) params.set("to", filter.to);
  if (filter.receipt_no) params.set("receipt_no", filter.receipt_no);
  if (filter.page && filter.page > 1) params.set("page", String(filter.page));
  return params.toString();
}

/**
 * GET /api/donations — paginated list with filters (SERVER-side via apiFetch).
 *
 * apiFetch unwraps the D-R2 `{data:...}` envelope, so this resolves to the inner
 * `{items,total,page,per_page}` payload directly.
 */
export async function searchDonations(
  filter: SearchFilter
): Promise<DonationListResponse> {
  const qs = buildDonationQuery(filter);
  return apiFetch<DonationListResponse>(`/api/donations${qs ? `?${qs}` : ""}`);
}

/**
 * fetchDonations — CLIENT-side list fetcher for TanStack Query (D-R1).
 *
 * Calls the same-origin BFF Route Handler `/api/bff/donations`, which obtains
 * the Keycloak token server-side and forwards to the Go API. The access token
 * therefore never reaches the browser. The BFF passes the Go `{data:{...}}`
 * envelope through unchanged, so we unwrap `.data` here.
 */
export async function fetchDonations(
  filter: SearchFilter
): Promise<DonationListResponse> {
  const qs = buildDonationQuery(filter);
  const res = await fetch(`/api/bff/donations${qs ? `?${qs}` : ""}`, {
    method: "GET",
    headers: { Accept: "application/json" },
  });

  if (!res.ok) {
    let message = "ไม่สามารถโหลดรายการบริจาคได้ — กรุณาลองอีกครั้ง";
    try {
      const body = (await res.json()) as { message?: string };
      if (body?.message) message = body.message;
    } catch {
      // keep default message
    }
    throw new Error(message);
  }

  const body = (await res.json()) as { data?: DonationListResponse };
  // BFF returns the Go envelope { data: { items, total, page, per_page } }.
  return (body.data ?? (body as unknown as DonationListResponse));
}

/**
 * GET /api/donations/:id — full record including server-computed authorization flags.
 * SERVER-side (apiFetch). Still used by the edit page (app/donations/[id]/edit/page.tsx)
 * to seed initial form values. The detail/review screen (Screen 3) uses the
 * CLIENT-side fetchDonation below instead (03-12).
 */
export async function getDonation(id: string): Promise<DonationDetail> {
  return apiFetch<DonationDetail>(`/api/donations/${id}`);
}

// ---------------------------------------------------------------------------
// Client-side BFF fetchers (03-12) — used by DonationDetailView (TanStack Query).
//
// D-R1: calls the same-origin BFF Route Handlers under app/api/bff/donations/[id]/**,
// which obtain the Keycloak token server-side and forward it to the Go API. The
// access token never reaches the browser. Errors are thrown as DonnaRecApiError
// (same shape as the server-side apiFetch) so callers can branch on .error.type.
// ---------------------------------------------------------------------------

function mapBffError(status: number, body: Record<string, unknown>): ApiError {
  switch (status) {
    case 403:
      // SoD violation: Go API returns code="SOD_VIOLATION"
      if (body?.code === "SOD_VIOLATION") {
        return {
          type: "sod",
          status: 403,
          message:
            (body.message as string) ??
            "คุณเป็นผู้สร้างรายการนี้ — ผู้อนุมัติต้องเป็นบุคคลอื่น (หลักการแยกหน้าที่)",
          details: body,
        };
      }
      return {
        type: "forbidden",
        status: 403,
        message: (body.message as string) ?? "ไม่มีสิทธิ์ดำเนินการ",
        details: body,
      };
    case 409:
      return {
        type: "statusConflict",
        status: 409,
        message:
          (body.message as string) ??
          "รายการนี้ได้รับการดำเนินการแล้ว — กรุณาโหลดหน้าใหม่เพื่อดูสถานะล่าสุด",
        details: body,
      };
    case 422:
      return {
        type: "validation",
        status: 422,
        message: (body.message as string) ?? "กรุณาระบุเหตุผลก่อนดำเนินการ",
        details: body,
      };
    case 413:
      // Slip upload — file exceeds the 10 MB server limit (T-03-15).
      return {
        type: "validation",
        status: 413,
        message: (body.message as string) ?? "ขนาดไฟล์เกิน 10 MB",
        details: body,
      };
    case 415:
      // Slip upload — magic-byte check rejected the file type (T-03-14).
      return {
        type: "validation",
        status: 415,
        message: (body.message as string) ?? "ประเภทไฟล์ไม่รองรับ (รองรับ JPG/PNG/PDF เท่านั้น)",
        details: body,
      };
    default:
      return {
        type: "network",
        status,
        message:
          (body.message as string) ??
          "บันทึกไม่สำเร็จ — กรุณาตรวจสอบการเชื่อมต่อและลองอีกครั้ง",
        details: body,
      };
  }
}

async function bffClientFetch<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, {
      ...init,
      headers: {
        Accept: "application/json",
        ...(init?.body ? { "Content-Type": "application/json" } : {}),
        ...(init?.headers as Record<string, string> | undefined),
      },
    });
  } catch (err) {
    throw new DonnaRecApiError({
      type: "network",
      status: 0,
      message: "บันทึกไม่สำเร็จ — กรุณาตรวจสอบการเชื่อมต่อและลองอีกครั้ง",
      details: err,
    });
  }

  const text = await res.text();
  let parsed: unknown = null;
  if (text) {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = null;
    }
  }

  if (!res.ok) {
    const body = (parsed ?? {}) as Record<string, unknown>;
    throw new DonnaRecApiError(mapBffError(res.status, body));
  }

  if (res.status === 204) return undefined as T;

  const body = parsed as { data?: T } | null;
  return (body?.data ?? (parsed as T)) as T;
}

/**
 * bffClientFetchFormData — like bffClientFetch but for multipart uploads
 * (03-13 slip). Does NOT set Content-Type — the browser generates the
 * multipart boundary from the FormData body (T-03-35 client-side; the slip
 * BFF route re-posts a fresh FormData to Go the same way).
 */
async function bffClientFetchFormData<T>(
  path: string,
  formData: FormData,
  method: "POST" = "POST"
): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, {
      method,
      body: formData,
    });
  } catch (err) {
    throw new DonnaRecApiError({
      type: "network",
      status: 0,
      message: "บันทึกไม่สำเร็จ — กรุณาตรวจสอบการเชื่อมต่อและลองอีกครั้ง",
      details: err,
    });
  }

  const text = await res.text();
  let parsed: unknown = null;
  if (text) {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = null;
    }
  }

  if (!res.ok) {
    const body = (parsed ?? {}) as Record<string, unknown>;
    throw new DonnaRecApiError(mapBffError(res.status, body));
  }

  if (res.status === 204) return undefined as T;

  const body = parsed as { data?: T } | null;
  return (body?.data ?? (parsed as T)) as T;
}

/**
 * apiErrorMessage — extracts a user-facing Thai message from any error thrown
 * by a client BFF call. Shared by DonationForm/DonationDetailView (03-13) so
 * both surfaces render the same message shape for the same server error.
 */
export function apiErrorMessage(err: unknown): string {
  if (err instanceof DonnaRecApiError) return err.error.message;
  if (err instanceof Error) return err.message;
  return "เกิดข้อผิดพลาด";
}

/**
 * fetchDonation — CLIENT-side detail fetcher for TanStack Query (D-R1, 03-12).
 * Calls the same-origin BFF `/api/bff/donations/:id`, which composes `slip_url`
 * server-side and unwraps to the DonationDetailResponse contract (03-11).
 */
export async function fetchDonation(id: string): Promise<DonationDetail> {
  return bffClientFetch<DonationDetail>(`/api/bff/donations/${id}`);
}

/**
 * approve — CLIENT-side mutation via the BFF (D-R1, 03-12).
 * Server enforces SoD (approver != creator). UI check is UX-only (T-03-31).
 */
export async function approve(id: string): Promise<DonationDetail> {
  return bffClientFetch<DonationDetail>(`/api/bff/donations/${id}/approve`, {
    method: "POST",
  });
}

/**
 * returnForEdit — CLIENT-side mutation via the BFF (D-R1, 03-12).
 * Reason is mandatory (FR-12, UI-SPEC Copywriting Contract).
 */
export async function returnForEdit(
  id: string,
  reason: string
): Promise<DonationDetail> {
  return bffClientFetch<DonationDetail>(`/api/bff/donations/${id}/return`, {
    method: "POST",
    body: JSON.stringify({ reason }),
  });
}

/**
 * reject — CLIENT-side mutation via the BFF (D-R1, 03-12).
 * Reason is mandatory (FR-12).
 */
export async function reject(id: string, reason: string): Promise<DonationDetail> {
  return bffClientFetch<DonationDetail>(`/api/bff/donations/${id}/reject`, {
    method: "POST",
    body: JSON.stringify({ reason }),
  });
}

/**
 * revealPII — CLIENT-side audited reveal fetcher via the BFF (D-R1, 03-12).
 * Server creates an audit log entry on every call (T-03-32/T-12-01).
 * Only available to Checker / Admin (can_reveal_pii = true).
 */
export async function revealPII(id: string): Promise<{ national_id: string }> {
  return bffClientFetch<{ national_id: string }>(`/api/bff/donations/${id}/pii`);
}

// ---------------------------------------------------------------------------
// Request types (03-08 additions)
// ---------------------------------------------------------------------------

/** Body for POST /api/donations (create draft) */
export interface CreateDonationRequest {
  donor_name: string;
  /** Plaintext 13-digit national/tax ID — encrypted by Go service before DB write */
  national_id: string;
  address: string;
  email?: string;
  amount: number;
  /** ISO date YYYY-MM-DD */
  donated_at: string;
  note?: string;
  consent_given: boolean;
  /** e.g. "1.0" — shown in ConsentBlock per D-49 */
  consent_text_version?: string;
}

/**
 * Body for PUT /api/donations/:id (update draft).
 *
 * national_id is REQUIRED here (matches CreateDonationRequest) — the Go
 * UpdateDraftRequest re-encrypts the tax ID on EVERY update and its validator
 * rejects an empty value (`ErrMissingTaxID` / 422 `missing_tax_id`); there is
 * no "leave blank to keep existing" path server-side. Because GetByID only
 * ever returns the masked value (T-03-09 — plaintext is never returned, even
 * to the creating Maker), the edit form must have the Maker RE-ENTER the full
 * 13-digit ID on every save (D-R3 contract-alignment fix).
 */
export interface UpdateDraftRequest {
  donor_name: string;
  national_id: string;
  address: string;
  email?: string;
  amount: number;
  donated_at: string;
  note?: string;
  consent_given: boolean;
  consent_text_version?: string;
}

export interface SlipViewResponse {
  url: string;
  /** 900 = 15-minute presigned TTL (T-03-16) */
  expires_in_seconds: number;
}

/** Body for POST /api/donations/:id/cancel and POST /:id/reissue */
export interface CancelDonationRequest {
  reason: string;
  /** Required only when edonation_keyed=true (D-51 / ErrEDonationKeyedCancel) */
  rd_confirmation_reason?: string;
}

// ---------------------------------------------------------------------------
// Mutation functions (CLIENT-side via the BFF — D-R1, 03-13)
//
// All of these now call the same-origin `/api/bff/donations/**` routes added
// in 03-13 instead of talking to the Go API directly — the Keycloak access
// token stays server-side inside the BFF route handler and never reaches the
// browser. The BFF also maps FE field names (national_id/address/email/note)
// to the Go contract (donor_tax_id/donor_address/donor_email/notes) for
// create/update/reissue, so these functions keep the FE field names.
// ---------------------------------------------------------------------------

/**
 * POST /api/bff/donations — create a new draft (FR-07).
 * Maker role required. national_id encrypted server-side (T-03-08 / D-44).
 */
export async function createDonation(
  body: CreateDonationRequest
): Promise<DonationDetail> {
  return bffClientFetch<DonationDetail>("/api/bff/donations", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

/**
 * PUT /api/bff/donations/:id — update an existing draft (FR-09).
 * Only allowed in draft status (ErrInvalidTransition→409 otherwise).
 * national_id is always re-encrypted (see UpdateDraftRequest doc comment).
 */
export async function updateDraft(
  id: string,
  body: UpdateDraftRequest
): Promise<DonationDetail> {
  return bffClientFetch<DonationDetail>(`/api/bff/donations/${id}`, {
    method: "PUT",
    body: JSON.stringify(body),
  });
}

/**
 * POST /api/bff/donations/:id/submit — transition draft → pending_review.
 * Row-level lock ensures gap-less counter safety (D-45).
 */
export async function submitDonation(id: string): Promise<DonationDetail> {
  return bffClientFetch<DonationDetail>(`/api/bff/donations/${id}/submit`, {
    method: "POST",
  });
}

// ---------------------------------------------------------------------------
// Slip functions (CLIENT-side via the BFF — D-R1, 03-13)
// ---------------------------------------------------------------------------

/**
 * POST /api/bff/donations/:id/slip (multipart/form-data, field "file").
 * Server performs magic-byte validation (T-03-14/T-03-15).
 * 413 → file too large; 415/422 → unsupported type (see mapBffError).
 */
export async function uploadSlip(
  id: string,
  formData: FormData
): Promise<void> {
  return bffClientFetchFormData<void>(`/api/bff/donations/${id}/slip`, formData);
}

/**
 * GET /api/bff/donations/:id/slip — returns a 15-min presigned URL (T-03-16).
 * 404 if no active slip (D-48 — cash donations may have none).
 */
export async function viewSlip(id: string): Promise<SlipViewResponse> {
  return bffClientFetch<SlipViewResponse>(`/api/bff/donations/${id}/slip`);
}

/**
 * DELETE /api/bff/donations/:id/slip — soft-delete (D-54).
 * The file is NEVER hard-deleted; DB record retains the object key.
 * Returns 204 No Content on success.
 */
export async function removeSlip(id: string): Promise<void> {
  return bffClientFetch<void>(`/api/bff/donations/${id}/slip`, {
    method: "DELETE",
  });
}

// ---------------------------------------------------------------------------
// Cancel / Reissue functions (Checker + Admin, CLIENT-side via the BFF — 03-13)
// ---------------------------------------------------------------------------

/**
 * POST /api/bff/donations/:id/cancel — void a receipt (Checker + Admin only).
 * Receipt number is RETAINED for audit trail (no gap created).
 * If edonation_keyed=true, rd_confirmation_reason is mandatory (D-51).
 * ErrEDonationKeyedCancel → 409 when rd_confirmation_reason is missing.
 */
export async function cancelDonation(
  id: string,
  body: CancelDonationRequest
): Promise<DonationDetail> {
  return bffClientFetch<DonationDetail>(`/api/bff/donations/${id}/cancel`, {
    method: "POST",
    body: JSON.stringify(body),
  });
}

/**
 * POST /api/bff/donations/:id/reissue — void + create replacement draft (D-50).
 * New draft requires normal Submit → Approve flow; gets a NEW receipt number.
 * If edonation_keyed=true, rd_confirmation_reason is mandatory (D-51).
 * The BFF composes the Go request's donor fields server-side from the
 * original record (see app/api/bff/donations/[id]/reissue/route.ts) — this
 * client body only ever carries the cancellation reason fields.
 */
export async function reissueDonation(
  id: string,
  body: CancelDonationRequest
): Promise<DonationDetail> {
  return bffClientFetch<DonationDetail>(`/api/bff/donations/${id}/reissue`, {
    method: "POST",
    body: JSON.stringify(body),
  });
}
