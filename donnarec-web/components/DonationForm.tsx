"use client";

import { useState, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslations, useLocale } from "next-intl";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { format } from "date-fns";
import { CalendarIcon, AlertTriangle, ArrowLeft } from "lucide-react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Calendar } from "@/components/ui/calendar";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { ConsentBlock } from "@/components/ConsentBlock";
import { SlipUploadZone } from "@/components/SlipUploadZone";
import { useToast } from "@/hooks/use-toast";
import { cn } from "@/lib/utils";

import {
  createDonation,
  updateDraft,
  submitDonation,
  uploadSlip,
  removeSlip,
  apiErrorMessage,
} from "@/lib/donations";
import type { CreateDonationRequest, UpdateDraftRequest } from "@/lib/donations";

// ---------------------------------------------------------------------------
// Form schema
// ---------------------------------------------------------------------------

/**
 * Zod schema for both create and edit modes.
 *
 * national_id is REQUIRED in BOTH modes (D-R3 contract-alignment fix, 03-13):
 * the Go UpdateDraftRequest re-encrypts the tax ID on every PUT and rejects
 * an empty value (missing_tax_id, 422) — there is no server-side "leave blank
 * to keep existing" path. Because GetByID only ever returns the masked value
 * (T-03-09 — plaintext is never returned, even to the creating Maker), the
 * Maker must re-enter the full 13-digit ID on every save, in both modes.
 */
function buildSchema() {
  const nationalId = z
    .string()
    .min(1, "กรุณากรอก เลขประจำตัวผู้เสียภาษี / เลขบัตรประชาชน")
    .length(13, "เลขประจำตัว/เลขผู้เสียภาษีต้องเป็นตัวเลข 13 หลัก")
    .regex(/^\d+$/, "เลขประจำตัว/เลขผู้เสียภาษีต้องเป็นตัวเลข 13 หลัก");

  return z.object({
    donor_name: z
      .string()
      .min(1, "กรุณากรอก ชื่อ-นามสกุลผู้บริจาค"),
    national_id: nationalId,
    address: z.string().min(1, "กรุณากรอก ที่อยู่ผู้บริจาค"),
    email: z
      .string()
      .optional()
      .refine(
        (val) => !val || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(val),
        { message: "รูปแบบอีเมลไม่ถูกต้อง" }
      ),
    amount: z
      .number({ error: "กรุณากรอกจำนวนเงิน" })
      .positive("จำนวนเงินต้องมากกว่า 0"),
    donated_at: z
      .date({ error: "กรุณาเลือกวันที่บริจาค" })
      .refine((d) => d <= new Date(), {
        message: "วันที่บริจาคต้องไม่เป็นวันในอนาคต",
      }),
    note: z.string().optional(),
    consent: z.boolean().optional(),
    donor_language: z.enum(["th", "en"]),
  });
}

type FormValues = {
  donor_name: string;
  national_id: string;
  address: string;
  email?: string;
  amount: number;
  donated_at: Date;
  note?: string;
  consent?: boolean;
  /** D-55/FR-23: document language driving PDF/email rendering, default "th" */
  donor_language: "th" | "en";
};

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface DonationFormProps {
  mode: "create" | "edit";
  donationId?: string;
  /** Pre-populated values for edit mode (from a server-side getDonation fetch) */
  initialData?: {
    donor_name?: string;
    address?: string;
    email?: string | null;
    amount?: number;
    donated_at?: string; // ISO YYYY-MM-DD
    note?: string | null;
    slip_url?: string | null;
    review_history?: Array<{ action: string; reason: string }>;
    /** D-55/FR-23: pre-populates the edit-mode Select; defaults to "th" if absent */
    donor_language?: "th" | "en";
  };
}

// ---------------------------------------------------------------------------
// Consent text version
// ---------------------------------------------------------------------------

/** MVP: read from env var; wire to backend config endpoint in future plan */
const CONSENT_TEXT_VERSION =
  process.env.NEXT_PUBLIC_CONSENT_TEXT_VERSION ?? "1.0";

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

/**
 * DonationForm — create/edit form for donor details (Screen 2).
 *
 * 03-13: create/update/submit/slip mutations now run CLIENT-side via
 * useMutation against lib/donations.ts's BFF-backed functions (D-R1) instead
 * of Server Action props — the Keycloak token stays server-side inside the
 * BFF route handlers.
 *
 * UI-SPEC §Screen 2:
 *   - 4 Card sections: Donor Info / Donation Details / Slip / Consent
 *   - Max-width 680px, single column
 *   - "บันทึกร่าง": saves without consent validation
 *   - "ส่งรอตรวจสอบ": full validation incl consent; disabled until consent checked
 *   - Return-from-checker amber alert shown when review_history has a "return" entry
 *   - beforeunload dirty guard
 */
export function DonationForm({
  mode,
  donationId,
  initialData,
}: DonationFormProps) {
  const t = useTranslations();
  const locale = useLocale() as "th" | "en";
  const router = useRouter();
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const schema = buildSchema();

  // Parse initial date for edit mode
  const initialDate = initialData?.donated_at
    ? new Date(initialData.donated_at + "T00:00:00")
    : undefined;

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      donor_name: initialData?.donor_name ?? "",
      national_id: "",
      address: initialData?.address ?? "",
      email: initialData?.email ?? "",
      amount: initialData?.amount ?? (undefined as unknown as number),
      donated_at: initialDate,
      note: initialData?.note ?? "",
      consent: false,
      donor_language: initialData?.donor_language ?? "th",
    },
    mode: "onTouched",
  });

  const {
    control,
    handleSubmit,
    watch,
    formState: { isDirty },
  } = form;

  const consentChecked = watch("consent") ?? false;

  // ── Current record id — starts from the prop, updated after the first
  //    successful create so a subsequent "save"/"submit" click updates the
  //    same draft instead of creating a duplicate. ─────────────────────────

  const [currentId, setCurrentId] = useState<string | undefined>(donationId);

  // ── Which top-level action is currently in flight ──────────────────────────
  // Distinguishes "save draft" from "submit for review" even though both share
  // the same create/update mutations underneath (persistDraft).
  const [activeAction, setActiveAction] = useState<"save" | "submit" | null>(
    null
  );

  // ── Pending slip state ────────────────────────────────────────────────────

  const [pendingSlipFile, setPendingSlipFile] = useState<File | null>(null);
  const [slipServerError, setSlipServerError] = useState<string | undefined>();
  const [existingSlipUrl, setExistingSlipUrl] = useState<string | null>(
    initialData?.slip_url ?? null
  );

  // ── Mutations (D-R1: client BFF calls, 03-13) ─────────────────────────────

  const createMutation = useMutation({
    mutationFn: (body: CreateDonationRequest) => createDonation(body),
  });
  const updateMutation = useMutation({
    mutationFn: (vars: { id: string; body: UpdateDraftRequest }) =>
      updateDraft(vars.id, vars.body),
  });
  const submitMutation = useMutation({
    mutationFn: (id: string) => submitDonation(id),
  });
  const uploadSlipMutation = useMutation({
    mutationFn: (vars: { id: string; formData: FormData }) =>
      uploadSlip(vars.id, vars.formData),
  });
  const removeSlipMutation = useMutation({
    mutationFn: (id: string) => removeSlip(id),
  });

  const isSaving = activeAction === "save";
  const isSubmittingForReview = activeAction === "submit";
  const isAnyPending = activeAction !== null || removeSlipMutation.isPending;

  // ── Return-from-checker alert ─────────────────────────────────────────────

  const reviewReason = initialData?.review_history
    ?.slice()
    .reverse()
    .find((e) => e.action === "return")?.reason ?? null;

  const hasBeenReturned = !!reviewReason;

  // ── beforeunload dirty guard ──────────────────────────────────────────────

  useEffect(() => {
    const handler = (e: BeforeUnloadEvent) => {
      if (isDirty || pendingSlipFile) {
        e.preventDefault();
      }
    };
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [isDirty, pendingSlipFile]);

  // ── Date formatting for display ───────────────────────────────────────────

  function formatDateDisplay(date: Date | undefined): string {
    if (!date) return t("form.selectDate");
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

  // ── Invalidate cached queries after a successful write ────────────────────

  function invalidateDonationQueries(id: string) {
    queryClient.invalidateQueries({ queryKey: ["donation", id] });
    queryClient.invalidateQueries({ queryKey: ["donations"] });
  }

  // ── Upload slip helper (called after draft save) ──────────────────────────

  async function uploadPendingSlip(id: string): Promise<string | null> {
    if (!pendingSlipFile) return null;
    setSlipServerError(undefined);
    const fd = new FormData();
    fd.append("file", pendingSlipFile);
    try {
      await uploadSlipMutation.mutateAsync({ id, formData: fd });
      setPendingSlipFile(null);
      return null;
    } catch (err) {
      const message = apiErrorMessage(err);
      setSlipServerError(message);
      return message;
    }
  }

  // ── Remove existing slip ──────────────────────────────────────────────────

  const handleRemoveExistingSlip = useCallback(async () => {
    const id = currentId;
    if (!id) return;
    try {
      await removeSlipMutation.mutateAsync(id);
      setExistingSlipUrl(null);
      invalidateDonationQueries(id);
      toast({ description: "ลบสลิปเรียบร้อยแล้ว" });
    } catch (err) {
      toast({ variant: "destructive", description: apiErrorMessage(err) });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentId, removeSlipMutation, toast]);

  // ── Save (create or update) the draft; returns the record id or null ──────

  async function persistDraft(
    values: FormValues,
    consentGiven: boolean
  ): Promise<string | null> {
    const donorFields = {
      donor_name: values.donor_name,
      national_id: values.national_id,
      address: values.address,
      ...(values.email ? { email: values.email } : {}),
      amount: values.amount,
      donated_at: format(values.donated_at, "yyyy-MM-dd"),
      ...(values.note ? { note: values.note } : {}),
      consent_given: consentGiven,
      consent_text_version: CONSENT_TEXT_VERSION,
      donor_language: values.donor_language,
    };

    try {
      if (currentId) {
        const updated = await updateMutation.mutateAsync({
          id: currentId,
          body: donorFields as UpdateDraftRequest,
        });
        invalidateDonationQueries(currentId);
        return updated.id;
      }
      const created = await createMutation.mutateAsync(
        donorFields as CreateDonationRequest
      );
      setCurrentId(created.id);
      return created.id;
    } catch (err) {
      toast({
        variant: "destructive",
        title: "เกิดข้อผิดพลาด",
        description: apiErrorMessage(err),
      });
      return null;
    }
  }

  // ── Handle "บันทึกร่าง" ───────────────────────────────────────────────────

  const handleSaveDraftClick = handleSubmit(async (values) => {
    setActiveAction("save");
    try {
      const savedId = await persistDraft(values, values.consent ?? false);
      if (!savedId) return;

      if (pendingSlipFile) {
        const slipErr = await uploadPendingSlip(savedId);
        if (slipErr) return; // slip error displayed inline
      }

      invalidateDonationQueries(savedId);
      toast({ description: t("form.savedSuccess") });
      if (mode === "create") {
        router.push(`/donations/${savedId}/edit`);
      } else {
        router.refresh();
      }
    } finally {
      setActiveAction(null);
    }
  });

  // ── Handle "ส่งรอตรวจสอบ" ─────────────────────────────────────────────────

  const handleSubmitForReviewClick = handleSubmit(async (values) => {
    if (!consentChecked) {
      toast({
        variant: "destructive",
        description: t("errors.consentRequired"),
      });
      return;
    }

    setActiveAction("submit");
    try {
      const savedId = await persistDraft(values, true);
      if (!savedId) return;

      if (pendingSlipFile) {
        const slipErr = await uploadPendingSlip(savedId);
        if (slipErr) return;
      }

      try {
        await submitMutation.mutateAsync(savedId);
      } catch (err) {
        toast({
          variant: "destructive",
          description: apiErrorMessage(err),
        });
        return;
      }

      invalidateDonationQueries(savedId);
      queryClient.invalidateQueries({ queryKey: ["donations"] });
      toast({ description: t("form.submitSuccess") });
      router.push(`/donations/${savedId}`);
    } finally {
      setActiveAction(null);
    }
  });

  // ── Render ────────────────────────────────────────────────────────────────

  return (
    <div className="flex flex-col gap-6">
      {/* Back link */}
      <Link
        href="/donations"
        className="inline-flex items-center gap-1.5 text-[14px] text-blue-600 hover:underline"
      >
        <ArrowLeft className="h-3.5 w-3.5" />
        {t("form.backToList")}
      </Link>

      {/* Page title */}
      <h1 className="text-[28px] font-semibold leading-tight text-slate-900">
        {mode === "create" ? t("form.newTitle") : t("form.editTitle")}
      </h1>

      {/* Return-from-checker amber alert */}
      {hasBeenReturned && reviewReason && (
        <div
          className="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3"
          role="alert"
        >
          <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-amber-600" />
          <p className="text-[14px] text-amber-800">
            {t("form.returnedAlert", { reason: reviewReason })}
          </p>
        </div>
      )}

      <Form {...form}>
        <form className="flex flex-col gap-6" noValidate>
          {/* ── Section 1: ข้อมูลผู้บริจาค ──────────────────────────────── */}
          <Card>
            <CardHeader className="pb-4">
              <CardTitle className="text-[20px] font-semibold text-slate-900">
                {t("form.section1")}
              </CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-5">
              {/* ชื่อ-นามสกุล */}
              <FormField
                control={control}
                name="donor_name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-[14px] text-slate-700">
                      {t("fields.donorName")}{" "}
                      <span className="text-red-600" aria-hidden="true">*</span>
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        aria-required="true"
                        className="min-h-[44px]"
                        placeholder="ชื่อ-นามสกุลผู้บริจาค"
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* เลขประจำตัวผู้เสียภาษี / เลขบัตรประชาชน */}
              <FormField
                control={control}
                name="national_id"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-[14px] text-slate-700">
                      {t("fields.nationalId")}{" "}
                      <span className="text-red-600" aria-hidden="true">*</span>
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ""}
                        type="text"
                        inputMode="numeric"
                        maxLength={13}
                        aria-required="true"
                        className="min-h-[44px] font-mono"
                        placeholder="1234567890123"
                      />
                    </FormControl>
                    {mode === "edit" && (
                      <FormDescription className="text-[13px] text-slate-500">
                        {t("form.nationalIdEditHelper")}
                      </FormDescription>
                    )}
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* ที่อยู่ */}
              <FormField
                control={control}
                name="address"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-[14px] text-slate-700">
                      {t("fields.address")}{" "}
                      <span className="text-red-600" aria-hidden="true">*</span>
                    </FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        rows={3}
                        aria-required="true"
                        className="min-h-[80px] resize-y"
                        placeholder="ที่อยู่ของผู้บริจาค"
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* อีเมล (optional) */}
              <FormField
                control={control}
                name="email"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-[14px] text-slate-700">
                      {t("fields.email")}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ""}
                        type="email"
                        className="min-h-[44px]"
                        placeholder="example@email.com"
                      />
                    </FormControl>
                    <FormDescription className="text-[13px] text-slate-500">
                      {t("form.emailHelper")}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </CardContent>
          </Card>

          {/* ── Section 2: รายละเอียดการบริจาค ──────────────────────────── */}
          <Card>
            <CardHeader className="pb-4">
              <CardTitle className="text-[20px] font-semibold text-slate-900">
                {t("form.section2")}
              </CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-5">
              {/* จำนวนเงิน */}
              <FormField
                control={control}
                name="amount"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-[14px] text-slate-700">
                      {t("fields.amount")}{" "}
                      <span className="text-red-600" aria-hidden="true">*</span>
                    </FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        step="0.01"
                        min="0.01"
                        aria-required="true"
                        className="min-h-[44px] text-right"
                        placeholder="0.00"
                        value={field.value === undefined || isNaN(field.value) ? "" : field.value}
                        onChange={(e) => {
                          const v = e.target.value;
                          field.onChange(v === "" ? undefined : parseFloat(v));
                        }}
                        onBlur={field.onBlur}
                        name={field.name}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* วันที่บริจาค — Calendar Popover */}
              <FormField
                control={control}
                name="donated_at"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-[14px] text-slate-700">
                      {t("fields.donatedAt")}{" "}
                      <span className="text-red-600" aria-hidden="true">*</span>
                    </FormLabel>
                    <Popover>
                      <PopoverTrigger asChild>
                        <FormControl>
                          <Button
                            type="button"
                            variant="outline"
                            aria-required="true"
                            className={cn(
                              "min-h-[44px] w-full justify-start text-left font-normal",
                              !field.value && "text-slate-400"
                            )}
                          >
                            <CalendarIcon className="mr-2 h-4 w-4 opacity-50" />
                            {formatDateDisplay(field.value)}
                          </Button>
                        </FormControl>
                      </PopoverTrigger>
                      <PopoverContent className="w-auto p-0" align="start">
                        <Calendar
                          mode="single"
                          selected={field.value}
                          onSelect={field.onChange}
                          disabled={(date) => date > new Date()}
                        />
                      </PopoverContent>
                    </Popover>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* หมายเหตุ (optional) */}
              <FormField
                control={control}
                name="note"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-[14px] text-slate-700">
                      {t("fields.note")}
                    </FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        value={field.value ?? ""}
                        rows={2}
                        className="min-h-[60px] resize-y"
                        placeholder="หมายเหตุเพิ่มเติม (ไม่บังคับ)"
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* ภาษาของเอกสาร — D-55/FR-23, frozen at create-time into the snapshot */}
              <FormField
                control={control}
                name="donor_language"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-[14px] text-slate-700">
                      {t("fields.donorLanguage")}
                    </FormLabel>
                    <Select value={field.value} onValueChange={field.onChange}>
                      <FormControl>
                        <SelectTrigger className="min-h-[44px]">
                          <SelectValue />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        <SelectItem value="th">{t("fields.donorLanguageTh")}</SelectItem>
                        <SelectItem value="en">{t("fields.donorLanguageEn")}</SelectItem>
                      </SelectContent>
                    </Select>
                    <FormDescription className="text-[13px] text-slate-500">
                      {t("form.donorLanguageHelper")}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </CardContent>
          </Card>

          {/* ── Section 3: สลิปการโอนเงิน ────────────────────────────────── */}
          <Card>
            <CardHeader className="pb-4">
              <CardTitle className="text-[20px] font-semibold text-slate-900">
                {t("form.section3")}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <SlipUploadZone
                existingSlipUrl={existingSlipUrl}
                onFileChange={setPendingSlipFile}
                onRemoveExisting={
                  currentId ? handleRemoveExistingSlip : undefined
                }
                uploading={uploadSlipMutation.isPending}
                serverError={slipServerError}
                disabled={isAnyPending}
              />
            </CardContent>
          </Card>

          {/* ── Section 4: การให้ความยินยอม PDPA ────────────────────────── */}
          <Card>
            <CardHeader className="pb-4">
              <CardTitle className="text-[20px] font-semibold text-slate-900">
                {t("form.section4")}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <FormField
                control={control}
                name="consent"
                render={({ field }) => (
                  <ConsentBlock
                    checked={field.value ?? false}
                    onChange={field.onChange}
                    consentTextVersion={CONSENT_TEXT_VERSION}
                    error={
                      !consentChecked && isSubmittingForReview
                        ? t("errors.consentRequired")
                        : undefined
                    }
                    disabled={isAnyPending}
                  />
                )}
              />
            </CardContent>
          </Card>

          {/* ── Form actions ──────────────────────────────────────────────── */}
          <div className="flex flex-col gap-3 sm:flex-row sm:justify-between">
            {/* "บันทึกร่าง" — outline, no consent required */}
            <Button
              type="button"
              variant="outline"
              className="min-h-[44px] sm:w-auto"
              disabled={isAnyPending}
              onClick={handleSaveDraftClick}
            >
              {isSaving ? "กำลังบันทึก..." : t("actions.saveDraft")}
            </Button>

            {/* "ส่งรอตรวจสอบ" / "ส่งรอตรวจสอบอีกครั้ง" — accent, consent required */}
            <Button
              type="button"
              className="min-h-[44px] bg-blue-600 text-white hover:bg-blue-700 sm:w-auto"
              disabled={!consentChecked || isAnyPending}
              onClick={handleSubmitForReviewClick}
              aria-disabled={!consentChecked}
            >
              {isSubmittingForReview
                ? "กำลังส่ง..."
                : hasBeenReturned
                ? t("actions.resubmit")
                : t("actions.submitForReview")}
            </Button>
          </div>
        </form>
      </Form>
    </div>
  );
}
