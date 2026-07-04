import { getRequestConfig } from "next-intl/server";
import type { AbstractIntlMessages } from "next-intl";
import { cookies } from "next/headers";

const SUPPORTED_LOCALES = ["th", "en"] as const;
type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];

function isSupportedLocale(value: string): value is SupportedLocale {
  return (SUPPORTED_LOCALES as readonly string[]).includes(value);
}

/**
 * next-intl request config (non-routing approach).
 * Locale is read from the `locale` cookie (set by LocaleSwitcher via server action).
 * Falls back to Thai (`th`) — the system default per UI-SPEC i18n Contract.
 */
export default getRequestConfig(async () => {
  // Next.js 15: cookies() is async
  const cookieStore = await cookies();
  const raw = cookieStore.get("locale")?.value ?? "";
  const locale: SupportedLocale = isSupportedLocale(raw) ? raw : "th";

  const messages = (
    await import(`../messages/${locale}.json`)
  ).default as AbstractIntlMessages;

  return {
    locale,
    messages,
  };
});
