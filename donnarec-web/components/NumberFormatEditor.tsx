"use client";

import { useTranslations } from "next-intl";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { formatReceiptNumberExample } from "@/lib/receipt-number-format";
import type { NumberFormatYear } from "@/lib/settings";

interface NumberFormatEditorProps {
  separator: string;
  onSeparatorChange: (value: string) => void;
  runningNoPadding: number;
  onRunningNoPaddingChange: (value: number) => void;
  yearFormat: NumberFormatYear;
  onYearFormatChange: (value: NumberFormatYear) => void;
  prefix: string;
  onPrefixChange: (value: string) => void;
  disabled?: boolean;
}

/**
 * NumberFormatEditor — Tab 4 (UI-SPEC Screen 6): surfaces the Phase 2
 * receipt_number_config fields consolidated into this same settings screen
 * (CONTEXT.md canonical_refs). Separator + zero-pad digit count are the
 * UI-SPEC-highlighted fields; year-format + prefix are additionally required
 * here because the "save all tabs" PUT (settings/model.go's ReceiptSettings)
 * validates YearFormat as `required,oneof=BE4 CE4` — omitting it from the UI
 * would make every save fail 422 regardless of what the admin intended to
 * change on this tab.
 *
 * The live example is computed CLIENT-side (formatReceiptNumberExample,
 * mirrors internal/receiptno/format.go exactly) — this is a DISPLAY-ONLY
 * preview; the backend's row-locked allocator remains the sole source of
 * truth for what actually gets frozen into the ledger (D-42).
 */
export function NumberFormatEditor({
  separator,
  onSeparatorChange,
  runningNoPadding,
  onRunningNoPaddingChange,
  yearFormat,
  onYearFormatChange,
  prefix,
  onPrefixChange,
  disabled,
}: NumberFormatEditorProps) {
  const t = useTranslations("settings.numberFormat");

  const example = formatReceiptNumberExample({
    prefix,
    separator,
    runningNoPadding,
    yearFormat,
  });

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <Label htmlFor="number-format-separator">{t("separatorLabel")}</Label>
        <Input
          id="number-format-separator"
          value={separator}
          maxLength={4}
          onChange={(e) => onSeparatorChange(e.target.value)}
          disabled={disabled}
          className="max-w-[120px]"
        />
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="number-format-padding">{t("paddingLabel")}</Label>
        <Input
          id="number-format-padding"
          type="number"
          min={1}
          max={12}
          value={runningNoPadding}
          onChange={(e) => {
            const parsed = parseInt(e.target.value, 10);
            onRunningNoPaddingChange(Number.isNaN(parsed) ? 1 : parsed);
          }}
          disabled={disabled}
          className="max-w-[120px]"
        />
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="number-format-year">{t("yearFormatLabel")}</Label>
        <Select
          value={yearFormat}
          onValueChange={(value) => onYearFormatChange(value as NumberFormatYear)}
          disabled={disabled}
        >
          <SelectTrigger id="number-format-year" className="max-w-[220px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="BE4">{t("yearFormatBE")}</SelectItem>
            <SelectItem value="CE4">{t("yearFormatCE")}</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="number-format-prefix">{t("prefixLabel")}</Label>
        <Input
          id="number-format-prefix"
          value={prefix}
          onChange={(e) => onPrefixChange(e.target.value)}
          disabled={disabled}
          className="max-w-[220px]"
        />
      </div>

      <div>
        <p className="text-[14px] text-slate-600">{t("exampleLabel")}</p>
        <p
          className="font-mono text-[16px] font-semibold text-slate-900"
          aria-live="polite"
        >
          {example}
        </p>
      </div>

      <p className="text-[14px] text-amber-600">{t("freezeNote")}</p>
    </div>
  );
}
