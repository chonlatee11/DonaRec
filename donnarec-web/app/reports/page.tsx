"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { format } from "date-fns";
import { CalendarIcon, FileSpreadsheet, FileText } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Calendar } from "@/components/ui/calendar";
import { Skeleton } from "@/components/ui/skeleton";
import { useToast } from "@/hooks/use-toast";
import { ReportSummaryCards } from "@/components/ReportSummaryCards";
import { ReportBreakdownTable } from "@/components/ReportBreakdownTable";
import {
  currentFiscalYearDateRange,
  downloadReportExport,
  fetchReportSummary,
  type ReportExportFormat,
  type ReportGroupBy,
} from "@/lib/reports";

/**
 * Donation Summary Reports page — Screen 8 (FR-32, D-70/D-71, plan 05-07).
 *
 * Visible to ALL staff (Maker/Checker/Admin) — no RBAC gate on the route or
 * nav item (D-71: zero PII in this report). Owns the filter/apply state,
 * the summary query (shared across the cards + breakdown table so switching
 * the month/day toggle only re-fetches the breakdown granularity, not a
 * whole separate screen), and the PII-free export buttons (no confirmation
 * dialog, contrast with Screen 7's audited export).
 *
 * "use client" at the page level (not a Server Component + client child
 * split like app/admin/settings or app/e-donation) because this route has
 * no server-side RBAC redirect to perform — it goes straight into
 * TanStack-Query-backed interactive state.
 */
export default function ReportsPage() {
  const t = useTranslations("reports");
  const { toast } = useToast();

  const defaultRange = useMemo(() => currentFiscalYearDateRange(), []);

  const [from, setFrom] = useState<Date | undefined>(defaultRange.from);
  const [to, setTo] = useState<Date | undefined>(defaultRange.to);
  const [appliedFrom, setAppliedFrom] = useState<Date | undefined>(defaultRange.from);
  const [appliedTo, setAppliedTo] = useState<Date | undefined>(defaultRange.to);
  const [groupBy, setGroupBy] = useState<ReportGroupBy>("month");
  const [downloadingFormat, setDownloadingFormat] = useState<ReportExportFormat | null>(null);

  const fromStr = appliedFrom ? format(appliedFrom, "yyyy-MM-dd") : undefined;
  const toStr = appliedTo ? format(appliedTo, "yyyy-MM-dd") : undefined;

  const { data, isLoading, isError } = useQuery({
    queryKey: ["reportSummary", fromStr, toStr, groupBy],
    queryFn: () => fetchReportSummary({ from: fromStr, to: toStr, groupBy }),
  });

  function handleApply() {
    setAppliedFrom(from);
    setAppliedTo(to);
  }

  function handleClear() {
    setFrom(undefined);
    setTo(undefined);
    setAppliedFrom(undefined);
    setAppliedTo(undefined);
  }

  async function handleExport(fmt: ReportExportFormat) {
    setDownloadingFormat(fmt);
    try {
      await downloadReportExport({ from: fromStr, to: toStr, groupBy }, fmt);
      toast({ description: t("exportSuccessToast") });
    } catch {
      toast({ variant: "destructive", description: t("exportFailed") });
    } finally {
      setDownloadingFormat(null);
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-[28px] font-semibold leading-tight text-slate-900">
        {t("pageTitle")}
      </h1>

      {/* Filter bar — explicit apply, not live-update-on-change (avoids
          re-querying aggregates on every keystroke/date-pick) */}
      <div className="flex flex-wrap gap-3 items-end rounded-lg border border-slate-200 bg-white p-4">
        <div className="flex flex-col gap-1">
          <label className="text-[14px] font-normal text-slate-600">{t("filterFrom")}</label>
          <Popover>
            <PopoverTrigger asChild>
              <Button
                variant="outline"
                className="min-h-[44px] min-w-[150px] justify-start text-left font-normal"
              >
                <CalendarIcon className="mr-2 h-4 w-4 text-slate-400" />
                {from ? (
                  format(from, "dd/MM/yyyy")
                ) : (
                  <span className="text-slate-400">{t("filterFrom")}</span>
                )}
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-auto p-0" align="start">
              <Calendar mode="single" selected={from} onSelect={setFrom} />
            </PopoverContent>
          </Popover>
        </div>

        <div className="flex flex-col gap-1">
          <label className="text-[14px] font-normal text-slate-600">{t("filterTo")}</label>
          <Popover>
            <PopoverTrigger asChild>
              <Button
                variant="outline"
                className="min-h-[44px] min-w-[150px] justify-start text-left font-normal"
              >
                <CalendarIcon className="mr-2 h-4 w-4 text-slate-400" />
                {to ? (
                  format(to, "dd/MM/yyyy")
                ) : (
                  <span className="text-slate-400">{t("filterTo")}</span>
                )}
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-auto p-0" align="start">
              <Calendar mode="single" selected={to} onSelect={setTo} />
            </PopoverContent>
          </Popover>
        </div>

        <div className="flex items-end gap-2">
          <Button
            type="button"
            onClick={handleApply}
            className="min-h-[44px] bg-blue-600 text-white hover:bg-blue-700"
          >
            {t("applyFilter")}
          </Button>
          <Button
            type="button"
            variant="ghost"
            onClick={handleClear}
            className="min-h-[44px] text-slate-600"
          >
            {t("clearFilter")}
          </Button>
        </div>
      </div>

      {isLoading ? (
        <div className="flex flex-col gap-4" aria-busy="true">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-[300px] w-full" />
        </div>
      ) : isError ? (
        <div
          className="rounded-lg border border-red-200 bg-red-50 p-4 text-[14px] text-red-600"
          role="alert"
        >
          {t("loadFailed")}
        </div>
      ) : (
        <>
          <ReportSummaryCards
            totalAmount={data?.total_amount ?? 0}
            receiptCount={data?.receipt_count ?? 0}
            averagePerReceipt={data?.average_per_receipt ?? 0}
          />

          <ReportBreakdownTable
            breakdown={data?.breakdown ?? []}
            groupBy={groupBy}
            onGroupByChange={setGroupBy}
          />

          {/* Export row — no confirmation dialog (D-70: zero PII) */}
          <div className="flex flex-wrap gap-3">
            <Button
              type="button"
              variant="outline"
              className="min-h-[44px]"
              disabled={downloadingFormat !== null}
              aria-busy={downloadingFormat === "xlsx"}
              onClick={() => void handleExport("xlsx")}
            >
              <FileSpreadsheet className="mr-2 h-4 w-4" />
              {downloadingFormat === "xlsx" ? t("generating") : t("exportExcel")}
            </Button>
            <Button
              type="button"
              variant="outline"
              className="min-h-[44px]"
              disabled={downloadingFormat !== null}
              aria-busy={downloadingFormat === "csv"}
              onClick={() => void handleExport("csv")}
            >
              <FileText className="mr-2 h-4 w-4" />
              {downloadingFormat === "csv" ? t("generating") : t("exportCsv")}
            </Button>
          </div>
        </>
      )}
    </div>
  );
}
