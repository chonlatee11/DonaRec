import { describe, it, expect, afterEach, vi } from "vitest";
import { formatReceiptNumberExample } from "@/lib/receipt-number-format";

/**
 * formatReceiptNumberExample — hermetic unit tests (04-08, NumberFormatEditor's
 * client-computed live example, UI-SPEC Screen 6 Tab 4).
 *
 * Ports donnarec-api/internal/receiptno/format.go's formatReceiptNo algorithm
 * EXACTLY (prefix + yearStr + separator + zero-padded runningNo, D-29 min-width
 * expansion, D-28 BE4/CE4 year rendering) so the FE example never drifts from
 * what the backend will actually freeze at allocation time. Test cases are the
 * literal examples documented in format.go's own doc comment.
 */
describe("formatReceiptNumberExample", () => {
  it("matches format.go's documented default example: 2569/000123", () => {
    expect(
      formatReceiptNumberExample({
        prefix: "",
        separator: "/",
        runningNoPadding: 6,
        yearFormat: "BE4",
        fiscalYear: 2569,
        runningNo: 123,
      })
    ).toBe("2569/000123");
  });

  it("expands beyond padding width without truncation (D-29)", () => {
    expect(
      formatReceiptNumberExample({
        prefix: "",
        separator: "/",
        runningNoPadding: 6,
        yearFormat: "BE4",
        fiscalYear: 2569,
        runningNo: 1000000,
      })
    ).toBe("2569/1000000");
  });

  it("applies a prefix and custom separator/padding: HOSP2569-0005", () => {
    expect(
      formatReceiptNumberExample({
        prefix: "HOSP",
        separator: "-",
        runningNoPadding: 4,
        yearFormat: "BE4",
        fiscalYear: 2569,
        runningNo: 5,
      })
    ).toBe("HOSP2569-0005");
  });

  it("renders CE4 as fiscalYear - 543: 2026/000007", () => {
    expect(
      formatReceiptNumberExample({
        prefix: "",
        separator: "/",
        runningNoPadding: 6,
        yearFormat: "CE4",
        fiscalYear: 2569,
        runningNo: 7,
      })
    ).toBe("2026/000007");
  });

  it("defaults runningNo to 123 when omitted", () => {
    // fiscalYear supplied explicitly so this stays wall-clock-independent.
    expect(
      formatReceiptNumberExample({
        prefix: "",
        separator: "/",
        runningNoPadding: 6,
        yearFormat: "BE4",
        fiscalYear: 2569,
      })
    ).toBe("2569/000123");
  });

  /**
   * Default fiscal-year derivation must mirror the backend's canonical rule
   * (donnarec-api/internal/receiptno/fiscal_year.go):
   *   - Thai govt fiscal year = 1 Oct – 30 Sep, expressed in BE (CE + 543)
   *   - Oct–Dec of CE year Y → BE fiscal year Y + 544
   *   - Jan–Sep of CE year Y → BE fiscal year Y + 543
   * The counter is keyed per fiscal year, so during Oct–Dec the flat "+543"
   * (calendar BE) rendered a year one behind what the backend freezes (FW-04).
   */
  describe("default fiscal year (Oct-rollover rule, mirrors backend fiscal_year.go)", () => {
    afterEach(() => {
      vi.useRealTimers();
    });

    it("Oct–Dec: derives fiscal BE year as CE + 544", () => {
      // 15 Oct 2025 CE → fiscal year starting 1 Oct 2025 → BE 2569.
      vi.useFakeTimers();
      vi.setSystemTime(new Date(2025, 9, 15, 12, 0, 0)); // month index 9 = October
      expect(
        formatReceiptNumberExample({
          prefix: "",
          separator: "/",
          runningNoPadding: 6,
          yearFormat: "BE4",
        })
      ).toBe("2569/000123");
    });

    it("Jan–Sep: derives fiscal BE year as CE + 543", () => {
      // 15 Mar 2026 CE → fiscal year that started 1 Oct 2025 → BE 2569.
      vi.useFakeTimers();
      vi.setSystemTime(new Date(2026, 2, 15, 12, 0, 0)); // month index 2 = March
      expect(
        formatReceiptNumberExample({
          prefix: "",
          separator: "/",
          runningNoPadding: 6,
          yearFormat: "BE4",
        })
      ).toBe("2569/000123");
    });
  });
});
