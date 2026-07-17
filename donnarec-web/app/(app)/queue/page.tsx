"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { useQuery } from "@tanstack/react-query";
import { QueueSourceFilter } from "@/components/QueueSourceFilter";
import { QueueTable } from "@/components/QueueTable";
import { fetchQueue } from "@/lib/donations";
import type { QueueSource } from "@/lib/donations";

/**
 * Pending-Review Queue — Screen 11 (FR-08, D-77). Implements the previously-dead
 * /queue nav link. Lives in the (app) route group → slate/blue back-office theme,
 * behind middleware auth + AppShell (no warm public theme here).
 *
 * Smart container: holds the source-filter + page as local state (not URL —
 * matches the segmented-control interaction model), drives a TanStack query
 * against the authenticated `/api/bff/queue` BFF (status pinned to
 * pending_review server-side), and renders the source-aware empty states.
 */
export default function QueuePage() {
  const t = useTranslations();
  const [source, setSource] = useState<QueueSource>("all");
  const [page, setPage] = useState(1);

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["queue", source, page],
    queryFn: () => fetchQueue(source, page),
  });

  function handleSourceChange(next: QueueSource) {
    setSource(next);
    setPage(1); // reset to first page when the filter changes
  }

  const result = data ?? { items: [], total: 0, page: 1, per_page: 20 };
  const items = Array.isArray(result.items) ? result.items : [];
  const isFiltered = source !== "all";

  return (
    <div className="flex flex-col gap-6">
      {/* Page heading */}
      <h1 className="text-[28px] font-semibold leading-tight text-slate-900">
        {t("queue.title")}
      </h1>

      {/* Source filter chips */}
      <QueueSourceFilter active={source} onChange={handleSourceChange} />

      {/* Loading skeleton */}
      {isLoading && (
        <div
          className="overflow-hidden rounded-lg border border-slate-200"
          aria-busy="true"
          aria-label={t("queue.title")}
        >
          <div className="h-12 bg-slate-50" />
          {Array.from({ length: 6 }).map((_, i) => (
            <div
              key={i}
              className="flex items-center gap-4 border-t border-slate-100 px-4 py-4"
            >
              <div className="h-4 w-full animate-pulse rounded bg-slate-100" />
            </div>
          ))}
        </div>
      )}

      {/* Error state */}
      {!isLoading && isError && (
        <div
          className="rounded-lg border border-red-200 bg-red-50 p-4 text-[14px] text-red-600"
          role="alert"
        >
          {error instanceof Error ? error.message : t("errors.network")}
        </div>
      )}

      {/* Empty state — source-aware (UI-SPEC Queue empty/copy) */}
      {!isLoading && !isError && items.length === 0 && (
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <p className="text-[20px] font-semibold leading-snug text-slate-900">
            {isFiltered
              ? t("queue.emptyFiltered.heading")
              : t("queue.empty.heading")}
          </p>
          <p className="mt-2 text-[16px] text-slate-600">
            {isFiltered
              ? t("queue.emptyFiltered.body")
              : t("queue.empty.body")}
          </p>
        </div>
      )}

      {/* Table */}
      {!isLoading && !isError && items.length > 0 && (
        <QueueTable
          items={items}
          total={result.total ?? 0}
          page={result.page ?? page}
          perPage={result.per_page ?? 20}
          onPageChange={setPage}
        />
      )}
    </div>
  );
}
