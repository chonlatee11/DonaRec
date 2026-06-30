"use server";

import { cookies } from "next/headers";

const SUPPORTED_LOCALES = ["th", "en"] as const;
type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];

function isSupportedLocale(value: string): value is SupportedLocale {
  return (SUPPORTED_LOCALES as readonly string[]).includes(value);
}

/**
 * Server action: persist locale preference in a cookie.
 * Called by LocaleSwitcher (client component) to toggle th/en.
 * After this, the caller calls router.refresh() to re-render with the new locale.
 */
export async function setLocale(locale: string): Promise<void> {
  if (!isSupportedLocale(locale)) return;
  const cookieStore = await cookies();
  cookieStore.set("locale", locale, {
    path: "/",
    maxAge: 60 * 60 * 24 * 365, // 1 year
    sameSite: "lax",
    httpOnly: false, // readable by client for display
  });
}
