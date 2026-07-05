"use client";

import Link from "next/link";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslations, useLocale } from "next-intl";
import { ExternalLink, ArrowLeft } from "lucide-react";
import {
  fetchDonation,
  approve,
  returnForEdit,
  reject,
  revealPII,
  cancelDonation,
  reissueDonation,
  resendReceipt,
  downloadReceipt,
  apiErrorMessage,
} from "@/lib/donations";
import { DonnaRecApiError } from "@/lib/api";
import { StatusBadge } from "@/components/StatusBadge";
import { MaskedIdField } from "@/components/MaskedIdField";
import { ReviewActionPanel } from "@/components/ReviewActionPanel";
import { EmailDeliveryPanel } from "@/components/EmailDeliveryPanel";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { useToast } from "@/hooks/use-toast";
import type { CancelDonationRequest } from "@/lib/donations";

interface DonationDetailViewProps {
  id: string;
}

function errorMessage(err: unknown): string {
  if (err instanceof DonnaRecApiError) return err.error.message;
  if (err instanceof Error) return err.message;
  return "เกิดข้อผิดพลาด";
}

/**
 * DonationDetailView — client detail/review view for Screen 3 + Screen 4 PII
 * reveal (03-12; cancel/reissue migrated to client mutations in 03-13).
 *
 * Fetches the record via useQuery against the BFF (D-R1: the Keycloak access
 * token stays server-side, obtained inside the BFF route via getServerSession
 * — this component only ever calls the same-origin `/api/bff/donations/:id`).
 * Approve/return/reject/cancel/reissue all run as useMutation calls through
 * the BFF and invalidate the detail query on success so the panel updates
 * without a full page reload. ReviewActionPanel's branching (role x status
 * matrix, SoD DOM-removal) and MaskedIdField's session-only reveal state are
 * unchanged — this view only supplies data + BFF-backed callbacks matching
 * their existing prop contracts.
 */
export function DonationDetailView({
  id,
}: DonationDetailViewProps) {
  const t = useTranslations();
  const locale = useLocale() as "th" | "en";
  const queryClient = useQueryClient();
  const { toast } = useToast();

  const {
    data: donation,
    isLoading,
    isError,
    error,
  } = useQuery({
    queryKey: ["donation", id],
    queryFn: () => fetchDonation(id),
  });

  function invalidate() {
    queryClient.invalidateQueries({ queryKey: ["donation", id] });
  }

  const approveMutation = useMutation({
    mutationFn: () => approve(id),
    onSuccess: invalidate,
  });
  const returnMutation = useMutation({
    mutationFn: (reason: string) => returnForEdit(id, reason),
    onSuccess: invalidate,
  });
  const rejectMutation = useMutation({
    mutationFn: (reason: string) => reject(id, reason),
    onSuccess: invalidate,
  });
  const cancelMutation = useMutation({
    mutationFn: (body: CancelDonationRequest) => cancelDonation(id, body),
    onSuccess: () => {
      invalidate();
      queryClient.invalidateQueries({ queryKey: ["donations"] });
    },
  });
  const reissueMutation = useMutation({
    mutationFn: (body: CancelDonationRequest) => reissueDonation(id, body),
    onSuccess: (result) => {
      invalidate();
      queryClient.invalidateQueries({ queryKey: ["donation", result.id] });
      queryClient.invalidateQueries({ queryKey: ["donations"] });
    },
  });

  // ── Resend / Download (Screen 3b, D-56/D-57, FR-27/28, plan 04-06) ───────
  const resendMutation = useMutation({
    mutationFn: () => resendReceipt(id),
    onSuccess: () => {
      invalidate();
      toast({ description: t("emailDelivery.resendSuccessToast") });
    },
    onError: (err) => {
      toast({
        variant: "destructive",
        description: `${t("emailDelivery.resendErrorToast")} (${apiErrorMessage(err)})`,
      });
    },
  });
  const downloadMutation = useMutation({
    mutationFn: () => downloadReceipt(id),
    onSuccess: (result) => {
      // FW-03: window.open() here runs after the await, outside the click's
      // user-gesture stack, so popup blockers commonly suppress it and the
      // receipt PDF silently fails to open. A synthesized anchor click is
      // treated as a navigation, not a popup, and is not blocked.
      const a = document.createElement("a");
      a.href = result.url;
      a.target = "_blank";
      a.rel = "noopener noreferrer";
      document.body.appendChild(a);
      a.click();
      a.remove();
    },
    onError: (err) => {
      toast({ variant: "destructive", description: apiErrorMessage(err) });
    },
  });

  function handleResend() {
    resendMutation.mutate();
  }

  function handleDownload() {
    downloadMutation.mutate();
  }

  // ── ReviewActionPanel-compatible wrappers ────────────────────────────────
  // ReviewActionPanel expects Promise<{error:string}|null> (its original
  // Server Action contract) — preserved exactly so ReviewActionPanel itself
  // needs no changes.

  async function handleApprove(): Promise<{ error: string } | null> {
    try {
      await approveMutation.mutateAsync();
      return null;
    } catch (err) {
      return { error: errorMessage(err) };
    }
  }

  async function handleReturn(reason: string): Promise<{ error: string } | null> {
    try {
      await returnMutation.mutateAsync(reason);
      return null;
    } catch (err) {
      return { error: errorMessage(err) };
    }
  }

  async function handleReject(reason: string): Promise<{ error: string } | null> {
    try {
      await rejectMutation.mutateAsync(reason);
      return null;
    } catch (err) {
      return { error: errorMessage(err) };
    }
  }

  /**
   * Audited PII reveal — MaskedIdField expects
   * Promise<{national_id:string}|{error:string}>. Session-only: revealed
   * state lives in MaskedIdField's own component state, not persisted; reload
   * re-masks. Server audits every call (T-03-34 / T-12-01).
   */
  async function handleRevealPII(): Promise<
    { national_id: string } | { error: string }
  > {
    try {
      const result = await revealPII(id);
      return { national_id: result.national_id };
    } catch (err) {
      return { error: errorMessage(err) };
    }
  }

  /**
   * Void (cancel) an issued receipt (D-R1, 03-13: client BFF mutation).
   * T-03-36: rd_confirmation_reason required when edonation_keyed=true.
   * Server returns 409 ErrEDonationKeyedConfirmation if missing.
   */
  async function handleCancel(
    body: CancelDonationRequest
  ): Promise<{ error?: string } | null> {
    try {
      await cancelMutation.mutateAsync(body);
      return null;
    } catch (err) {
      return { error: errorMessage(err) };
    }
  }

  /**
   * Void & Reissue — cancel original + create replacement draft (D-50).
   * New draft earns a receipt number only via the normal Submit → Approve path.
   */
  async function handleReissue(
    body: CancelDonationRequest
  ): Promise<{ error?: string; newId?: string } | null> {
    try {
      const result = await reissueMutation.mutateAsync(body);
      return { newId: result.id };
    } catch (err) {
      return { error: errorMessage(err) };
    }
  }

  // ── Formatters ─────────────────────────────────────────────────────────────

  function formatDate(dateStr: string): string {
    const date = new Date(dateStr);
    if (locale === "th") {
      return new Intl.DateTimeFormat("th-TH-u-ca-buddhist", {
        year: "numeric",
        month: "long",
        day: "numeric",
      }).format(date);
    }
    return new Intl.DateTimeFormat("en-GB", {
      year: "numeric",
      month: "long",
      day: "numeric",
    }).format(date);
  }

  function formatDateTime(dateStr: string): string {
    const date = new Date(dateStr);
    if (locale === "th") {
      return new Intl.DateTimeFormat("th-TH-u-ca-buddhist", {
        year: "numeric",
        month: "long",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      }).format(date);
    }
    return new Intl.DateTimeFormat("en-GB", {
      year: "numeric",
      month: "long",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    }).format(date);
  }

  function formatAmount(amount: string): string {
    // D-R2/03-09: amount arrives as a numeric string ("1500.00").
    const value = parseFloat(amount);
    return new Intl.NumberFormat("th-TH", {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(Number.isFinite(value) ? value : 0);
  }

  // ── Loading skeleton ─────────────────────────────────────────────────────────

  if (isLoading) {
    return (
      <div className="flex flex-col gap-6" aria-busy="true">
        <div className="h-4 w-32 animate-pulse rounded bg-slate-100" />
        <div className="rounded-lg border border-slate-200 bg-white p-6">
          <Skeleton className="mb-4 h-8 w-48" />
          <Skeleton className="mb-2 h-5 w-full" />
          <Skeleton className="mb-2 h-5 w-full" />
          <Skeleton className="h-5 w-2/3" />
        </div>
      </div>
    );
  }

  // ── Error state (UI-SPEC alert; matches DonationListView's pattern) ─────────

  if (isError || !donation) {
    const message =
      error instanceof DonnaRecApiError
        ? error.error.message
        : error instanceof Error
        ? error.message
        : t("errors.network");
    return (
      <div
        className="rounded-lg border border-red-200 bg-red-50 p-4 text-[14px] text-red-600"
        role="alert"
      >
        {message}
      </div>
    );
  }

  const isReceiptVisible =
    donation.status === "issued" || donation.status === "cancelled";

  // ── Render ─────────────────────────────────────────────────────────────────

  return (
    <div className="flex flex-col gap-6">
      {/* Back link */}
      <Link
        href="/donations"
        className="inline-flex items-center gap-1.5 text-[14px] text-blue-600 hover:underline"
      >
        <ArrowLeft className="h-3.5 w-3.5" />
        {t("detail.backToList")}
      </Link>

      {/* Two-column layout: 60% left / 40% right */}
      <div className="flex flex-col gap-6 lg:flex-row lg:items-start lg:gap-8">
        {/* ── Left panel — ข้อมูลรายการ ──────────────────────────────────── */}
        <div className="flex min-w-0 flex-1 flex-col gap-6 lg:w-3/5">
          <div className="rounded-lg border border-slate-200 bg-white p-6">
            <h1 className="mb-4 text-[28px] font-semibold leading-tight text-slate-900">
              {t("detail.title")}
            </h1>

            {/* Status badge — 20px label per UI-SPEC Screen 3 */}
            <div className="mb-4">
              <StatusBadge
                status={donation.status}
                locale={locale}
                className="text-sm px-3 py-1"
              />
            </div>

            {/* Receipt number block — visible only for issued / cancelled */}
            {isReceiptVisible && donation.receipt_formatted && (
              <div className="mb-6 rounded-md border border-slate-200 bg-slate-50 p-4">
                <p className="text-[14px] font-normal text-slate-600">
                  {t("detail.receiptLabel")}
                </p>
                <p className="mt-1 font-mono text-[20px] font-semibold text-slate-900">
                  {donation.receipt_formatted}
                </p>
                {donation.status === "cancelled" && (
                  <p className="mt-0.5 text-[14px] text-slate-500">
                    {t("detail.cancelledSuffix")}
                  </p>
                )}
              </div>
            )}

            {/* Email Delivery panel (Screen 3b, D-56/D-57, FR-27/28) — directly
                below the receipt-number block, issued/cancelled only. */}
            {(donation.status === "issued" || donation.status === "cancelled") && (
              <div className="mb-6">
                <EmailDeliveryPanel
                  status={donation.status}
                  emailDelivery={donation.email_delivery}
                  hasPdf={!!donation.receipt_pdf_object_key}
                  canResend={donation.can_reveal_pii}
                  onDownload={handleDownload}
                  onResend={handleResend}
                  isDownloading={downloadMutation.isPending}
                  isResending={resendMutation.isPending}
                  locale={locale}
                  formatDateTime={formatDateTime}
                />
              </div>
            )}

            <Separator className="my-4" />

            {/* Donor definition list */}
            <dl className="flex flex-col gap-3">
              {/* ชื่อผู้บริจาค */}
              <div className="grid grid-cols-[180px_1fr] gap-2">
                <dt className="text-[14px] text-slate-600">
                  {t("fields.donorName")}
                </dt>
                <dd className="text-[16px] text-slate-900">
                  {donation.donor_name}
                </dd>
              </div>

              {/* เลขประจำตัว — masked by default; reveal wired to Screen 4 */}
              <div className="grid grid-cols-[180px_1fr] gap-2">
                <dt className="text-[14px] text-slate-600">
                  {t("fields.nationalId")}
                </dt>
                <dd>
                  {/*
                   * T-03-34: plaintext only via the audited BFF /pii route.
                   * Session-only: reload re-masks. Server audits every reveal.
                   */}
                  <MaskedIdField
                    maskedValue={donation.national_id_masked}
                    canReveal={donation.can_reveal_pii}
                    onRevealAction={
                      donation.can_reveal_pii ? handleRevealPII : undefined
                    }
                  />
                </dd>
              </div>

              {/* ที่อยู่ */}
              <div className="grid grid-cols-[180px_1fr] gap-2">
                <dt className="text-[14px] text-slate-600">
                  {t("fields.address")}
                </dt>
                <dd className="text-[16px] text-slate-900 whitespace-pre-line">
                  {donation.address}
                </dd>
              </div>

              {/* อีเมล */}
              {donation.email && (
                <div className="grid grid-cols-[180px_1fr] gap-2">
                  <dt className="text-[14px] text-slate-600">
                    {t("fields.email")}
                  </dt>
                  <dd className="text-[16px] text-slate-900">
                    {donation.email}
                  </dd>
                </div>
              )}

              {/* จำนวนเงิน */}
              <div className="grid grid-cols-[180px_1fr] gap-2">
                <dt className="text-[14px] text-slate-600">
                  {t("fields.amount")}
                </dt>
                <dd className="text-[16px] tabular-nums text-slate-900">
                  {formatAmount(donation.amount)}
                </dd>
              </div>

              {/* วันที่บริจาค */}
              <div className="grid grid-cols-[180px_1fr] gap-2">
                <dt className="text-[14px] text-slate-600">
                  {t("fields.donatedAt")}
                </dt>
                <dd className="text-[16px] text-slate-900">
                  {formatDate(donation.donated_at)}
                </dd>
              </div>

              {/* หมายเหตุ */}
              {donation.note && (
                <div className="grid grid-cols-[180px_1fr] gap-2">
                  <dt className="text-[14px] text-slate-600">
                    {t("fields.note")}
                  </dt>
                  <dd className="text-[16px] text-slate-900">
                    {donation.note}
                  </dd>
                </div>
              )}
            </dl>

            <Separator className="my-4" />

            {/* Slip section */}
            <div className="flex flex-col gap-1">
              <p className="text-[14px] font-medium text-slate-700">
                สลิปการโอนเงิน
              </p>
              {donation.slip_url ? (
                <a
                  href={donation.slip_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1.5 text-[14px] text-blue-600 hover:underline"
                >
                  <ExternalLink className="h-3.5 w-3.5" />
                  {t("detail.viewSlip")}
                </a>
              ) : (
                <p className="text-[14px] text-slate-400">
                  {t("detail.noSlip")}
                </p>
              )}
            </div>

            {/* Consent row */}
            <div className="mt-3 flex flex-col gap-1">
              <p className="text-[14px] font-medium text-slate-700">
                ความยินยอม PDPA
              </p>
              {donation.consent_at ? (
                <p className="text-[14px] text-slate-900">
                  {t("detail.consentGiven", {
                    date: formatDateTime(donation.consent_at),
                  })}
                </p>
              ) : (
                <p className="text-[14px] text-slate-500">
                  {t("detail.consentPending")}
                </p>
              )}
            </div>

            {/* Replace chain */}
            {(donation.replaces || donation.replaced_by) && (
              <>
                <Separator className="my-4" />
                <div className="flex flex-col gap-2">
                  {donation.replaces && (
                    <p className="text-[14px] text-slate-700">
                      {t("detail.replacesLink")}{" "}
                      <Link
                        href={`/donations/${donation.replaces.id}`}
                        className="text-blue-600 hover:underline"
                      >
                        {donation.replaces.receipt_formatted}
                      </Link>
                    </p>
                  )}
                  {donation.replaced_by && (
                    <p className="text-[14px] text-slate-700">
                      {t("detail.replacedByLink")}{" "}
                      <Link
                        href={`/donations/${donation.replaced_by.id}`}
                        className="text-blue-600 hover:underline"
                      >
                        {donation.replaced_by.receipt_formatted}
                      </Link>
                    </p>
                  )}
                </div>
              </>
            )}

            {/* Review history accordion — collapsed by default */}
            {donation.review_history.length > 0 && (
              <>
                <Separator className="my-4" />
                <details className="group">
                  <summary className="flex cursor-pointer items-center justify-between text-[14px] font-medium text-slate-700 hover:text-slate-900">
                    {t("detail.reviewHistory")} ({donation.review_history.length})
                  </summary>
                  <div className="mt-3 flex flex-col gap-3">
                    {donation.review_history.map((entry) => (
                      <div
                        key={entry.id}
                        className="rounded-md border border-slate-200 bg-slate-50 p-3"
                      >
                        <div className="flex items-center justify-between gap-2">
                          <span className="text-[14px] font-medium text-slate-900">
                            {entry.action === "return"
                              ? t("actions.returnForEdit")
                              : t("actions.reject")}
                          </span>
                          <span className="text-[14px] text-slate-500">
                            {entry.actor_name} · {formatDateTime(entry.acted_at)}
                          </span>
                        </div>
                        <p className="mt-1 text-[14px] text-slate-700">
                          {entry.reason}
                        </p>
                      </div>
                    ))}
                  </div>
                </details>
              </>
            )}
          </div>
        </div>

        {/* ── Right panel — การดำเนินการ ─────────────────────────────────── */}
        <div className="flex flex-col gap-4 lg:w-2/5 lg:sticky lg:top-6">
          <div className="rounded-lg border border-slate-200 bg-white p-6">
            <ReviewActionPanel
              donation={{
                id: donation.id,
                status: donation.status,
                viewer_is_creator: donation.viewer_is_creator,
                can_approve: donation.can_approve,
                can_return: donation.can_return,
                can_reject: donation.can_reject,
                can_reveal_pii: donation.can_reveal_pii,
                edonation_keyed: donation.edonation_keyed,
                receipt_formatted: donation.receipt_formatted,
              }}
              onApprove={handleApprove}
              onReturn={handleReturn}
              onReject={handleReject}
              onCancel={handleCancel}
              onReissue={handleReissue}
            />
          </div>
        </div>
      </div>
    </div>
  );
}
