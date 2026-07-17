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
import { SourceBadge } from "@/components/SourceBadge";
import type { DonationSummary } from "@/lib/donations";

interface QueueTableProps {
  /** Pending-review rows (Flow A + Flow B) from the queue BFF envelope */
  items: DonationSummary[];
  total: number;
  page: number;
  perPage: number;
  /** Client-driven pagination — the queue's filter/page live in page state, not the URL */
  onPageChange: (page: number) => void;
}

/**
 * QueueTable — Screen 11 pending-review table (TanStack Table, FR-08).
 *
 * UI-SPEC §Screen 11 column subset (order preserved):
 *   วันที่ส่ง (created_at) / ชื่อผู้บริจาค / จำนวนเงิน (right-aligned comma) /
 *   แหล่งที่มา (SourceBadge) / จัดการ (link → Screen 3 detail).
 *
 * Deliberately NO status column (every row is pending_review by definition) and
 * NO receipt column (nothing in pending_review has a receipt number yet).
 * Pagination: 20 rows/page, shadcn Pagination, client-driven via onPageChange.
 */
export function QueueTable({
  items,
  total,
  page,
  perPage,
  onPageChange,
}: QueueTableProps) {
  const t = useTranslations();
  const locale = useLocale() as "th" | "en";

  const totalPages = Math.max(1, Math.ceil(total / perPage));

  // ── Formatters ──────────────────────────────────────────────────────────────

  function formatAmount(amount: string): string {
    const value = parseFloat(amount);
    return new Intl.NumberFormat("th-TH", {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(Number.isFinite(value) ? value : 0);
  }

  function formatDate(dateStr: string): string {
    const date = new Date(dateStr);
    if (locale === "th") {
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

  // ── Column model (TanStack Table) ─────────────────────────────────────────────

  const columns = useMemo<ColumnDef<DonationSummary>[]>(
    () => [
      {
        id: "submittedAt",
        header: () => t("queue.columns.submittedAt"),
        cell: ({ row }) => formatDate(row.original.created_at),
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
          headClass: "w-[120px] text-right",
          cellClass: "text-right text-[16px] tabular-nums",
        },
      },
      {
        id: "source",
        header: () => t("queue.columns.source"),
        cell: ({ row }) => <SourceBadge source={row.original.source} />,
        meta: { headClass: "w-[130px]", cellClass: "" },
      },
      {
        id: "manage",
        header: () => t("fields.managedAt"),
        cell: ({ row }) => (
          <Link
            href={`/donations/${row.original.id}`}
            className="text-[14px] font-medium text-blue-600 hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-600 focus-visible:ring-offset-1"
          >
            {t("actions.viewDetails")}
          </Link>
        ),
        meta: { headClass: "w-[80px]", cellClass: "" },
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [t, locale]
  );

  const table = useReactTable({
    data: items,
    columns,
    getCoreRowModel: getCoreRowModel(),
  });

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

  function handlePageClick(
    e: React.MouseEvent<HTMLAnchorElement>,
    target: number
  ) {
    e.preventDefault();
    if (target >= 1 && target <= totalPages && target !== page) {
      onPageChange(target);
    }
  }

  // ── Render ────────────────────────────────────────────────────────────────────

  return (
    <div className="flex flex-col gap-4">
      {/* Table — horizontal scroll wrapper below md (UI-SPEC responsive contract) */}
      <div className="overflow-x-auto rounded-lg border border-slate-200">
        <Table role="grid">
          <caption className="sr-only">{t("queue.title")}</caption>
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
              <TableRow key={row.id} className="min-h-[56px]">
                {row.getVisibleCells().map((cell) => {
                  const cellClass =
                    (
                      cell.column.columnDef.meta as
                        | { cellClass?: string }
                        | undefined
                    )?.cellClass ?? "";
                  return (
                    <TableCell key={cell.id} className={`py-4 ${cellClass}`}>
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </TableCell>
                  );
                })}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      {/* Pagination — client-driven (source filter + page live in page state) */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between">
          <span className="text-[14px] text-slate-600">
            {total} {locale === "th" ? "รายการ" : "records"}
          </span>
          <Pagination className="w-auto mx-0 justify-end">
            <PaginationContent>
              {page > 1 && (
                <PaginationItem>
                  <PaginationPrevious
                    href="#"
                    onClick={(e) => handlePageClick(e, page - 1)}
                  />
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
                      href="#"
                      isActive={item === page}
                      onClick={(e) => handlePageClick(e, item)}
                    >
                      {item}
                    </PaginationLink>
                  </PaginationItem>
                )
              )}
              {page < totalPages && (
                <PaginationItem>
                  <PaginationNext
                    href="#"
                    onClick={(e) => handlePageClick(e, page + 1)}
                  />
                </PaginationItem>
              )}
            </PaginationContent>
          </Pagination>
        </div>
      )}
    </div>
  );
}
