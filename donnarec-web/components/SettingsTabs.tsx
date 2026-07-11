"use client";

import { useEffect, useRef, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { useToast } from "@/hooks/use-toast";
import { DonnaRecApiError } from "@/lib/api";
import { TemplateEditorFields, TemplateLivePreview } from "@/components/TemplateEditor";
import { ImageUploadSlot } from "@/components/ImageUploadSlot";
import { NumberFormatEditor } from "@/components/NumberFormatEditor";
import { EdonationConfigTab } from "@/components/EdonationConfigTab";
import {
  fetchSettingsClient,
  saveSettings,
  uploadTemplateImage,
  settingsErrorMessage,
  type DeductionMultiplier,
  type ImageSlot,
  type NumberFormatYear,
  type ReceiptSettings,
  type SettingsFormValues,
} from "@/lib/settings";

const IMAGE_SLOTS: ImageSlot[] = ["letterhead", "seal", "signature", "watermark"];

const OBJECT_KEY_FIELD: Record<ImageSlot, keyof SettingsFormValues> = {
  letterhead: "letterhead_object_key",
  seal: "seal_object_key",
  signature: "signature_object_key",
  watermark: "watermark_object_key",
};

const IMAGE_ARIA_KEY: Record<ImageSlot, string> = {
  letterhead: "ariaLetterhead",
  seal: "ariaSeal",
  signature: "ariaSignature",
  watermark: "ariaWatermark",
};

function toFormValues(settings: ReceiptSettings): SettingsFormValues {
  return {
    template_html: settings.template_html,
    template_html_en: settings.template_html_en,
    section6_text_th: settings.section6_text_th,
    section6_text_en: settings.section6_text_en,
    deduction_multiplier: settings.deduction_multiplier,
    letterhead_object_key: settings.letterhead_object_key,
    seal_object_key: settings.seal_object_key,
    signature_object_key: settings.signature_object_key,
    watermark_object_key: settings.watermark_object_key,
    separator: settings.separator,
    running_no_padding: settings.running_no_padding,
    year_format: settings.year_format,
    prefix: settings.prefix,
  };
}

function emptyImageMap<T>(value: T): Record<ImageSlot, T> {
  return { letterhead: value, seal: value, signature: value, watermark: value };
}

/**
 * SettingsTabs — Screen 6: four tabs (template / images / tax text / number
 * format) + a persistent right-column live preview + a single "save all
 * tabs" button (UI-SPEC Screen 6, D-58/D-59/D-61).
 *
 * Fetches the seed via TanStack Query against the BFF (D-R1: the Keycloak
 * access token stays server-side, obtained inside the BFF route) — mirrors
 * DonationListView's client-component pattern. All four tabs' values live in
 * ONE local form-state object so "Save" issues a single PUT with everything
 * (no partial/inconsistent config states, per model.go's doc comment).
 */
export function SettingsTabs() {
  const t = useTranslations("settings");
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const { data, isLoading, isError } = useQuery({
    queryKey: ["settings"],
    queryFn: fetchSettingsClient,
  });

  const [values, setValues] = useState<SettingsFormValues | null>(null);
  const [localPreviewUrls, setLocalPreviewUrls] = useState<Record<ImageSlot, string | null>>(
    emptyImageMap<string | null>(null)
  );
  const [imageErrors, setImageErrors] = useState<Record<ImageSlot, string | null>>(
    emptyImageMap<string | null>(null)
  );
  const [uploadingSlot, setUploadingSlot] = useState<ImageSlot | null>(null);

  // FW-01: keep a ref pointing at the LATEST preview URLs so the unmount
  // cleanup revokes the live blob URLs — not the initial (all-null) snapshot
  // an empty-dep cleanup closure would otherwise capture, which leaked memory.
  const localPreviewUrlsRef = useRef(localPreviewUrls);
  localPreviewUrlsRef.current = localPreviewUrls;

  // Seed local editable state once — never clobber in-progress edits on a
  // background refetch (e.g. after a successful save invalidates the query).
  useEffect(() => {
    if (data && !values) {
      setValues(toFormValues(data));
    }
  }, [data, values]);

  // Revoke locally-created object URLs on unmount to avoid leaking memory.
  // Reads the ref at teardown so it sees whatever URLs are live at that moment.
  useEffect(() => {
    return () => {
      Object.values(localPreviewUrlsRef.current).forEach((url) => {
        if (url) URL.revokeObjectURL(url);
      });
    };
  }, []);

  const uploadMutation = useMutation({
    mutationFn: ({ slot, file }: { slot: ImageSlot; file: File }) =>
      uploadTemplateImage(slot, file),
  });

  const saveMutation = useMutation({
    mutationFn: (payload: SettingsFormValues) => saveSettings(payload),
    onSuccess: () => {
      toast({ description: t("saveSuccessToast") });
      queryClient.invalidateQueries({ queryKey: ["settings"] });
    },
    onError: (err: unknown) => {
      const isValidation =
        err instanceof DonnaRecApiError && err.error.type === "validation";
      toast({
        variant: "destructive",
        description: isValidation ? t("saveValidationError") : t("saveNetworkError"),
      });
    },
  });

  function update<K extends keyof SettingsFormValues>(key: K, value: SettingsFormValues[K]) {
    setValues((prev) => (prev ? { ...prev, [key]: value } : prev));
  }

  async function handleImageSelect(slot: ImageSlot, file: File) {
    setImageErrors((prev) => ({ ...prev, [slot]: null }));
    setUploadingSlot(slot);
    const objectUrl = URL.createObjectURL(file);
    setLocalPreviewUrls((prev) => {
      if (prev[slot]) URL.revokeObjectURL(prev[slot] as string);
      return { ...prev, [slot]: objectUrl };
    });

    try {
      const result = await uploadMutation.mutateAsync({ slot, file });
      update(OBJECT_KEY_FIELD[slot], result.object_key);
    } catch (err) {
      setImageErrors((prev) => ({ ...prev, [slot]: settingsErrorMessage(err) }));
      setLocalPreviewUrls((prev) => {
        if (prev[slot]) URL.revokeObjectURL(prev[slot] as string);
        return { ...prev, [slot]: null };
      });
    } finally {
      setUploadingSlot(null);
    }
  }

  function handleImageRemove(slot: ImageSlot) {
    // Clears the key LOCALLY — takes effect on the next "Save all tabs" PUT.
    // No dedicated remove endpoint exists (04-07 only built upload); this
    // matches D-58's "save all tabs at once" model.
    update(OBJECT_KEY_FIELD[slot], null);
    setImageErrors((prev) => ({ ...prev, [slot]: null }));
    setLocalPreviewUrls((prev) => {
      if (prev[slot]) URL.revokeObjectURL(prev[slot] as string);
      return { ...prev, [slot]: null };
    });
  }

  if (isLoading || !values) {
    return (
      <div className="flex flex-col gap-4" aria-busy="true">
        <Skeleton className="h-8 w-64" />
        <div className="flex flex-col gap-6 lg:flex-row">
          <Skeleton className="h-[480px] lg:w-[45%]" />
          <Skeleton className="h-[480px] lg:w-[55%]" />
        </div>
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
        <h1 className="text-[28px] font-semibold leading-tight text-slate-900">
          {t("pageTitle")}
        </h1>
        <Button
          type="button"
          className="min-h-[44px] bg-blue-600 text-white hover:bg-blue-700"
          onClick={() => saveMutation.mutate(values)}
          disabled={saveMutation.isPending}
          aria-busy={saveMutation.isPending}
        >
          {saveMutation.isPending ? t("saving") : t("save")}
        </Button>
      </div>

      <div className="flex flex-col gap-6 lg:flex-row lg:items-start">
        {/* Left column — Tabs (45% width min 360px, desktop) */}
        <div className="lg:min-w-[360px] lg:basis-[45%]">
          <Tabs defaultValue="template">
            <TabsList className="mb-4">
              <TabsTrigger value="template">{t("tabs.template")}</TabsTrigger>
              <TabsTrigger value="images">{t("tabs.images")}</TabsTrigger>
              <TabsTrigger value="taxText">{t("tabs.taxText")}</TabsTrigger>
              <TabsTrigger value="numberFormat">{t("tabs.numberFormat")}</TabsTrigger>
              <TabsTrigger value="edonation">{t("tabs.edonation")}</TabsTrigger>
            </TabsList>

            <TabsContent value="template">
              <TemplateEditorFields
                templateHtml={values.template_html}
                onTemplateHtmlChange={(v) => update("template_html", v)}
                templateHtmlEn={values.template_html_en}
                onTemplateHtmlEnChange={(v) => update("template_html_en", v)}
              />
            </TabsContent>

            <TabsContent value="images">
              <div className="grid grid-cols-2 gap-4">
                {IMAGE_SLOTS.map((slot) => (
                  <ImageUploadSlot
                    key={slot}
                    slot={slot}
                    label={t(`images.${slot}`)}
                    ariaLabel={t(`images.${IMAGE_ARIA_KEY[slot]}`)}
                    hasImage={!!values[OBJECT_KEY_FIELD[slot]]}
                    localPreviewUrl={localPreviewUrls[slot]}
                    uploading={uploadingSlot === slot}
                    error={imageErrors[slot]}
                    onSelectFile={(file) => void handleImageSelect(slot, file)}
                    onRemove={() => handleImageRemove(slot)}
                  />
                ))}
              </div>
            </TabsContent>

            <TabsContent value="taxText">
              <div className="flex flex-col gap-4">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="section6-th">{t("taxText.labelTh")}</Label>
                  <Textarea
                    id="section6-th"
                    className="min-h-[120px] text-[16px]"
                    value={values.section6_text_th}
                    onChange={(e) => update("section6_text_th", e.target.value)}
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="section6-en">{t("taxText.labelEn")}</Label>
                  <Textarea
                    id="section6-en"
                    className="min-h-[120px] text-[16px]"
                    value={values.section6_text_en}
                    onChange={(e) => update("section6_text_en", e.target.value)}
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="deduction-multiplier">{t("taxText.multiplierLabel")}</Label>
                  <Select
                    value={values.deduction_multiplier}
                    onValueChange={(v) =>
                      update("deduction_multiplier", v as DeductionMultiplier)
                    }
                  >
                    <SelectTrigger id="deduction-multiplier" className="max-w-[260px]">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="1x">{t("taxText.multiplier1x")}</SelectItem>
                      <SelectItem value="2x">{t("taxText.multiplier2x")}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <p className="text-[14px] text-slate-600">{t("taxText.helperText")}</p>
              </div>
            </TabsContent>

            <TabsContent value="numberFormat">
              <NumberFormatEditor
                separator={values.separator}
                onSeparatorChange={(v) => update("separator", v)}
                runningNoPadding={values.running_no_padding}
                onRunningNoPaddingChange={(v) => update("running_no_padding", v)}
                yearFormat={values.year_format}
                onYearFormatChange={(v) => update("year_format", v as NumberFormatYear)}
                prefix={values.prefix}
                onPrefixChange={(v) => update("prefix", v)}
              />
            </TabsContent>

            <TabsContent value="edonation">
              {/*
               * EdonationConfigTab is self-contained (own query/mutation/save
               * button) — it persists edonation_config, a DIFFERENT config
               * store than `values`/the top-level "Save" button above, which
               * only ever PUTs receipt settings (D-58).
               */}
              <EdonationConfigTab />
            </TabsContent>
          </Tabs>
        </div>

        {/* Right column — persistent live preview (55% width min 360px, sticky on desktop) */}
        <div className="lg:sticky lg:top-4 lg:min-w-[360px] lg:basis-[55%]">
          <TemplateLivePreview formValues={values} />
        </div>
      </div>
    </div>
  );
}
