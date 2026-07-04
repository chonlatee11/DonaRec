import { describe, it, expect } from "vitest";
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

  it("defaults fiscalYear to the current Buddhist-era year and runningNo to 123 when omitted", () => {
    const expectedBEYear = new Date().getFullYear() + 543;
    expect(
      formatReceiptNumberExample({
        prefix: "",
        separator: "/",
        runningNoPadding: 6,
        yearFormat: "BE4",
      })
    ).toBe(`${expectedBEYear}/000123`);
  });
});
