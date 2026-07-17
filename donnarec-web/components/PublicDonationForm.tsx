"use client";

import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslations, useLocale } from "next-intl";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Alert, AlertDescription } from "@/components/ui/alert";
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
import { TurnstileWidget } from "@/components/TurnstileWidget";
import { PublicDonationConfirmation } from "@/components/PublicDonationConfirmation";

/**
 * consent_text_version for the public Flow B form. Distinct from Flow A's
 * version (D-81). The Go E2E asserts "public-form-v1"; overridable by env.
 */
const PUBLIC_CONSENT_TEXT_VERSION =
  process.env.NEXT_PUBLIC_PUBLIC_CONSENT_TEXT_VERSION ?? "public-form-v1";

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

type FormValues = {
  donor_name: string;
  national_id: string;
  address: string;
  email?: string;
  amount: number;
  donated_at: string;
  note?: string;
};

function buildSchema(t: ReturnType<typeof useTranslations>) {
  return z.object({
    donor_name: z
      .string()
      .min(1, t("errors.requiredField", { fieldName: t("fields.donorName") })),
    national_id: z
      .string()
      .min(1, t("errors.requiredField", { fieldName: t("fields.nationalId") }))
      .length(13, t("errors.nationalId"))
      .regex(/^\d+$/, t("errors.nationalId")),
    address: z
      .string()
      .min(1, t("errors.requiredField", { fieldName: t("fields.address") })),
    email: z
      .string()
      .optional()
      .refine((v) => !v || EMAIL_RE.test(v), {
        message: t("errors.invalidEmail"),
      }),
    amount: z
      .number({ error: t("errors.amountPositive") })
      .positive(t("errors.amountPositive")),
    donated_at: z
      .string()
      .min(1, t("errors.requiredField", { fieldName: t("fields.donatedAt") }))
      .refine(
        (v) => {
          const d = new Date(v);
          return !isNaN(d.valueOf()) && d <= new Date();
        },
        { message: t("errors.validation") }
      ),
    note: z.string().optional(),
  });
}

/**
 * PublicDonationForm — Screen 9 (warm theme). The donor-facing public form
 * that reuses Flow A's field set but adds a MANDATORY slip (D-80), the Flow-B
 * PDPA consent (D-81), and a Turnstile CAPTCHA (D-82), then submits through the
 * session-less /api/public/donations passthrough to Go.
 *
 * Gating: submit is disabled until all required fields are valid AND a slip is
 * attached AND consent is checked AND a Turnstile token is present. The locale
 * (useLocale) is the single source of truth for both the rendered language and
 * the donor_language sent on submit (FR-06).
 *
 * On success it swaps in-page to PublicDonationConfirmation showing the returned
 * reference number (no navigation, no query string, D-84/D-86). On error it
 * shows an inline warm-destructive Alert and preserves the form + slip.
 */
export function PublicDonationForm() {
  const t = useTranslations();
  const tp = useTranslations("publicDonation");
  const locale = useLocale() as "th" | "en";

  const form = useForm<FormValues>({
    resolver: zodResolver(buildSchema(t)),
    mode: "onChange",
    defaultValues: {
      donor_name: "",
      national_id: "",
      address: "",
      email: "",
      amount: undefined as unknown as number,
      donated_at: "",
      note: "",
    },
  });

  const { control, handleSubmit, watch, reset } = form;
  const emailValue = watch("email");

  const [pendingSlipFile, setPendingSlipFile] = useState<File | null>(null);
  const [consentChecked, setConsentChecked] = useState(false);
  const [turnstileToken, setTurnstileToken] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [serverError, setServerError] = useState<string | null>(null);
  const [confirmation, setConfirmation] = useState<{
    reference: string;
    hasEmail: boolean;
  } | null>(null);

  const isValid = form.formState.isValid;
  const canSubmit =
    isValid &&
    consentChecked &&
    !!pendingSlipFile &&
    !!turnstileToken &&
    !submitting;

  // Slip is mandatory (D-80): once everything else is ready, an absent slip
  // surfaces the slip-required copy instead of silently blocking submit.
  const showSlipRequired =
    isValid && consentChecked && !!turnstileToken && !pendingSlipFile;

  const onSubmit = handleSubmit(async (values) => {
    if (!pendingSlipFile || !turnstileToken) return;
    setSubmitting(true);
    setServerError(null);

    const fd = new FormData();
    fd.set("donor_name", values.donor_name);
    fd.set("donor_tax_id", values.national_id);
    fd.set("donor_address", values.address);
    fd.set("donor_email", values.email ?? "");
    fd.set("amount", String(values.amount));
    fd.set("donated_at", values.donated_at);
    fd.set("notes", values.note ?? "");
    fd.set("consent_given", "true");
    fd.set("consent_text_version", PUBLIC_CONSENT_TEXT_VERSION);
    fd.set("consent_purpose", "tax-receipt");
    fd.set("donor_language", locale);
    fd.set("turnstile_token", turnstileToken);
    fd.set("slip", pendingSlipFile, pendingSlipFile.name);

    let res: Response;
    try {
      res = await fetch("/api/public/donations", {
        method: "POST",
        body: fd,
      });
    } catch {
      setServerError(tp("submitError"));
      setSubmitting(false);
      return;
    }

    if (res.ok) {
      let reference = "";
      try {
        const json = await res.json();
        reference = json?.data?.reference_number ?? "";
      } catch {
        // fall through to error handling below
      }
      if (reference) {
        setConfirmation({ reference, hasEmail: !!values.email });
        setSubmitting(false);
        return;
      }
      setServerError(tp("submitError"));
      setSubmitting(false);
      return;
    }

    // Map failure to warm-destructive copy; preserve form + slip + token.
    if (res.status === 429) {
      setServerError(tp("rateLimit"));
    } else if (res.status === 400 || res.status === 403) {
      // captcha rejection surfaces as a 4xx from the Go captcha middleware
      setServerError(tp("captchaFailed"));
    } else {
      setServerError(tp("submitError"));
    }
    setSubmitting(false);
  });

  function handleSubmitAnother() {
    reset();
    setPendingSlipFile(null);
    setConsentChecked(false);
    setTurnstileToken(null);
    setServerError(null);
    setConfirmation(null);
  }

  if (confirmation) {
    return (
      <main className="px-4 py-6 sm:py-8 md:py-10">
        <PublicDonationConfirmation
          referenceNumber={confirmation.reference}
          hasEmail={confirmation.hasEmail}
          onSubmitAnother={handleSubmitAnother}
        />
      </main>
    );
  }

  return (
    <main className="px-4 py-6 sm:py-8 md:py-10">
      <div className="mx-auto w-full max-w-[560px]">
        <h1
          className="text-[24px] font-medium leading-tight text-primary sm:text-[28px]"
          style={{ fontFamily: "var(--font-trirong)" }}
        >
          {tp("pageTitle")}
        </h1>
        <p className="mt-2 text-[14px] leading-[1.6] text-muted-foreground">
          {tp("intro")}
        </p>

        <Form {...form}>
          <form className="mt-6 flex flex-col gap-6" noValidate onSubmit={onSubmit}>
            {/* ── Section 1: ข้อมูลผู้บริจาค ── */}
            <Card className="shadow-[var(--shadow-public-card)]">
              <CardHeader className="pb-4">
                <CardTitle
                  className="text-[20px] font-semibold text-primary"
                  style={{ fontFamily: "var(--font-trirong)" }}
                >
                  {tp("section1")}
                </CardTitle>
              </CardHeader>
              <CardContent className="flex flex-col gap-5">
                <FormField
                  control={control}
                  name="donor_name"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="text-[14px] font-semibold text-primary">
                        {t("fields.donorName")}{" "}
                        <span className="text-destructive" aria-hidden="true">*</span>
                      </FormLabel>
                      <FormControl>
                        <Input {...field} aria-required="true" className="min-h-[44px]" />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={control}
                  name="national_id"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="text-[14px] font-semibold text-primary">
                        {t("fields.nationalId")}{" "}
                        <span className="text-destructive" aria-hidden="true">*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type="text"
                          inputMode="numeric"
                          maxLength={13}
                          aria-required="true"
                          className="min-h-[44px] font-mono"
                          placeholder="1234567890123"
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={control}
                  name="address"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="text-[14px] font-semibold text-primary">
                        {t("fields.address")}{" "}
                        <span className="text-destructive" aria-hidden="true">*</span>
                      </FormLabel>
                      <FormControl>
                        <Textarea {...field} rows={3} aria-required="true" className="min-h-[80px] resize-y" />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={control}
                  name="email"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="text-[14px] font-semibold text-primary">
                        {t("fields.email")}
                      </FormLabel>
                      <FormControl>
                        <Input {...field} value={field.value ?? ""} type="email" className="min-h-[44px]" placeholder="example@email.com" />
                      </FormControl>
                      <FormDescription className="text-[13px] text-muted-foreground">
                        {tp("emailHelper")}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </CardContent>
            </Card>

            {/* ── Section 2: รายละเอียดการบริจาค ── */}
            <Card className="shadow-[var(--shadow-public-card)]">
              <CardHeader className="pb-4">
                <CardTitle
                  className="text-[20px] font-semibold text-primary"
                  style={{ fontFamily: "var(--font-trirong)" }}
                >
                  {tp("section2")}
                </CardTitle>
              </CardHeader>
              <CardContent className="flex flex-col gap-5">
                <FormField
                  control={control}
                  name="amount"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="text-[14px] font-semibold text-primary">
                        {t("fields.amount")}{" "}
                        <span className="text-destructive" aria-hidden="true">*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          step="0.01"
                          min="0.01"
                          aria-required="true"
                          className="min-h-[44px] text-right"
                          placeholder="0.00"
                          value={
                            field.value === undefined || isNaN(field.value)
                              ? ""
                              : field.value
                          }
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
                <FormField
                  control={control}
                  name="donated_at"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="text-[14px] font-semibold text-primary">
                        {t("fields.donatedAt")}{" "}
                        <span className="text-destructive" aria-hidden="true">*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type="date"
                          aria-required="true"
                          className="min-h-[44px]"
                          max={new Date().toISOString().slice(0, 10)}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={control}
                  name="note"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="text-[14px] font-semibold text-primary">
                        {t("fields.note")}
                      </FormLabel>
                      <FormControl>
                        <Textarea {...field} value={field.value ?? ""} rows={2} className="min-h-[60px] resize-y" />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </CardContent>
            </Card>

            {/* ── Section 3: สลิปการโอนเงิน (mandatory, D-80) ── */}
            <Card className="shadow-[var(--shadow-public-card)]">
              <CardHeader className="pb-4">
                <CardTitle
                  className="text-[20px] font-semibold text-primary"
                  style={{ fontFamily: "var(--font-trirong)" }}
                >
                  {tp("section3")}
                </CardTitle>
              </CardHeader>
              <CardContent className="flex flex-col gap-2">
                <SlipUploadZone
                  required
                  label={tp("slipLabel")}
                  onFileChange={setPendingSlipFile}
                  disabled={submitting}
                />
                {showSlipRequired && (
                  <p role="alert" className="text-[14px] text-destructive">
                    {tp("slipRequired")}
                  </p>
                )}
              </CardContent>
            </Card>

            {/* ── Section 4: การให้ความยินยอม PDPA (D-81) ── */}
            <Card className="shadow-[var(--shadow-public-card)]">
              <CardHeader className="pb-4">
                <CardTitle
                  className="text-[20px] font-semibold text-primary"
                  style={{ fontFamily: "var(--font-trirong)" }}
                >
                  {tp("section4")}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <ConsentBlock
                  checked={consentChecked}
                  onChange={setConsentChecked}
                  consentTextVersion={PUBLIC_CONSENT_TEXT_VERSION}
                  labelText={tp("consentLabel", {
                    version: PUBLIC_CONSENT_TEXT_VERSION,
                  })}
                  disabled={submitting}
                />
              </CardContent>
            </Card>

            {/* ── CAPTCHA (D-82) ── */}
            <TurnstileWidget
              onVerify={setTurnstileToken}
              onError={() => setTurnstileToken(null)}
              onExpire={() => setTurnstileToken(null)}
            />

            {serverError && (
              <Alert
                role="alert"
                className="border-destructive/40 bg-[hsl(var(--destructive)/0.08)] text-destructive"
              >
                <AlertDescription className="text-destructive">
                  {serverError}
                </AlertDescription>
              </Alert>
            )}

            {/* ── Submit — warm primary pill ── */}
            <Button
              type="submit"
              disabled={!canSubmit}
              className="min-h-[44px] rounded-full bg-primary text-primary-foreground hover:bg-[hsl(var(--primary))] sm:w-auto sm:self-end"
            >
              {submitting ? tp("submitting") : tp("submit")}
            </Button>
          </form>
        </Form>
      </div>
    </main>
  );
}
