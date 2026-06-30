import { apiFetch } from "@/lib/api";

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
  amount: number;
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
  donations: DonationSummary[];
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
 * GET /api/donations — paginated list with filters.
 *
 * D-53: Only name, date range, status, receipt_no are searchable.
 * national/tax ID search is intentionally absent.
 */
export async function searchDonations(
  filter: SearchFilter
): Promise<DonationListResponse> {
  const params = new URLSearchParams();
  if (filter.name) params.set("name", filter.name);
  if (filter.status) params.set("status", filter.status);
  if (filter.from) params.set("from", filter.from);
  if (filter.to) params.set("to", filter.to);
  if (filter.receipt_no) params.set("receipt_no", filter.receipt_no);
  if (filter.page && filter.page > 1) params.set("page", String(filter.page));

  const qs = params.toString();
  return apiFetch<DonationListResponse>(`/api/donations${qs ? `?${qs}` : ""}`);
}

/** GET /api/donations/:id — full record including server-computed authorization flags */
export async function getDonation(id: string): Promise<DonationDetail> {
  return apiFetch<DonationDetail>(`/api/donations/${id}`);
}

/**
 * POST /api/donations/:id/approve — Checker approves a pending_review record.
 * Server enforces SoD (approver != creator). UI check is UX-only (T-03-31).
 */
export async function approve(id: string): Promise<void> {
  return apiFetch<void>(`/api/donations/${id}/approve`, { method: "POST" });
}

/**
 * POST /api/donations/:id/return — Checker returns a record to Maker with reason.
 * Reason is mandatory (FR-12, UI-SPEC Copywriting Contract).
 */
export async function returnForEdit(id: string, reason: string): Promise<void> {
  return apiFetch<void>(`/api/donations/${id}/return`, {
    method: "POST",
    body: JSON.stringify({ reason }),
  });
}

/**
 * POST /api/donations/:id/reject — Checker permanently rejects a record.
 * Reason is mandatory (FR-12).
 */
export async function reject(id: string, reason: string): Promise<void> {
  return apiFetch<void>(`/api/donations/${id}/reject`, {
    method: "POST",
    body: JSON.stringify({ reason }),
  });
}

/**
 * GET /api/donations/:id/pii — reveal plaintext national/tax ID.
 * Server creates an audit log entry on every call (T-03-32).
 * Only available to Checker / Admin (can_reveal_pii = true).
 */
export async function revealPII(id: string): Promise<{ national_id: string }> {
  return apiFetch<{ national_id: string }>(`/api/donations/${id}/pii`);
}
