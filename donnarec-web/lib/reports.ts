// ---------------------------------------------------------------------------
// Donation Summary Reports — CLIENT-side BFF fetchers (Screen 8, FR-32,
// plan 05-07).
//
// D-R1: all calls go through the same-origin `/api/bff/reports*` routes
// (app/api/bff/reports/**), which obtain the Keycloak access token
// server-side and forward it to the Go API. The access token never reaches
// the browser. Mirrors lib/edonation.ts's client-fetcher conventions (plain
// Error with a Thai message on failure, `{data:...}` envelope unwrapping).
//
// D-71: this report has NO RBAC gate — every authenticated staff member can
// call these routes, and the underlying data carries no PII column.
// ---------------------------------------------------------------------------

export type ReportGroupBy = "month" | "day";

/** One breakdown row — mirrors PeriodRowResponse (Go). */
export interface PeriodRow {
  /** "YYYY-MM-DD" — first day of the month for monthly, exact day for daily. */
  period: string;
  receipt_count: number;
  total_amount: number;
}

/** GET /api/reports/summary response shape — mirrors SummaryResponse (Go). */
export interface SummaryResult {
  total_amount: number;
  receipt_count: number;
  average_per_receipt: number;
  breakdown: PeriodRow[];
}

export interface ReportFilters {
  /** ISO date YYYY-MM-DD */
  from?: string;
  /** ISO date YYYY-MM-DD */
  to?: string;
  groupBy: ReportGroupBy;
}

export type ReportExportFormat = "xlsx" | "csv";

async function parseErrorMessage(res: Response, fallback: string): Promise<string> {
  try {
    const body = (await res.json()) as { message?: string };
    return body?.message ?? fallback;
  } catch {
    return fallback;
  }
}

/**
 * buildReportQuery — builds the query string shared by GET
 * /api/bff/reports and /api/bff/reports/export (from/to/group_by).
 */
export function buildReportQuery(filters: ReportFilters): string {
  const params = new URLSearchParams();
  if (filters.from) params.set("from", filters.from);
  if (filters.to) params.set("to", filters.to);
  params.set("group_by", filters.groupBy);
  return params.toString();
}

/**
 * currentFiscalYearDateRange — the Thai government fiscal year (1 Oct – 30
 * Sep) as a concrete { from, to } Date range for `now`, used as the Screen 8
 * filter bar's default (UI-SPEC Screen 8: "default: current fiscal year").
 * Mirrors lib/receipt-number-format.ts's currentFiscalYearBE() rollover rule
 * (Oct–Dec of CE year Y belongs to the fiscal year starting that October).
 */
export function currentFiscalYearDateRange(now: Date = new Date()): { from: Date; to: Date } {
  const ceYear = now.getFullYear();
  const startYear = now.getMonth() >= 9 ? ceYear : ceYear - 1; // October === 9 (0-indexed)
  return {
    from: new Date(startYear, 9, 1), // 1 Oct
    to: new Date(startYear + 1, 8, 30), // 30 Sep next year
  };
}

/**
 * fetchReportSummary — CLIENT-side fetcher for GET /api/bff/reports
 * (FR-32/D-70/D-71).
 */
export async function fetchReportSummary(filters: ReportFilters): Promise<SummaryResult> {
  const qs = buildReportQuery(filters);
  const res = await fetch(`/api/bff/reports?${qs}`, {
    method: "GET",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) {
    throw new Error(
      await parseErrorMessage(res, "โหลดรายงานไม่สำเร็จ — กรุณาตรวจสอบการเชื่อมต่อและลองอีกครั้ง")
    );
  }
  const raw = (await res.json()) as { data?: SummaryResult };
  return (
    raw.data ?? { total_amount: 0, receipt_count: 0, average_per_receipt: 0, breakdown: [] }
  );
}

/** ASCII filename Go's report Export handler sets (internal/report/handler.go). */
export function reportExportFileName(format: ReportExportFormat): string {
  return `donation-report.${format}`;
}

/**
 * downloadReportExport — triggers the browser download for GET
 * /api/bff/reports/export via a same-origin fetch + blob anchor click.
 * D-70: zero PII in this report — NO confirmation dialog is shown before
 * this is called (contrast with lib/edonation.ts's downloadExport, which is
 * always gated behind ExportConfirmDialog).
 */
export async function downloadReportExport(
  filters: ReportFilters,
  format: ReportExportFormat
): Promise<void> {
  const params = new URLSearchParams(buildReportQuery(filters));
  params.set("format", format);
  const res = await fetch(`/api/bff/reports/export?${params.toString()}`, {
    method: "GET",
  });

  if (!res.ok) {
    throw new Error(
      await parseErrorMessage(res, "ส่งออกรายงานไม่สำเร็จ — กรุณาลองใหม่อีกครั้ง")
    );
  }

  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = reportExportFileName(format);
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}
