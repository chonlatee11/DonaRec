"use client";

import { useTranslations } from "next-intl";
import { useQuery } from "@tanstack/react-query";
import { DonationTable } from "@/components/DonationTable";
import { fetchDonations } from "@/lib/donations";
import type { SearchFilter } from "@/lib/donations";

interface DonationListViewProps {
  /** Filter parsed from the URL query string (server-passed, drives the query key) */
  filter: SearchFilter;
  /** Current user's Keycloak sub (UUID) — routes Maker's own drafts to edit */
  viewerId?: string;
}

/**
 * DonationListView — client view for Screen 1 (D-R1).
 *
 * Drives the donation list through TanStack Query against the same-origin BFF
 * route (`/api/bff/donations`), which forwards a server-side Bearer to the Go
 * API — the access token never reaches the browser. Renders a skeleton while
 * loading and the UI-SPEC error alert on failure; otherwise hands the unwrapped
 * `{items,total,page,per_page}` to DonationTable.
 */
export function DonationListView({ filter, viewerId }: DonationListViewProps) {
  const t = useTranslations();

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["donations", filter],
    queryFn: () => fetchDonations(filter),
  });

  // Current filter map (excluding `page`) for DonationTable pagination hrefs.
  const currentFilter: Record<string, string> = {};
  if (filter.name) currentFilter.name = filter.name;
  if (filter.status) currentFilter.status = filter.status;
  if (filter.from) currentFilter.from = filter.from;
  if (filter.to) currentFilter.to = filter.to;
  if (filter.receipt_no) currentFilter.receipt_no = filter.receipt_no;

  // ── Loading skeleton ─────────────────────────────────────────────────────────
  if (isLoading) {
    return (
      <div
        className="overflow-hidden rounded-lg border border-slate-200"
        aria-busy="true"
        aria-label={t("donations.title")}
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
    );
  }

  // ── Error state (UI-SPEC alert) ──────────────────────────────────────────────
  if (isError) {
    const message =
      error instanceof Error ? error.message : t("errors.network");
    return (
      <div
        className="rounded-lg border border-red-200 bg-red-50 p-4 text-[14px] text-red-600"
        role="alert"
      >
        {message}
      </div>
    );
  }

  const result = data ?? { items: [], total: 0, page: 1, per_page: 20 };
  // Defense-in-depth (Phase 3 UAT bug): never hand a non-array `items` to
  // DonationTable — it reads `items.length` and crashes. fetchDonations already
  // normalizes the shape, but guard here too so any upstream contract drift
  // degrades to an empty table instead of a runtime TypeError.
  const items = Array.isArray(result.items) ? result.items : [];

  return (
    <DonationTable
      items={items}
      total={result.total ?? 0}
      page={result.page ?? 1}
      perPage={result.per_page ?? 20}
      currentFilter={currentFilter}
      viewerId={viewerId}
    />
  );
}
