"use client";

import { useTranslations } from "next-intl";

interface ConsentBlockProps {
  /** Controlled value — react-hook-form field value */
  checked: boolean;
  /** Called whenever checkbox changes */
  onChange: (checked: boolean) => void;
  /**
   * Consent text version rendered from system config.
   * Default "1.0" for MVP; wire to backend config endpoint in a future plan.
   */
  consentTextVersion?: string;
  /** Validation error message — shown when submit attempted without consent */
  error?: string;
  /** When true the checkbox is disabled (e.g. non-draft status view) */
  disabled?: boolean;
}

/**
 * ConsentBlock — PDPA consent checkbox for the donation create/edit form.
 *
 * UI-SPEC §Screen 2 §Section 4:
 *   - Required to SUBMIT (disabled submit button until checked)
 *   - NOT required to save draft
 *   - Consent text version shown inline with the label (D-49)
 *
 * T-03-37: consent_text_version shown from config and recorded server-side (03-03).
 * No checkbox shadcn component installed — uses native input styled with Tailwind.
 */
export function ConsentBlock({
  checked,
  onChange,
  consentTextVersion = "1.0",
  error,
  disabled = false,
}: ConsentBlockProps) {
  const t = useTranslations();

  return (
    <div className="flex flex-col gap-2">
      <label
        className={[
          "flex cursor-pointer items-start gap-3",
          disabled ? "cursor-not-allowed opacity-60" : "",
        ].join(" ")}
      >
        <input
          type="checkbox"
          checked={checked}
          onChange={(e) => !disabled && onChange(e.target.checked)}
          disabled={disabled}
          aria-required="true"
          aria-describedby={error ? "consent-error" : undefined}
          className="mt-0.5 h-4 w-4 shrink-0 cursor-pointer rounded border-slate-300 accent-blue-600 focus:ring-2 focus:ring-blue-600 focus:ring-offset-1"
        />
        <span className="text-[14px] leading-relaxed text-slate-700">
          {t("consent.label", { version: consentTextVersion })}
        </span>
      </label>

      {error && (
        <p
          id="consent-error"
          className="text-[14px] text-red-600"
          role="alert"
          aria-live="polite"
        >
          {error}
        </p>
      )}
    </div>
  );
}
