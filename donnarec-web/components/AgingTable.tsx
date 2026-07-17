"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useLocale, useTranslations } from "next-intl";
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
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination";
import { Checkbox } from "@/components/ui/checkbox";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useToast } from "@/hooks/use-toast";
import { AgingStatCards } from "@/components/AgingStatCards";
import { BulkActionBar } from "@/components/BulkActionBar";
import { AgingBucketBadge } from "@/components/AgingBucketBadge";
import { KeyedStatusBadge } from "@/components/KeyedStatusBadge";
import {
  fetchAging,
  setKeyed,
  type AgingBucket,
  type AgingRow,
} from "@/lib/edonation";

const PER_PAGE = 20;

/**
 * AgingTable — Screen 7 Tab B smart container (FR-31/D-67/D-68): owns the
 * aging query, the bucket-filter/selection/pagination state, and the
 * mark/unmark mutation, and composes AgingStatCards + BulkActionBar + the
 * TanStack Table itself (select column, badges, per-row toggle). Wraps
 * DonationTable's column-definition conventions (meta headClass/cellClass,
 * 56px row height, 20/page Pagination).
 *
 * Select-all scope: the header checkbox's tri-state select-all applies to
 * the CURRENT PAGE's 20 visible rows (not the entire filtered result set) —
 * the conventional data-table bulk-select scope, avoiding a surprising bulk
 * mutation against off-screen rows the user never saw.
 */
export function AgingTable() {
  const t = useTranslations("aging");
  const locale = useLocale() as "th" | "en";
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const { data, isLoading, isError } = useQuery({
    queryKey: ["edonationAging"],
    queryFn: fetchAging,
  });

  const [bucketFilter, setBucketFilter] = useState<AgingBucket | null>(null);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [page, setPage] = useState(1);

  const keyedMutation = useMutation({
    mutationFn: ({ ids, keyed }: { ids: string[]; keyed: boolean }) =>
      setKeyed(ids, keyed),
    onSuccess: (_result, variables) => {
      toast({
        description: variables.keyed
          ? t("markSuccessToast", { n: variables.ids.length })
          : t("unmarkSuccessToast", { n: variables.ids.length }),
      });
      setSelectedIds(new Set());
      void queryClient.invalidateQueries({ queryKey: ["edonationAging"] });
    },
    onError: () => {
      toast({ variant: "destructive", description: t("markFailed") });
    },
  });

  const rows = useMemo(() => data?.rows ?? [], [data]);
  const counts = data?.counts ?? {};

  const filteredRows = useMemo(
    () => (bucketFilter ? rows.filter((r) => r.bucket === bucketFilter) : rows),
    [rows, bucketFilter]
  );

  const totalPages = Math.max(1, Math.ceil(filteredRows.length / PER_PAGE));
  const pagedRows = useMemo(
    () => filteredRows.slice((page - 1) * PER_PAGE, page * PER_PAGE),
    [filteredRows, page]
  );

  function toggleBucket(bucket: AgingBucket) {
    setBucketFilter((prev) => (prev === bucket ? null : bucket));
    setPage(1);
  }

  function toggleRow(id: string, checked: boolean) {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  }

  const pageIds = pagedRows.map((r) => r.id);
  const selectedOnPageCount = pageIds.filter((id) => selectedIds.has(id)).length;
  const allOnPageSelected = pageIds.length > 0 && selectedOnPageCount === pageIds.length;
  const someOnPageSelected = selectedOnPageCount > 0 && !allOnPageSelected;

  function toggleSelectAll(checked: boolean | "indeterminate") {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (checked === true) {
        pageIds.forEach((id) => next.add(id));
      } else {
        pageIds.forEach((id) => next.delete(id));
      }
      return next;
    });
  }

  const selectedRows = rows.filter((r) => selectedIds.has(r.id));
  const canMark = selectedRows.some((r) => !r.keyed);
  const canUnmark = selectedRows.some((r) => r.keyed);

  function formatDate(dateStr: string): string {
    const date = new Date(dateStr);
    if (locale === "th") {
      return new Intl.DateTimeFormat("th-TH-u-ca-buddhist", {
        year: "numeric",
        month: "short",
        day: "numeric",
      }).format(date);
    }
    return new Intl.DateTimeFormat("en-GB", {
      year: "numeric",
      month: "short",
      day: "numeric",
    }).format(date);
  }

  const columns = useMemo<ColumnDef<AgingRow>[]>(
    () => [
      {
        id: "select",
        header: () => (
          <Checkbox
            checked={allOnPageSelected ? true : someOnPageSelected ? "indeterminate" : false}
            onCheckedChange={toggleSelectAll}
            aria-label={t("selectAllAria")}
          />
        ),
        cell: ({ row }) => (
          <Checkbox
            checked={selectedIds.has(row.original.id)}
            onCheckedChange={(checked) => toggleRow(row.original.id, checked === true)}
            aria-label={t("selectRowAria", { receipt: row.original.receipt_formatted })}
          />
        ),
        meta: { headClass: "w-[40px]", cellClass: "w-[40px]" },
      },
      {
        id: "approvedAt",
        header: () => t("columnIssuedDate"),
        cell: ({ row }) => formatDate(row.original.approved_at),
        meta: { headClass: "w-[120px]", cellClass: "text-[16px]" },
      },
      {
        id: "donorName",
        header: () => t("columnDonorName"),
        cell: ({ row }) => row.original.donor_name,
        meta: { headClass: "", cellClass: "text-[16px] font-medium text-slate-900" },
      },
      {
        id: "receiptFormatted",
        header: () => t("columnReceiptNo"),
        cell: ({ row }) => (
          <span className="font-mono text-[14px]">{row.original.receipt_formatted}</span>
        ),
        meta: { headClass: "w-[140px]", cellClass: "" },
      },
      {
        id: "deadline",
        header: () => t("columnDeadline"),
        cell: ({ row }) => (
          <span className="font-mono text-[14px]">{formatDate(row.original.deadline)}</span>
        ),
        meta: { headClass: "w-[140px]", cellClass: "" },
      },
      {
        id: "bucket",
        header: () => t("columnBucket"),
        cell: ({ row }) => <AgingBucketBadge bucket={row.original.bucket} />,
        meta: { headClass: "w-[140px]", cellClass: "" },
      },
      {
        id: "keyedStatus",
        header: () => t("columnKeyedStatus"),
        cell: ({ row }) => {
          const record = row.original;
          return (
            <div className="flex items-center gap-2">
              <KeyedStatusBadge keyed={record.keyed} />
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="h-auto min-h-0 px-2 py-1 text-[13px] text-blue-600 hover:text-blue-700"
                disabled={keyedMutation.isPending}
                onClick={() =>
                  keyedMutation.mutate({ ids: [record.id], keyed: !record.keyed })
                }
              >
                {record.keyed ? t("unmarkKeyed") : t("markKeyed")}
              </Button>
            </div>
          );
        },
        meta: { headClass: "w-[220px]", cellClass: "" },
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [t, locale, selectedIds, allOnPageSelected, someOnPageSelected, pageIds, keyedMutation.isPending]
  );

  const table = useReactTable({
    data: pagedRows,
    columns,
    getCoreRowModel: getCoreRowModel(),
  });

  if (isLoading) {
    return (
      <div className="flex flex-col gap-4" aria-busy="true">
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-[400px] w-full" />
      </div>
    );
  }

  if (isError) {
    return (
      <div
        className="rounded-lg border border-red-200 bg-red-50 p-4 text-[14px] text-red-600"
        role="alert"
      >
        {t("loadFailed")}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <AgingStatCards counts={counts} activeBucket={bucketFilter} onToggle={toggleBucket} />

      {bucketFilter && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="w-fit text-slate-600"
          onClick={() => {
            setBucketFilter(null);
            setPage(1);
          }}
        >
          {t("clearBucketFilter")}
        </Button>
      )}

      <BulkActionBar
        selectedCount={selectedIds.size}
        canMark={canMark}
        canUnmark={canUnmark}
        isPending={keyedMutation.isPending}
        onMark={() => keyedMutation.mutate({ ids: Array.from(selectedIds), keyed: true })}
        onUnmark={() => keyedMutation.mutate({ ids: Array.from(selectedIds), keyed: false })}
        onClear={() => setSelectedIds(new Set())}
      />

      {filteredRows.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <p className="text-[20px] font-semibold leading-snug text-slate-900">
            {t("noUnkeyedHeading")}
          </p>
          <p className="mt-2 text-[16px] text-slate-600">{t("noUnkeyedBody")}</p>
        </div>
      ) : (
        <div className="flex flex-col gap-4">
          {/* overflow-x-auto so wide tables scroll horizontally below 768px
              (UI-SPEC §Table responsiveness) instead of overflowing */}
          <div className="overflow-x-auto rounded-lg border border-slate-200">
            <Table role="grid">
              <caption className="sr-only">{t("tabAging")}</caption>
              <TableHeader>
                {table.getHeaderGroups().map((headerGroup) => (
                  <TableRow key={headerGroup.id} className="bg-slate-50">
                    {headerGroup.headers.map((header) => {
                      const headClass =
                        (header.column.columnDef.meta as { headClass?: string } | undefined)
                          ?.headClass ?? "";
                      return (
                        <TableHead
                          key={header.id}
                          scope="col"
                          className={`text-[14px] text-slate-600 ${headClass}`}
                        >
                          {header.isPlaceholder
                            ? null
                            : flexRender(header.column.columnDef.header, header.getContext())}
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
                        (cell.column.columnDef.meta as { cellClass?: string } | undefined)
                          ?.cellClass ?? "";
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

          {totalPages > 1 && (
            <div className="flex items-center justify-between">
              <span className="text-[14px] text-slate-600">
                {filteredRows.length} {locale === "th" ? "รายการ" : "records"}
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
