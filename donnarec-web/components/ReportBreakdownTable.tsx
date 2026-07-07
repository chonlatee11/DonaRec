"use client";

import { useEffect, useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination";
import type { PeriodRow, ReportGroupBy } from "@/lib/reports";

/** Daily grouping paginates at 31 rows/page (max days in a month); monthly
 * grouping needs no cap (max 12 rows/year, UI-SPEC Screen 8). */
const PER_PAGE_DAILY = 31;

const amountFormatter = new Intl.NumberFormat("th-TH", {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});
const countFormatter = new Intl.NumberFormat("th-TH");

interface ReportBreakdownTableProps {
  breakdown: PeriodRow[];
  groupBy: ReportGroupBy;
  onGroupByChange: (groupBy: ReportGroupBy) => void;
}

/**
 * ReportBreakdownTable — Screen 8: month/day breakdown table with a
 * segmented group-by toggle (same 2-option Button-pair pattern as Phase 4's
 * TemplateEditor HTML/Real-PDF preview toggle — reused here, not a new
 * visual pattern).
 */
export function ReportBreakdownTable({
  breakdown,
  groupBy,
  onGroupByChange,
}: ReportBreakdownTableProps) {
  const t = useTranslations("reports");
  const locale = useLocale() as "th" | "en";
  const [page, setPage] = useState(1);

  // Reset to page 1 whenever the grouping or the underlying data changes —
  // avoids landing on a now-out-of-range page after a filter/toggle change.
  useEffect(() => {
    setPage(1);
  }, [groupBy, breakdown]);

  function formatPeriod(period: string): string {
    const date = new Date(`${period}T00:00:00Z`);
    const options: Intl.DateTimeFormatOptions =
      groupBy === "month"
        ? { year: "numeric", month: "short", timeZone: "UTC" }
        : { year: "numeric", month: "short", day: "numeric", timeZone: "UTC" };
    if (locale === "th") {
      return new Intl.DateTimeFormat("th-TH-u-ca-buddhist", options).format(date);
    }
    return new Intl.DateTimeFormat("en-GB", options).format(date);
  }

  const perPage = groupBy === "day" ? PER_PAGE_DAILY : Math.max(breakdown.length, 1);
  const totalPages = Math.max(1, Math.ceil(breakdown.length / perPage));
  const pagedRows = useMemo(
    () => breakdown.slice((page - 1) * perPage, page * perPage),
    [breakdown, page, perPage]
  );

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h2 className="text-[20px] font-semibold leading-snug text-slate-900">
          {t("breakdownHeading")}
        </h2>
        {/* Group-by segmented control (2-option toggle, not a Tabs navigation) */}
        <div className="flex gap-1">
          <Button
            type="button"
            size="sm"
            variant={groupBy === "month" ? "default" : "outline"}
            className={groupBy === "month" ? "bg-blue-600 text-white hover:bg-blue-700" : ""}
            onClick={() => onGroupByChange("month")}
          >
            {t("groupByMonth")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant={groupBy === "day" ? "default" : "outline"}
            className={groupBy === "day" ? "bg-blue-600 text-white hover:bg-blue-700" : ""}
            onClick={() => onGroupByChange("day")}
          >
            {t("groupByDay")}
          </Button>
        </div>
      </div>

      {breakdown.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <p className="text-[20px] font-semibold leading-snug text-slate-900">
            {t("noDataHeading")}
          </p>
          <p className="mt-2 text-[16px] text-slate-600">{t("noDataBody")}</p>
        </div>
      ) : (
        <div className="flex flex-col gap-4">
          <div className="overflow-hidden rounded-lg border border-slate-200">
            <Table>
              <caption className="sr-only">{t("breakdownHeading")}</caption>
              <TableHeader>
                <TableRow className="bg-slate-50">
                  <TableHead scope="col" className="text-[14px] text-slate-600">
                    {t("columnPeriod")}
                  </TableHead>
                  <TableHead scope="col" className="text-right text-[14px] text-slate-600">
                    {t("columnReceiptCount")}
                  </TableHead>
                  <TableHead scope="col" className="text-right text-[14px] text-slate-600">
                    {t("columnTotalAmount")}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {pagedRows.map((row) => (
                  <TableRow key={row.period} className="min-h-[56px]">
                    <TableCell className="py-4 text-[16px]">{formatPeriod(row.period)}</TableCell>
                    <TableCell className="py-4 text-right text-[16px]">
                      {countFormatter.format(row.receipt_count)}
                    </TableCell>
                    <TableCell className="py-4 text-right text-[16px]">
                      {amountFormatter.format(row.total_amount)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>

          {totalPages > 1 && (
            <div className="flex items-center justify-between">
              <span className="text-[14px] text-slate-600">
                {breakdown.length} {locale === "th" ? "รายการ" : "records"}
              </span>
              <Pagination className="w-auto mx-0 justify-end">
                <PaginationContent>
                  {page > 1 && (
                    <PaginationItem>
                      <PaginationPrevious
                        href="#"
                        onClick={(e) => {
                          e.preventDefault();
                          setPage((p) => Math.max(1, p - 1));
                        }}
                      />
                    </PaginationItem>
                  )}
                  {Array.from({ length: totalPages }, (_, i) => i + 1).map((p) => (
                    <PaginationItem key={p}>
                      <PaginationLink
                        href="#"
                        isActive={p === page}
                        onClick={(e) => {
                          e.preventDefault();
                          setPage(p);
                        }}
                      >
                        {p}
                      </PaginationLink>
                    </PaginationItem>
                  ))}
                  {page < totalPages && (
                    <PaginationItem>
                      <PaginationNext
                        href="#"
                        onClick={(e) => {
                          e.preventDefault();
                          setPage((p) => Math.min(totalPages, p + 1));
                        }}
                      />
                    </PaginationItem>
                  )}
                </PaginationContent>
              </Pagination>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
