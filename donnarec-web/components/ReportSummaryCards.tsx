"use client";

import { useTranslations } from "next-intl";

interface ReportSummaryCardsProps {
  totalAmount: number;
  receiptCount: number;
  averagePerReceipt: number;
}

const amountFormatter = new Intl.NumberFormat("th-TH", {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});
const countFormatter = new Intl.NumberFormat("th-TH");

/**
 * ReportSummaryCards — Screen 8: 3 stat cards (total amount / receipt count /
 * average per receipt), UI-SPEC "Summary cards" section.
 *
 * Card figures reuse the existing Page Heading style (28px/600) in
 * slate-900 — deliberately NOT accent-colored (informational, not
 * interactive, per the 10% accent budget) — a distinct rule from
 * AgingStatCards' clickable/colored bucket cards.
 */
export function ReportSummaryCards({
  totalAmount,
  receiptCount,
  averagePerReceipt,
}: ReportSummaryCardsProps) {
  const t = useTranslations("reports");

  const totalAmountText = amountFormatter.format(totalAmount);
  const receiptCountText = countFormatter.format(receiptCount);
  const averagePerReceiptText = amountFormatter.format(averagePerReceipt);

  const cards = [
    {
      key: "totalAmount",
      label: t("cardTotalAmount"),
      value: totalAmountText,
      ariaLabel: t("ariaCardTotalAmount", { value: totalAmountText }),
    },
    {
      key: "receiptCount",
      label: t("cardReceiptCount"),
      value: receiptCountText,
      ariaLabel: t("ariaCardReceiptCount", { value: receiptCountText }),
    },
    {
      key: "averagePerReceipt",
      label: t("cardAveragePerReceipt"),
      value: averagePerReceiptText,
      ariaLabel: t("ariaCardAveragePerReceipt", { value: averagePerReceiptText }),
    },
  ];

  return (
    <div className="grid grid-cols-1 gap-6 sm:grid-cols-3">
      {cards.map((card) => (
        <div
          key={card.key}
          className="flex flex-col gap-1 rounded-lg border border-slate-200 bg-white p-4"
        >
          <span className="text-[14px] text-slate-600">{card.label}</span>
          <span
            className="text-[28px] font-semibold leading-tight text-slate-900"
            aria-label={card.ariaLabel}
          >
            {card.value}
          </span>
        </div>
      ))}
    </div>
  );
}
