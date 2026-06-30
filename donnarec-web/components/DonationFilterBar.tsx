"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { format } from "date-fns";
import { Search, X, CalendarIcon } from "lucide-react";
import { Input } from "@/components/ui/input";
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
import type { SearchFilter, DonationStatus } from "@/lib/donations";

/** Sentinel value for "all statuses" in the Select (SelectItem cannot have empty value) */
const ALL_STATUSES = "all";

const STATUS_VALUES: DonationStatus[] = [
  "draft",
  "pending_review",
  "issued",
  "rejected",
  "cancelled",
];

interface DonationFilterBarProps {
  currentFilter: SearchFilter;
}

/**
 * DonationFilterBar — horizontal filter bar for Screen 1 (Donation List).
 *
 * D-53: Only name, date range, status, and receipt number are exposed as filters.
 * tax/national ID search is intentionally absent.
 *
 * On "ค้นหา": pushes updated query string to the URL (Server Component re-renders).
 * On "ล้างการกรอง": resets all fields and navigates to /donations.
 */
export function DonationFilterBar({ currentFilter }: DonationFilterBarProps) {
  const t = useTranslations();
  const router = useRouter();

  const [name, setName] = useState(currentFilter.name ?? "");
  const [status, setStatus] = useState<DonationStatus | typeof ALL_STATUSES>(
    currentFilter.status || ALL_STATUSES
  );
  const [from, setFrom] = useState<Date | undefined>(
    currentFilter.from ? new Date(currentFilter.from) : undefined
  );
  const [to, setTo] = useState<Date | undefined>(
    currentFilter.to ? new Date(currentFilter.to) : undefined
  );
  const [receiptNo, setReceiptNo] = useState(currentFilter.receipt_no ?? "");

  function buildParams(): URLSearchParams {
    const params = new URLSearchParams();
    if (name.trim()) params.set("name", name.trim());
    if (status && status !== ALL_STATUSES) params.set("status", status);
    if (from) params.set("from", format(from, "yyyy-MM-dd"));
    if (to) params.set("to", format(to, "yyyy-MM-dd"));
    if (receiptNo.trim()) params.set("receipt_no", receiptNo.trim());
    return params;
  }

  function handleSearch() {
    const params = buildParams();
    const qs = params.toString();
    router.push(`/donations${qs ? `?${qs}` : ""}`);
  }

  function handleClear() {
    setName("");
    setStatus(ALL_STATUSES);
    setFrom(undefined);
    setTo(undefined);
    setReceiptNo("");
    router.push("/donations");
  }

  return (
    <div className="flex flex-wrap gap-3 items-end rounded-lg border border-slate-200 bg-white p-4">
      {/* ── Donor name ── */}
      <div className="flex min-w-[200px] flex-1 flex-col gap-1">
        <label className="text-[14px] font-normal text-slate-600">
          {t("fields.donorName")}
        </label>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={t("placeholders.donorNameSearch")}
          onKeyDown={(e) => e.key === "Enter" && handleSearch()}
          className="min-h-[44px]"
        />
      </div>

      {/* ── From date ── */}
      <div className="flex flex-col gap-1">
        <label className="text-[14px] font-normal text-slate-600">
          {t("filter.donatedFrom")}
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
                <span className="text-slate-400">{t("filter.donatedFrom")}</span>
              )}
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-auto p-0" align="start">
            <Calendar
              mode="single"
              selected={from}
              onSelect={setFrom}
            />
          </PopoverContent>
        </Popover>
      </div>

      {/* ── To date ── */}
      <div className="flex flex-col gap-1">
        <label className="text-[14px] font-normal text-slate-600">
          {t("filter.donatedTo")}
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
                <span className="text-slate-400">{t("filter.donatedTo")}</span>
              )}
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-auto p-0" align="start">
            <Calendar
              mode="single"
              selected={to}
              onSelect={setTo}
            />
          </PopoverContent>
        </Popover>
      </div>

      {/* ── Status ── */}
      <div className="flex min-w-[170px] flex-col gap-1">
        <label className="text-[14px] font-normal text-slate-600">
          {t("fields.status")}
        </label>
        <Select
          value={status}
          onValueChange={(v) => setStatus(v as DonationStatus | typeof ALL_STATUSES)}
        >
          <SelectTrigger className="min-h-[44px]">
            <SelectValue placeholder={t("filter.allStatuses")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL_STATUSES}>
              {t("filter.allStatuses")}
            </SelectItem>
            {STATUS_VALUES.map((s) => (
              <SelectItem key={s} value={s}>
                {t(`status.${s}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* ── Receipt number ── */}
      <div className="flex min-w-[160px] flex-col gap-1">
        <label className="text-[14px] font-normal text-slate-600">
          {t("fields.receiptNumber")}
        </label>
        <Input
          value={receiptNo}
          onChange={(e) => setReceiptNo(e.target.value)}
          placeholder={t("placeholders.receiptNumberSearch")}
          onKeyDown={(e) => e.key === "Enter" && handleSearch()}
          className="min-h-[44px]"
        />
      </div>

      {/* ── Actions ── */}
      <div className="flex items-end gap-2">
        <Button
          onClick={handleSearch}
          className="min-h-[44px] bg-blue-600 text-white hover:bg-blue-700"
        >
          <Search className="mr-2 h-4 w-4" />
          {t("actions.search")}
        </Button>
        <Button
          variant="ghost"
          onClick={handleClear}
          className="min-h-[44px] text-slate-600"
        >
          <X className="mr-2 h-4 w-4" />
          {t("actions.clearFilters")}
        </Button>
      </div>
    </div>
  );
}
