"use client";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { ArrowDown, ArrowUp } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { useToast } from "@/hooks/use-toast";
import {
  fetchEdonationConfig,
  saveEdonationConfig,
  type EdonationConfig,
  type EdonationConfigFormValues,
} from "@/lib/edonation";

function toFormValues(config: EdonationConfig): EdonationConfigFormValues {
  return {
    field_mapping: config.field_mapping.map((col) => ({ ...col })),
    cash_type_label: config.cash_type_label,
    near_due_days: config.near_due_days,
  };
}

/**
 * EdonationConfigTab — 5th SettingsTabs tab (D-75/NFR-09, plan 05-07): an
 * Admin editor for the e-Donation export field mapping (column order +
 * Thai/English header labels), the constant cash_type_label (D-65), and the
 * near_due_days aging threshold (D-68) — loaded from and saved to
 * GET/PUT /api/bff/edonation-config.
 *
 * Self-contained (own useQuery + useMutation + Save button), following the
 * TemplateEditor dirty-state/save/toast pattern — deliberately NOT wired
 * into SettingsTabs' top-level "save all tabs" button, since it persists a
 * DIFFERENT config store (edonation_config) than the receipt template/
 * images/tax-text/number-format tabs (settings).
 *
 * Go's adminGroup.Use(RequireRoles(RoleAdmin)) remains the real authority —
 * this tab is only ever rendered within the already-Admin-gated
 * /admin/settings route (app/admin/settings/page.tsx's isAdminViewer()
 * redirect); a non-Admin caller that somehow reached the BFF route still
 * gets a 403 from Go.
 */
export function EdonationConfigTab() {
  const t = useTranslations("settings.edonation");
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const { data, isLoading, isError } = useQuery({
    queryKey: ["edonationConfig"],
    queryFn: fetchEdonationConfig,
  });

  const [values, setValues] = useState<EdonationConfigFormValues | null>(null);

  // Seed local editable state once — never clobber in-progress edits on a
  // background refetch (e.g. after a successful save invalidates the query),
  // mirroring SettingsTabs' own seed-once discipline.
  useEffect(() => {
    if (data && !values) {
      setValues(toFormValues(data));
    }
  }, [data, values]);

  const isDirty =
    !!values && !!data && JSON.stringify(values) !== JSON.stringify(toFormValues(data));

  const saveMutation = useMutation({
    mutationFn: (payload: EdonationConfigFormValues) => saveEdonationConfig(payload),
    onSuccess: () => {
      toast({ description: t("saveSuccessToast") });
      void queryClient.invalidateQueries({ queryKey: ["edonationConfig"] });
    },
    onError: () => {
      toast({ variant: "destructive", description: t("saveError") });
    },
  });

  function updateColumn(index: number, field: "header_th" | "header_en", value: string) {
    setValues((prev) => {
      if (!prev) return prev;
      const nextColumns = prev.field_mapping.map((col, i) =>
        i === index ? { ...col, [field]: value } : col
      );
      return { ...prev, field_mapping: nextColumns };
    });
  }

  function moveColumn(index: number, direction: -1 | 1) {
    setValues((prev) => {
      if (!prev) return prev;
      const target = index + direction;
      if (target < 0 || target >= prev.field_mapping.length) return prev;
      const nextColumns = [...prev.field_mapping];
      [nextColumns[index], nextColumns[target]] = [nextColumns[target], nextColumns[index]];
      return { ...prev, field_mapping: nextColumns };
    });
  }

  if (isLoading || !values) {
    return (
      <div className="flex flex-col gap-4" aria-busy="true">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-[320px] w-full" />
      </div>
    );
  }

  if (isError) {
    return (
      <div
        className="rounded-lg border border-red-200 bg-red-50 p-4 text-[14px] text-red-600"
        role="alert"
      >
        {t("loadError")}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h2 className="text-[20px] font-semibold leading-snug text-slate-900">{t("heading")}</h2>
        <Button
          type="button"
          className="min-h-[44px] bg-blue-600 text-white hover:bg-blue-700"
          onClick={() => saveMutation.mutate(values)}
          disabled={!isDirty || saveMutation.isPending}
          aria-busy={saveMutation.isPending}
        >
          {saveMutation.isPending ? t("saving") : t("save")}
        </Button>
      </div>

      {/* Field mapping — column order + Thai/English headers (D-75) */}
      <div className="flex flex-col gap-3">
        <Label>{t("fieldMappingLabel")}</Label>
        <p className="text-[14px] text-slate-600">{t("fieldMappingHelper")}</p>
        <div className="flex flex-col gap-2">
          {values.field_mapping.map((col, index) => (
            <div
              key={col.column_key}
              className="flex flex-wrap items-end gap-3 rounded-lg border border-slate-200 bg-white p-3"
            >
              <div className="flex flex-col gap-1">
                <span className="text-[14px] text-slate-600">{t("columnKeyLabel")}</span>
                <span className="font-mono text-[14px] text-slate-900">{col.column_key}</span>
              </div>
              <div className="flex min-w-[160px] flex-1 flex-col gap-1">
                <Label htmlFor={`header-th-${col.column_key}`}>{t("headerThLabel")}</Label>
                <Input
                  id={`header-th-${col.column_key}`}
                  value={col.header_th}
                  onChange={(e) => updateColumn(index, "header_th", e.target.value)}
                  className="min-h-[44px]"
                />
              </div>
              <div className="flex min-w-[160px] flex-1 flex-col gap-1">
                <Label htmlFor={`header-en-${col.column_key}`}>{t("headerEnLabel")}</Label>
                <Input
                  id={`header-en-${col.column_key}`}
                  value={col.header_en}
                  onChange={(e) => updateColumn(index, "header_en", e.target.value)}
                  className="min-h-[44px]"
                />
              </div>
              <div className="flex gap-1">
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  className="h-11 w-11"
                  disabled={index === 0}
                  aria-label={t("moveColumnUpAria", { column: col.header_th })}
                  onClick={() => moveColumn(index, -1)}
                >
                  <ArrowUp className="h-4 w-4" />
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  className="h-11 w-11"
                  disabled={index === values.field_mapping.length - 1}
                  aria-label={t("moveColumnDownAria", { column: col.header_th })}
                  onClick={() => moveColumn(index, 1)}
                >
                  <ArrowDown className="h-4 w-4" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Cash type label (D-65) + near_due_days aging threshold (D-68) */}
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-2">
          <Label htmlFor="cash-type-label">{t("cashTypeLabelLabel")}</Label>
          <Input
            id="cash-type-label"
            value={values.cash_type_label}
            onChange={(e) =>
              setValues((prev) => (prev ? { ...prev, cash_type_label: e.target.value } : prev))
            }
            className="min-h-[44px] max-w-[320px]"
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="near-due-days">{t("nearDueDaysLabel")}</Label>
          <Input
            id="near-due-days"
            type="number"
            min={1}
            value={values.near_due_days}
            onChange={(e) =>
              setValues((prev) =>
                prev ? { ...prev, near_due_days: Number(e.target.value) || 0 } : prev
              )
            }
            className="min-h-[44px] max-w-[160px]"
          />
          <p className="text-[14px] text-slate-600">{t("nearDueDaysHelper")}</p>
        </div>
      </div>
    </div>
  );
}
