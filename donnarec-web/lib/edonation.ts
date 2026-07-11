// ---------------------------------------------------------------------------
// e-Donation Export + Keyed-Status/Aging — CLIENT-side BFF fetchers (Screen 7,
// FR-30/FR-31, plan 05-06).
//
// D-R1: all calls go through the same-origin `/api/bff/edonation/**` routes
// (app/api/bff/edonation/**), which obtain the Keycloak access token
// server-side and forward it to the Go API. The access token never reaches
// the browser (T-05-06-TOKEN). Mirrors lib/donations.ts's client-fetcher
// conventions (plain Error with a Thai message on failure, `{data:...}`
// envelope unwrapping).
// ---------------------------------------------------------------------------

/** Mirrors internal/edonation.AgingBucket (Go) — FR-31/D-68. */
export type AgingBucket = "not_due" | "near_due" | "overdue";

/** One bucketed unkeyed issued donation row — mirrors AgingRowResponse (Go). */
export interface AgingRow {
  id: string;
  donor_name: string;
  receipt_formatted: string;
  /** "YYYY-MM-DD" — the donation date; the SAME field the export endpoint
   * filters on (donated_at, D-66). Used by the Export tab's count preview so it
   * matches the actual export filter (WR-01). NOT the aging base date. */
  donated_at: string;
  /** ISO datetime — D-68 base field (donations has no issued_at column) */
  approved_at: string;
  /** ISO datetime — the 5th of the month after approved_at's month */
  deadline: string;
  bucket: AgingBucket;
  keyed: boolean;
}

/** GET /api/edonation/aging response shape — mirrors AgingResponse (Go). */
export interface AgingResult {
  rows: AgingRow[];
  counts: Partial<Record<AgingBucket, number>>;
}

/** Keyed-status filter values accepted by the Export tab's Select (D-66). */
export type KeyedStatusFilter = "all" | "keyed" | "not_keyed";

export type ExportFormat = "xlsx" | "csv";

export interface ExportFilters {
  /** ISO date YYYY-MM-DD */
  from?: string;
  /** ISO date YYYY-MM-DD */
  to?: string;
  keyedStatus: KeyedStatusFilter;
}

async function parseErrorMessage(res: Response, fallback: string): Promise<string> {
  try {
    const body = (await res.json()) as { message?: string };
    return body?.message ?? fallback;
  } catch {
    return fallback;
  }
}

/**
 * fetchAging — CLIENT-side fetcher for GET /api/bff/edonation/aging
 * (FR-31/D-68). Returns ALL unkeyed issued donations bucketed against the
 * config-driven near_due_days threshold — the Go handler does not paginate;
 * AgingTable paginates 20/page client-side.
 */
export async function fetchAging(): Promise<AgingResult> {
  const res = await fetch("/api/bff/edonation/aging", {
    method: "GET",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) {
    throw new Error(
      await parseErrorMessage(
        res,
        "โหลดข้อมูลติดตามสถานะคีย์ไม่สำเร็จ — กรุณาลองอีกครั้ง"
      )
    );
  }
  const raw = (await res.json()) as { data?: AgingResult };
  return raw.data ?? { rows: [], counts: {} };
}

/**
 * setKeyed — CLIENT-side mutation for POST /api/bff/edonation/keyed
 * (FR-31/D-67). Bulk or per-row (single-id array) mark/unmark. Every
 * mark/unmark writes one audit row per donation server-side (05-04) — no
 * client-side confirmation dialog is required (reversible boolean toggle,
 * not a PII-disclosure event, per the UI-SPEC Copywriting Contract).
 */
export async function setKeyed(
  donationIds: string[],
  keyed: boolean
): Promise<{ saved: boolean; keyed: boolean }> {
  const res = await fetch("/api/bff/edonation/keyed", {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify({ donation_ids: donationIds, keyed }),
  });
  if (!res.ok) {
    throw new Error(
      await parseErrorMessage(res, "บันทึกสถานะคีย์ไม่สำเร็จ — กรุณาลองใหม่อีกครั้ง")
    );
  }
  const raw = (await res.json()) as { data?: { saved: boolean; keyed: boolean } };
  return raw.data ?? { saved: true, keyed };
}

/**
 * buildExportQuery — builds the query string for GET /api/bff/edonation/export
 * (D-66). keyed_status is only sent for "keyed"/"not_keyed" (Go's
 * ?keyed_status=true|false); "all" omits the param entirely (no filter).
 */
export function buildExportQuery(filters: ExportFilters, format: ExportFormat): string {
  const params = new URLSearchParams();
  if (filters.from) params.set("from", filters.from);
  if (filters.to) params.set("to", filters.to);
  if (filters.keyedStatus === "keyed") params.set("keyed_status", "true");
  if (filters.keyedStatus === "not_keyed") params.set("keyed_status", "false");
  params.set("format", format);
  return params.toString();
}

/** ASCII filename Go's Export handler sets (internal/edonation/handler.go) — used as the client-side download's `download` attribute. */
export function exportFileName(format: ExportFormat): string {
  return `edonation-export.${format}`;
}

/**
 * downloadExport — triggers the browser download for GET
 * /api/bff/edonation/export via a same-origin fetch + blob anchor click
 * (D-74: stream-only, no server-side copy persists; the access token never
 * reaches the browser — this route is called same-origin with no auth
 * header attached by the client).
 *
 * Throws with a Thai message on 403 (forbidden)/404 (no_records)/network
 * error — callers map these to the UI-SPEC error copy.
 */
export async function downloadExport(
  filters: ExportFilters,
  format: ExportFormat
): Promise<void> {
  const qs = buildExportQuery(filters, format);
  const res = await fetch(`/api/bff/edonation/export?${qs}`, {
    method: "GET",
  });

  if (!res.ok) {
    if (res.status === 404) {
      throw new Error("no_records");
    }
    throw new Error(
      await parseErrorMessage(res, "ส่งออกข้อมูลไม่สำเร็จ — กรุณาลองใหม่อีกครั้ง")
    );
  }

  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = exportFileName(format);
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

// ---------------------------------------------------------------------------
// e-Donation admin config — CLIENT-side BFF fetchers (5th SettingsTabs tab,
// D-75/NFR-09, plan 05-02/05-07).
// ---------------------------------------------------------------------------

/** One ordered export column — mirrors FieldMappingColumn (Go, internal/edonation/config.go). */
export interface FieldMappingColumn {
  column_key: string;
  header_th: string;
  header_en: string;
}

/**
 * GET/PUT /api/bff/edonation-config response shape — mirrors ConfigResponse
 * (Go, internal/edonation/handler.go).
 */
export interface EdonationConfig {
  field_mapping: FieldMappingColumn[];
  cash_type_label: string;
  near_due_days: number;
  updated_at: string;
  updated_by: string;
}

/**
 * PUT request body — mirrors ConfigRequest (Go). The server-owned
 * updated_at/updated_by fields are never sent on save (mirrors
 * lib/settings.ts's SettingsFormValues Omit<> discipline).
 */
export type EdonationConfigFormValues = Pick<
  EdonationConfig,
  "field_mapping" | "cash_type_label" | "near_due_days"
>;

/**
 * fetchEdonationConfig — CLIENT-side fetcher for GET
 * /api/bff/edonation-config (D-75/NFR-09). Admin-only in practice: the Go
 * route this proxies is gated by adminGroup.RequireRoles(Admin); this
 * fetcher is only ever called from EdonationConfigTab, which only renders
 * inside the already-Admin-gated /admin/settings route.
 */
export async function fetchEdonationConfig(): Promise<EdonationConfig> {
  const res = await fetch("/api/bff/edonation-config", {
    method: "GET",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) {
    throw new Error(
      await parseErrorMessage(res, "โหลดการตั้งค่า e-Donation ไม่สำเร็จ — กรุณาลองอีกครั้ง")
    );
  }
  const raw = (await res.json()) as { data?: EdonationConfig };
  if (!raw.data) {
    throw new Error("โหลดการตั้งค่า e-Donation ไม่สำเร็จ — กรุณาลองอีกครั้ง");
  }
  return raw.data;
}

/**
 * saveEdonationConfig — CLIENT-side mutation for PUT
 * /api/bff/edonation-config (D-75/NFR-09). Persists field mapping order/
 * headers, cash_type_label (D-65), and near_due_days (D-68 aging threshold)
 * with no deploy required.
 */
export async function saveEdonationConfig(
  values: EdonationConfigFormValues
): Promise<void> {
  const res = await fetch("/api/bff/edonation-config", {
    method: "PUT",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(values),
  });
  if (!res.ok) {
    throw new Error(
      await parseErrorMessage(res, "บันทึกการตั้งค่า e-Donation ไม่สำเร็จ — กรุณาลองใหม่อีกครั้ง")
    );
  }
}
