"use client";

import { useEffect, useRef } from "react";
import { useTranslations } from "next-intl";
import { CheckCircle, Copy } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { useToast } from "@/hooks/use-toast";

interface PublicDonationConfirmationProps {
  /** The REF-xxxx reference number returned by the API (NOT a receipt number, D-84). */
  referenceNumber: string;
  /** Whether the donor supplied an email — drives the no-email advisory. */
  hasEmail: boolean;
  /** Resets local form state and swaps back to the form (Screen 9). */
  onSubmitAnother: () => void;
}

/**
 * PublicDonationConfirmation — Screen 10 (warm theme). An in-page swap shown
 * after a successful submit. It acknowledges receipt WITHOUT implying a receipt
 * has been issued: the gold-wash CheckCircle well is deliberately "acknowledged,
 * not done", and the body carries the non-negotiable "not yet a receipt" clause
 * (FR-05/D-84).
 *
 * The reference number is rendered in IBM Plex Mono (visually distinct from the
 * slate/blue back-office receipt number) with a copy-to-clipboard button. If no
 * email was provided, a gold-wash advisory tells the donor to quote the
 * reference when contacting staff (D-86 — no status portal is built).
 *
 * The card receives programmatic focus on mount and is wrapped in an
 * aria-live="polite" region (Accessibility Contract).
 */
export function PublicDonationConfirmation({
  referenceNumber,
  hasEmail,
  onSubmitAnother,
}: PublicDonationConfirmationProps) {
  const t = useTranslations("publicDonation");
  const { toast } = useToast();
  const cardRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    cardRef.current?.focus();
  }, []);

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(referenceNumber);
      toast({ description: t("confirmation.copied") });
    } catch {
      // Clipboard unavailable (e.g. insecure context) — silently ignore; the
      // reference is visible on-screen for manual copy.
    }
  }

  return (
    <div aria-live="polite" className="mx-auto w-full max-w-[560px]">
      <Card
        ref={cardRef}
        tabIndex={-1}
        className="border-border shadow-[var(--shadow-public-card)] outline-none"
      >
        <CardContent className="flex flex-col items-center gap-5 px-4 py-8 text-center sm:px-8">
          {/* Icon well — gold-wash circle, pine CheckCircle (acknowledged, not done) */}
          <div className="flex h-10 w-10 items-center justify-center rounded-full bg-accent sm:h-12 sm:w-12">
            <CheckCircle className="h-6 w-6 text-primary sm:h-7 sm:w-7" aria-hidden="true" />
          </div>

          <h1
            className="text-[24px] font-medium leading-tight text-primary sm:text-[28px]"
            style={{ fontFamily: "var(--font-trirong)" }}
          >
            {t("confirmation.heading")}
          </h1>

          <p className="text-[16px] leading-[1.6] text-foreground">
            {t("confirmation.body")}
          </p>

          {/* Reference number block — IBM Plex Mono, paper-2 inset panel */}
          <div className="w-full">
            <p className="mb-2 text-[14px] text-muted-foreground">
              {t("confirmation.referenceLabel")}
            </p>
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-[12px] border border-border bg-secondary px-4 py-3">
              <span
                className="text-[16px] font-medium text-foreground"
                style={{ fontFamily: "var(--font-ibm-plex-mono)" }}
              >
                {referenceNumber}
              </span>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={handleCopy}
                aria-label={t("confirmation.copyAria")}
                className="h-11 min-w-11 text-primary hover:bg-accent"
              >
                <Copy className="mr-1.5 h-4 w-4" aria-hidden="true" />
                {t("confirmation.copy")}
              </Button>
            </div>
            <p className="mt-2 text-[14px] text-muted-foreground">
              {t("confirmation.referenceHelper")}
            </p>
          </div>

          {/* No-email advisory (gold-wash / pine) */}
          {!hasEmail && (
            <Alert className="border-border bg-accent text-left text-primary">
              <AlertDescription className="text-primary">
                {t("confirmation.noEmail")}
              </AlertDescription>
            </Alert>
          )}

          {/* Submit another — outline pill, secondary action */}
          <Button
            type="button"
            variant="outline"
            onClick={onSubmitAnother}
            className="min-h-11 rounded-full border-border text-primary hover:bg-accent"
          >
            {t("confirmation.submitAnother")}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
