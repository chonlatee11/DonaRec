"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { format } from "date-fns";
import { FileSpreadsheet, FileText, CalendarIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Calendar } from "@/components/ui/calendar";
import { useToast } from "@/hooks/use-toast";
import { ExportConfirmDialog } from "@/components/ExportConfirmDialog";
import {
  downloadExport,
  fetchAging,
  type ExportFormat,
  type KeyedStatusFilter,
} from "@/lib/edonation";

/**
 * ExportPanel — Screen 7 Tab A (Export Data, FR-30, D-62/D-64/D-66/D-74).
 *
 * Filter bar (issued-date range via the existing Calendar Popover fields +
 * keyed-status Select, default "ยังไม่คีย์") + a live record-count preview +
 * the two export buttons, each opening ExportConfirmDialog before triggering
 * the actual streamed download from the BFF.
 *
 * Count-preview data source: the Go API has no dedicated count-only/dry-run
 * endpoint (calling the audited Export endpoint itself to "preview" would
 * write a spurious audit_log row and decrypt PII on every filter keystroke —
 * unacceptable). Instead this reuses the SAME `GET /api/bff/edonation/aging`
 * query the Aging tab (AgingTable) already fetches (shared TanStack Query
 * cache key `["edonationAging"]` — a single network request serves both
 * tabs) and derives an EXACT client-side count by filtering its rows by the
 * selected date range — this is correct because Aging's row set (all
 * unkeyed issued donations) is exactly the candidate set for keyedStatus
 * "not_keyed" (the UI-SPEC default / primary export workflow). For
 * keyedStatus "all"/"keyed" the Aging endpoint's rows cannot yield an
 * accurate count (it never includes keyed rows), so the preview is hidden
 * rather than showing a fabricated/undercounted number — the export
 * buttons stay enabled and the backend's own empty-result 404 remains the
 * true safety net against a zero-row download round trip.
 */
export function ExportPanel() {
  const t = useTranslations("eDonationExport");
  const { toast } = useToast();

  const [from, setFrom] = useState<Date | undefined>(undefined);
  const [to, setTo] = useState<Date | undefined>(undefined);
  const [keyedStatus, setKeyedStatus] = useState<KeyedStatusFilter>("not_keyed");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [pendingFormat, setPendingFormat] = useState<ExportFormat | null>(null);
  const [isDownloading, setIsDownloading] = useState(false);

  // Shared cache key with AgingTable — one request serves both tabs.
  const { data: agingData } = useQuery({
    queryKey: ["edonationAging"],
    queryFn: fetchAging,
  });

  const previewCount = useMemo<number | null>(() => {
    if (keyedStatus !== "not_keyed" || !agingData) return null;
    const fromStr = from ? format(from, "yyyy-MM-dd") : null;
    const toStr = to ? format(to, "yyyy-MM-dd") : null;
    return agingData.rows.filter((row) => {
      const approvedDate = row.approved_at.slice(0, 10);
      if (fromStr && approvedDate < fromStr) return false;
      if (toStr && approvedDate > toStr) return false;
      return true;
    }).length;
  }, [agingData, from, to, keyedStatus]);

  const zeroCount = previewCount === 0;

  function openConfirm(fmt: ExportFormat) {
    setPendingFormat(fmt);
    setDialogOpen(true);
  }

  async function handleConfirm() {
    if (!pendingFormat) return;
    setIsDownloading(true);
    try {
      await downloadExport(
        {
          from: from ? format(from, "yyyy-MM-dd") : undefined,
          to: to ? format(to, "yyyy-MM-dd") : undefined,
          keyedStatus,
        },
        pendingFormat
      );
      setDialogOpen(false);
      toast({
        description:
          previewCount !== null
            ? t("successToast", { n: previewCount })
            : t("successToastNoCount"),
      });
    } catch (err) {
      const message =
        err instanceof Error && err.message === "no_records"
          ? t("noRecordsError")
          : t("exportFailed");
      toast({ variant: "destructive", description: message });
    } finally {
      setIsDownloading(false);
      setPendingFormat(null);
    }
  }

  return (
    <div className="flex flex-col gap-6">
      {/* Filter bar */}
      <div className="flex flex-wrap gap-3 items-end rounded-lg border border-slate-200 bg-white p-4">
        <div className="flex flex-col gap-1">
          <label className="text-[14px] font-normal text-slate-600">
            {t("filterFrom")}
          </label>
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
          <label className="text-[14px] font-normal text-slate-600">
            {t("filterTo")}
          </label>
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

        <div className="flex min-w-[170px] flex-col gap-1">
          <label className="text-[14px] font-normal text-slate-600">
            {t("keyedStatusLabel")}
          </label>
          <Select
            value={keyedStatus}
            onValueChange={(v) => setKeyedStatus(v as KeyedStatusFilter)}
          >
            <SelectTrigger className="min-h-[44px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("keyedStatusAll")}</SelectItem>
              <SelectItem value="not_keyed">{t("keyedStatusNotKeyed")}</SelectItem>
              <SelectItem value="keyed">{t("keyedStatusKeyed")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      {/* Record-count preview */}
      <p className="text-[14px] text-slate-600" aria-live="polite">
        {previewCount !== null
          ? t("countPreview", { n: previewCount })
          : t("countPreviewUnavailable")}
      </p>

      {/* Export buttons */}
      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap gap-3">
          <Button
            type="button"
            className="min-h-[44px] bg-blue-600 text-white hover:bg-blue-700"
            disabled={zeroCount || isDownloading}
            aria-busy={isDownloading && pendingFormat === "xlsx"}
            onClick={() => openConfirm("xlsx")}
          >
            <FileSpreadsheet className="mr-2 h-4 w-4" />
            {isDownloading && pendingFormat === "xlsx"
              ? t("generating")
              : t("exportExcel")}
          </Button>
          <Button
            type="button"
            variant="outline"
            className="min-h-[44px]"
            disabled={zeroCount || isDownloading}
            aria-busy={isDownloading && pendingFormat === "csv"}
            onClick={() => openConfirm("csv")}
          >
            <FileText className="mr-2 h-4 w-4" />
            {isDownloading && pendingFormat === "csv"
              ? t("generating")
              : t("exportCsv")}
          </Button>
        </div>
        {zeroCount && (
          <p className="text-[14px] text-slate-600" role="status">
            {t("noRecordsHeading")} — {t("noRecordsBody")}
          </p>
        )}
      </div>

      <ExportConfirmDialog
        open={dialogOpen}
        onOpenChange={(open) => {
          if (!isDownloading) setDialogOpen(open);
        }}
        count={previewCount}
        isSubmitting={isDownloading}
        onConfirm={() => void handleConfirm()}
      />
    </div>
  );
}
