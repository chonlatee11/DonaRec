/**
 * formatReceiptNumberExample — TS port of donnarec-api/internal/receiptno/format.go's
 * formatReceiptNo algorithm, used ONLY for NumberFormatEditor's client-computed live
 * example (UI-SPEC Screen 6 Tab 4: "ตัวอย่าง: 2569/000123"). This is a display-only
 * preview — the backend's formatReceiptNo (behind SELECT ... FOR UPDATE, D-42) is the
 * single source of truth for what actually gets frozen into the ledger at allocation
 * time; this function must never be used to compute or submit an actual receipt number.
 *
 * Algorithm (mirrors format.go exactly):
 *   prefix + yearStr + separator + zero-padded(runningNo, minWidth=runningNoPadding)
 *   - yearFormat "CE4" -> String(fiscalYear - 543) padded to 4 digits
 *   - yearFormat "BE4" (default) -> String(fiscalYear) padded to 4 digits
 *   - D-29: padding is a MINIMUM width — a runningNo wider than padding expands
 *     naturally, never truncates.
 */
export interface ReceiptNumberFormatInput {
  prefix: string;
  separator: string;
  runningNoPadding: number;
  yearFormat: "BE4" | "CE4";
  /** Thai BE fiscal year. Defaults to the current Buddhist-era year (CE year + 543). */
  fiscalYear?: number;
  /** Sample running number for the example. Defaults to 123. */
  runningNo?: number;
}

export function formatReceiptNumberExample({
  prefix,
  separator,
  runningNoPadding,
  yearFormat,
  fiscalYear,
  runningNo = 123,
}: ReceiptNumberFormatInput): string {
  const be = fiscalYear ?? new Date().getFullYear() + 543;
  const yearStr =
    yearFormat === "CE4"
      ? String(be - 543).padStart(4, "0")
      : String(be).padStart(4, "0");

  const padding = Math.max(0, runningNoPadding | 0);
  const runningStr = String(Math.max(0, runningNo)).padStart(padding, "0");

  return `${prefix}${yearStr}${separator}${runningStr}`;
}
