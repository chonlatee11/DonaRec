"use client";

import { useState } from "react";
import { Turnstile } from "@marsidev/react-turnstile";
import { useTranslations } from "next-intl";

/**
 * TurnstileWidget — thin wrapper around @marsidev/react-turnstile (Cloudflare
 * Turnstile) for the public donation form (Screen 9, D-82).
 *
 * Surfaces three callbacks to the parent PublicDonationForm:
 *   - onVerify(token): the CAPTCHA passed — the parent stores the token and
 *     later sends it as the multipart `turnstile_token` field (captcha.TokenField).
 *   - onError: the widget failed to load or verification failed — the parent
 *     clears any held token so submit re-gates.
 *   - onExpire: a previously-issued token expired — the parent clears it.
 *
 * The site key is read from NEXT_PUBLIC_TURNSTILE_SITE_KEY (client-exposed by
 * design — the SECRET key lives only in the Go verifier, never here). Load- and
 * verification-failure copy is rendered inline with role="alert" using the warm
 * destructive token (06-UI-SPEC Copywriting Contract).
 *
 * Turnstile renders its own branded chrome that cannot be restyled to the warm
 * palette — expected and acceptable (06-UI-SPEC Registry Safety note).
 */
const SITE_KEY = process.env.NEXT_PUBLIC_TURNSTILE_SITE_KEY ?? "";

interface TurnstileWidgetProps {
  /** Called with the verification token when the CAPTCHA passes. */
  onVerify: (token: string) => void;
  /** Called when the widget fails to load or verification fails. */
  onError?: () => void;
  /** Called when a previously-issued token expires. */
  onExpire?: () => void;
}

export function TurnstileWidget({
  onVerify,
  onError,
  onExpire,
}: TurnstileWidgetProps) {
  const t = useTranslations("publicDonation");
  const [failure, setFailure] = useState<null | "load" | "verify">(null);

  return (
    <div className="flex flex-col items-center gap-2">
      <Turnstile
        siteKey={SITE_KEY}
        options={{ theme: "light" }}
        onSuccess={(token) => {
          setFailure(null);
          onVerify(token);
        }}
        onError={(err?: unknown) => {
          // Cloudflare surfaces network/script-load failures with error codes
          // that begin with "network" — treat those as a load failure, all
          // other codes as a verification failure.
          setFailure(String(err ?? "").includes("network") ? "load" : "verify");
          onError?.();
        }}
        onExpire={() => {
          onExpire?.();
        }}
      />
      {failure && (
        <p role="alert" className="text-[14px] text-destructive">
          {failure === "load" ? t("captchaLoadFailed") : t("captchaFailed")}
        </p>
      )}
    </div>
  );
}
