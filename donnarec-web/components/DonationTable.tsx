"use client";

import { useMemo } from "react";
import Link from "next/link";
import { useTranslations, useLocale } from "next-intl";
import {
  type ColumnDef,
  flexRender,
  getCoreRowModel,
  useReactTable,
} from "@tanstack/react-table";
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
import { SourceBadge } from "@/components/SourceBadge";
import type { DonationSummary } from "@/lib/donations";

interface DonationTableProps {
  /** D-R2: rows from the `{data:{items,...}}` envelope */
  items: DonationSummary[];
  total: number;
  page: number;
  perPage: number;
  /** Current URL query params (excluding `page`) for building pagination hrefs */
  currentFilter: Record<string, string>;
  /** Current user's Keycloak sub (UUID) — used to route Maker's own drafts to edit */
  viewerId?: string;
}

/**
 * DonationTable — Screen 1 table (TanStack Table).
 *
 * UI-SPEC §Screen 1 "Table columns" (order preserved):
 *   วันที่บริจาค / ชื่อผู้บริจาค / จำนวนเงิน (right-aligned comma) /
 *   สถานะ (StatusBadge) / เลขที่ใบเสร็จ (issued/cancelled only, em-dash otherwise) /
 *   ผู้สร้าง / จัดการ
 *
 * Row height: 56px minimum (UI-SPEC Spacing).
 * Pagination: 20 rows / page, shadcn Pagination.
 *
 * Rendered by DonationListView, which drives data through TanStack Query +
 * the same-origin BFF route (D-R1). Column model built with @tanstack/react-table.
 */
export function DonationTable({
  items,
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

  function formatAmount(amount: string): string {
    // D-R2/03-09: amount arrives as a numeric string ("1500.00").
    const value = parseFloat(amount);
    return new Intl.NumberFormat("th-TH", {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(Number.isFinite(value) ? value : 0);
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
    return (
      <span className="text-slate-400" aria-label="ไม่มีเลขที่ใบเสร็จ">
        —
      </span>
    );
  }

  // ── Column model (TanStack Table) ─────────────────────────────────────────────
  // Order + widths + alignment preserve UI-SPEC Screen 1 exactly.

  const columns = useMemo<ColumnDef<DonationSummary>[]>(
    () => [
      {
        id: "donatedAt",
        header: () => t("fields.donatedAt"),
        cell: ({ row }) => formatDate(row.original.donated_at),
        meta: { headClass: "w-[120px]", cellClass: "text-[16px]" },
      },
      {
        id: "donorName",
        header: () => t("fields.donorName"),
        cell: ({ row }) => row.original.donor_name,
        meta: {
          headClass: "",
          cellClass: "text-[16px] font-medium text-slate-900",
        },
      },
      {
        id: "amount",
        header: () => t("fields.amount"),
        cell: ({ row }) => formatAmount(row.original.amount),
        meta: {
          headClass: "w-[130px] text-right",
          cellClass: "text-right text-[16px] tabular-nums",
        },
      },
      {
        id: "status",
        header: () => t("fields.status"),
        cell: ({ row }) => (
          <StatusBadge status={row.original.status} locale={locale} />
        ),
        meta: { headClass: "w-[140px]", cellClass: "" },
      },
      {
        id: "source",
        header: () => t("fields.source"),
        cell: ({ row }) => <SourceBadge source={row.original.source} />,
        meta: { headClass: "w-[130px]", cellClass: "" },
      },
      {
        id: "receiptNumber",
        header: () => t("fields.receiptNumber"),
        cell: ({ row }) => renderReceiptCell(row.original),
        meta: { headClass: "w-[150px]", cellClass: "" },
      },
      {
        id: "createdBy",
        header: () => t("fields.createdBy"),
        cell: ({ row }) => row.original.created_by,
        meta: {
          headClass: "w-[110px]",
          cellClass: "text-[14px] text-slate-600",
        },
      },
      {
        id: "manage",
        header: () => t("fields.managedAt"),
        cell: ({ row }) => (
          <Link
            href={getDetailHref(row.original)}
            className="text-[14px] font-medium text-blue-600 hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-600 focus-visible:ring-offset-1"
          >
            {t("actions.viewDetails")}
          </Link>
        ),
        meta: { headClass: "w-[80px]", cellClass: "" },
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [t, locale, viewerId]
  );

  const table = useReactTable({
    data: items,
    columns,
    getCoreRowModel: getCoreRowModel(),
  });

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

  if (items.length === 0) {
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
    const pageItems: Array<number | "ellipsis"> = [];
    for (let p = 1; p <= totalPages; p++) {
      if (p === 1 || p === totalPages || (p >= page - 2 && p <= page + 2)) {
        pageItems.push(p);
      } else if (pageItems[pageItems.length - 1] !== "ellipsis") {
        pageItems.push("ellipsis");
      }
    }
    return pageItems;
  }

  // ── Render ────────────────────────────────────────────────────────────────────

  return (
    <div className="flex flex-col gap-4">
      {/* Table */}
      <div className="overflow-hidden rounded-lg border border-slate-200">
        <Table role="grid">
          <caption className="sr-only">{t("donations.title")}</caption>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id} className="bg-slate-50">
                {headerGroup.headers.map((header) => {
                  const headClass =
                    (
                      header.column.columnDef.meta as
                        | { headClass?: string }
                        | undefined
                    )?.headClass ?? "";
                  return (
                    <TableHead
                      key={header.id}
                      scope="col"
                      className={`text-[14px] text-slate-600 ${headClass}`}
                    >
                      {header.isPlaceholder
                        ? null
                        : flexRender(
                            header.column.columnDef.header,
                            header.getContext()
                          )}
                    </TableHead>
                  );
                })}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {table.getRowModel().rows.map((row) => (
              <TableRow
                key={row.id}
                className={[
                  "min-h-[56px]",
                  row.original.status === "rejected" ? "text-slate-500" : "",
                ]
                  .filter(Boolean)
                  .join(" ")}
              >
                {row.getVisibleCells().map((cell) => {
                  const cellClass =
                    (
                      cell.column.columnDef.meta as
                        | { cellClass?: string }
                        | undefined
                    )?.cellClass ?? "";
                  return (
                    <TableCell key={cell.id} className={`py-4 ${cellClass}`}>
                      {flexRender(
                        cell.column.columnDef.cell,
                        cell.getContext()
                      )}
                    </TableCell>
                  );
                })}
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
