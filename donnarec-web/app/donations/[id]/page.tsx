import Link from "next/link";
import { notFound } from "next/navigation";
import { revalidatePath } from "next/cache";
import { getTranslations, getLocale } from "next-intl/server";
import { ExternalLink, ArrowLeft } from "lucide-react";
import { getDonation, approve, returnForEdit, reject } from "@/lib/donations";
import { DonnaRecApiError } from "@/lib/api";
import { StatusBadge } from "@/components/StatusBadge";
import { MaskedIdField } from "@/components/MaskedIdField";
import { ReviewActionPanel } from "@/components/ReviewActionPanel";
import { Separator } from "@/components/ui/separator";
import type { DonationDetail } from "@/lib/donations";

interface DonationDetailPageProps {
  params: Promise<{ id: string }>;
}

/**
 * Donation detail + review page — Screen 3.
 * Server Component: fetches record, defines server actions, renders two-column layout.
 *
 * T-03-31: SoD/RBAC is enforced server-side (03-05).
 *   viewer_is_creator + status flags from Go API drive all UI branching.
 * T-03-32: national ID masked by default; reveal audited via /pii (03-08).
 * T-03-33: 409 status_conflict surfaced as toast via ReviewActionPanel → lib/api.ts.
 */
export default async function DonationDetailPage({
  params,
}: DonationDetailPageProps) {
  const { id } = await params;
  const t = await getTranslations();
  const locale = (await getLocale()) as "th" | "en";

  // ── Fetch donation ─────────────────────────────────────────────────────────

  let donation: DonationDetail;
  try {
    donation = await getDonation(id);
  } catch (err) {
    if (err instanceof DonnaRecApiError && err.error.status === 404) {
      notFound();
    }
    // Re-throw other errors (will be caught by Next.js error boundary)
    throw err;
  }

  // ── Inline server actions ──────────────────────────────────────────────────
  // T-03-31: server enforces SoD and RBAC; returning typed error lets
  //          ReviewActionPanel map to copywriting error messages.

  async function handleApprove(): Promise<{ error: string } | null> {
    "use server";
    try {
      await approve(id);
      revalidatePath(`/donations/${id}`);
      return null;
    } catch (err) {
      const e = err as { error?: { message?: string }; message?: string };
      return { error: e?.error?.message ?? e?.message ?? "เกิดข้อผิดพลาด" };
    }
  }

  async function handleReturn(
    reason: string
  ): Promise<{ error: string } | null> {
    "use server";
    try {
      await returnForEdit(id, reason);
      revalidatePath(`/donations/${id}`);
      return null;
    } catch (err) {
      const e = err as { error?: { message?: string }; message?: string };
      return { error: e?.error?.message ?? e?.message ?? "เกิดข้อผิดพลาด" };
    }
  }

  async function handleReject(
    reason: string
  ): Promise<{ error: string } | null> {
    "use server";
    try {
      await reject(id, reason);
      revalidatePath(`/donations/${id}`);
      return null;
    } catch (err) {
      const e = err as { error?: { message?: string }; message?: string };
      return { error: e?.error?.message ?? e?.message ?? "เกิดข้อผิดพลาด" };
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

  function formatAmount(amount: number): string {
    return new Intl.NumberFormat("th-TH", {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(amount);
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

              {/* เลขประจำตัว — masked by default */}
              <div className="grid grid-cols-[180px_1fr] gap-2">
                <dt className="text-[14px] text-slate-600">
                  {t("fields.nationalId")}
                </dt>
                <dd>
                  {/* T-03-32: plaintext only via audited reveal (03-08 full flow) */}
                  <MaskedIdField
                    maskedValue={donation.national_id_masked}
                    canReveal={donation.can_reveal_pii}
                    /* onReveal wired in 03-08 */
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
              <p className="text-[14px] font-medium text-slate-700">สลิปการโอนเงิน</p>
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
              <p className="text-[14px] font-medium text-slate-700">ความยินยอม PDPA</p>
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

            {/* Review history accordion — collapsed by default (native <details>) */}
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
            {/*
             * ReviewActionPanel (Client Component):
             * Receives server actions from this Server Component.
             * Manages dialog state, toast notifications, and router.refresh().
             */}
            <ReviewActionPanel
              donation={{
                id: donation.id,
                status: donation.status,
                viewer_is_creator: donation.viewer_is_creator,
                can_approve: donation.can_approve,
                can_return: donation.can_return,
                can_reject: donation.can_reject,
              }}
              onApprove={handleApprove}
              onReturn={handleReturn}
              onReject={handleReject}
            />
          </div>
        </div>
      </div>
    </div>
  );
}
