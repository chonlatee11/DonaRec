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
  /**
   * Thai BE fiscal year. Defaults to the current Thai government fiscal year
   * (1 Oct – 30 Sep) expressed in Buddhist Era — see currentFiscalYearBE().
   */
  fiscalYear?: number;
  /** Sample running number for the example. Defaults to 123. */
  runningNo?: number;
}

/**
 * currentFiscalYearBE — the Thai government fiscal year (1 Oct – 30 Sep) in
 * Buddhist Era for `now`, matching the backend's canonical rule in
 * donnarec-api/internal/receiptno/fiscal_year.go:
 *   - Oct–Dec of CE year Y → BE fiscal year Y + 544
 *   - Jan–Sep of CE year Y → BE fiscal year Y + 543
 *
 * JS Date#getMonth() is 0-indexed, so October === 9. Browser local time is
 * acceptable for this display-only example; the point is to match the
 * fiscal-year rule so the live example doesn't lag the frozen number during
 * Oct–Dec (FW-04). The backend normalises to Asia/Bangkok — a display example
 * in the admin's local time is close enough for the intended illustration.
 */
export function currentFiscalYearBE(now: Date = new Date()): number {
  const ceYear = now.getFullYear();
  return now.getMonth() >= 9 ? ceYear + 544 : ceYear + 543;
}

export function formatReceiptNumberExample({
  prefix,
  separator,
  runningNoPadding,
  yearFormat,
  fiscalYear,
  runningNo = 123,
}: ReceiptNumberFormatInput): string {
  const be = fiscalYear ?? currentFiscalYearBE();
  const yearStr =
    yearFormat === "CE4"
      ? String(be - 543).padStart(4, "0")
      : String(be).padStart(4, "0");

  const padding = Math.max(0, runningNoPadding | 0);
  const runningStr = String(Math.max(0, runningNo)).padStart(padding, "0");

  return `${prefix}${yearStr}${separator}${runningStr}`;
}
