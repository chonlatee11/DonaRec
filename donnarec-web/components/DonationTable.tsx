"use client";

import Link from "next/link";
import { useTranslations, useLocale } from "next-intl";
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
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination";
import { StatusBadge } from "@/components/StatusBadge";
import type { DonationSummary } from "@/lib/donations";

interface DonationTableProps {
  donations: DonationSummary[];
  total: number;
  page: number;
  perPage: number;
  /** Current URL query params (excluding `page`) for building pagination hrefs */
  currentFilter: Record<string, string>;
  /** Current user's Keycloak sub (UUID) — used to route Maker's own drafts to edit */
  viewerId?: string;
}

/**
 * DonationTable — Screen 1 table.
 *
 * UI-SPEC §Screen 1 "Table columns":
 *   วันที่บริจาค / ชื่อผู้บริจาค / จำนวนเงิน (right-aligned comma) /
 *   สถานะ (StatusBadge) / เลขที่ใบเสร็จ (issued/cancelled only, em-dash otherwise) /
 *   ผู้สร้าง / จัดการ
 *
 * Row height: 56px minimum (UI-SPEC Spacing).
 * Pagination: 20 rows / page, shadcn Pagination.
 */
export function DonationTable({
  donations,
  total,
  page,
  perPage,
  currentFilter,
  viewerId,
}: DonationTableProps) {
  const t = useTranslations();
  const locale = useLocale() as "th" | "en";

  const totalPages = Math.max(1, Math.ceil(total / perPage));
  const hasFilters = Object.values(currentFilter).some(Boolean);

  // ── Formatters ──────────────────────────────────────────────────────────────

  function formatAmount(amount: number): string {
    return new Intl.NumberFormat("th-TH", {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(amount);
  }

  function formatDate(dateStr: string): string {
    const date = new Date(dateStr);
    if (locale === "th") {
      // Buddhist Era display for Thai locale
      return new Intl.DateTimeFormat("th-TH-u-ca-buddhist", {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
      }).format(date);
    }
    return new Intl.DateTimeFormat("en-GB", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
    }).format(date);
  }

  // ── Row routing ─────────────────────────────────────────────────────────────

  function getDetailHref(donation: DonationSummary): string {
    // Maker's own draft → edit form; all other cases → detail/review page
    if (
      viewerId &&
      viewerId === donation.created_by_id &&
      donation.status === "draft"
    ) {
      return `/donations/${donation.id}/edit`;
    }
    return `/donations/${donation.id}`;
  }

  // ── Receipt cell ────────────────────────────────────────────────────────────

  function renderReceiptCell(donation: DonationSummary) {
    if (donation.status === "cancelled" && donation.receipt_formatted) {
      // UI-SPEC: cancelled → slate-500 strikethrough + "(ยกเลิก)"
      return (
        <span className="font-mono text-[14px]">
          <span className="text-slate-500 line-through">
            {donation.receipt_formatted}
          </span>{" "}
          <span className="text-slate-500">(ยกเลิก)</span>
        </span>
      );
    }
    if (donation.status === "issued" && donation.receipt_formatted) {
      return (
        <span className="font-mono text-[16px] font-semibold text-slate-900">
          {donation.receipt_formatted}
        </span>
      );
    }
    // All other statuses: em-dash
    return <span className="text-slate-400" aria-label="ไม่มีเลขที่ใบเสร็จ">—</span>;
  }

  // ── Pagination href builder ─────────────────────────────────────────────────

  function pageHref(targetPage: number): string {
    const params = new URLSearchParams(currentFilter);
    if (targetPage > 1) {
      params.set("page", String(targetPage));
    } else {
      params.delete("page");
    }
    const qs = params.toString();
    return `/donations${qs ? `?${qs}` : ""}`;
  }

  // ── Empty states ─────────────────────────────────────────────────────────────

  if (donations.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-center">
        <p className="text-[20px] font-semibold leading-snug text-slate-900">
          {hasFilters
            ? t("empty.noSearchResults.heading")
            : t("empty.noRecords.heading")}
        </p>
        <p className="mt-2 text-[16px] text-slate-600">
          {hasFilters
            ? t("empty.noSearchResults.body")
            : t("empty.noRecords.body")}
        </p>
      </div>
    );
  }

  // ── Pagination helper ─────────────────────────────────────────────────────────

  function buildPageItems(): Array<number | "ellipsis"> {
    const items: Array<number | "ellipsis"> = [];
    for (let p = 1; p <= totalPages; p++) {
      if (
        p === 1 ||
        p === totalPages ||
        (p >= page - 2 && p <= page + 2)
      ) {
        items.push(p);
      } else if (items[items.length - 1] !== "ellipsis") {
        items.push("ellipsis");
      }
    }
    return items;
  }

  // ── Render ────────────────────────────────────────────────────────────────────

  return (
    <div className="flex flex-col gap-4">
      {/* Table */}
      <div className="overflow-hidden rounded-lg border border-slate-200">
        <Table role="grid">
          <caption className="sr-only">
            {t("donations.title")}
          </caption>
          <TableHeader>
            <TableRow className="bg-slate-50">
              <TableHead
                scope="col"
                className="w-[120px] text-[14px] text-slate-600"
              >
                {t("fields.donatedAt")}
              </TableHead>
              <TableHead scope="col" className="text-[14px] text-slate-600">
                {t("fields.donorName")}
              </TableHead>
              <TableHead
                scope="col"
                className="w-[130px] text-right text-[14px] text-slate-600"
              >
                {t("fields.amount")}
              </TableHead>
              <TableHead
                scope="col"
                className="w-[140px] text-[14px] text-slate-600"
              >
                {t("fields.status")}
              </TableHead>
              <TableHead
                scope="col"
                className="w-[150px] text-[14px] text-slate-600"
              >
                {t("fields.receiptNumber")}
              </TableHead>
              <TableHead
                scope="col"
                className="w-[110px] text-[14px] text-slate-600"
              >
                {t("fields.createdBy")}
              </TableHead>
              <TableHead
                scope="col"
                className="w-[80px] text-[14px] text-slate-600"
              >
                {t("fields.managedAt")}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {donations.map((donation) => (
              <TableRow
                key={donation.id}
                className={[
                  "min-h-[56px]",
                  donation.status === "rejected" ? "text-slate-500" : "",
                ]
                  .filter(Boolean)
                  .join(" ")}
              >
                <TableCell className="py-4 text-[16px]">
                  {formatDate(donation.donated_at)}
                </TableCell>
                <TableCell className="py-4 text-[16px] font-medium text-slate-900">
                  {donation.donor_name}
                </TableCell>
                <TableCell className="py-4 text-right text-[16px] tabular-nums">
                  {formatAmount(donation.amount)}
                </TableCell>
                <TableCell className="py-4">
                  <StatusBadge status={donation.status} locale={locale} />
                </TableCell>
                <TableCell className="py-4">
                  {renderReceiptCell(donation)}
                </TableCell>
                <TableCell className="py-4 text-[14px] text-slate-600">
                  {donation.created_by}
                </TableCell>
                <TableCell className="py-4">
                  <Link
                    href={getDetailHref(donation)}
                    className="text-[14px] font-medium text-blue-600 hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-600 focus-visible:ring-offset-1"
                  >
                    {t("actions.viewDetails")}
                  </Link>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between">
          <span className="text-[14px] text-slate-600">
            {total} {locale === "th" ? "รายการ" : "records"}
          </span>
          <Pagination className="w-auto mx-0 justify-end">
            <PaginationContent>
              {page > 1 && (
                <PaginationItem>
                  <PaginationPrevious href={pageHref(page - 1)} />
                </PaginationItem>
              )}
              {buildPageItems().map((item, idx) =>
                item === "ellipsis" ? (
                  <PaginationItem key={`ellipsis-${idx}`}>
                    <PaginationEllipsis />
                  </PaginationItem>
                ) : (
                  <PaginationItem key={item}>
                    <PaginationLink
                      href={pageHref(item)}
                      isActive={item === page}
                    >
                      {item}
                    </PaginationLink>
                  </PaginationItem>
                )
              )}
              {page < totalPages && (
                <PaginationItem>
                  <PaginationNext href={pageHref(page + 1)} />
                </PaginationItem>
              )}
            </PaginationContent>
          </Pagination>
        </div>
      )}
    </div>
  );
}
