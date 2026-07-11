import type { Metadata } from "next";
import { Sarabun, Inter } from "next/font/google";
import { NextIntlClientProvider } from "next-intl";
import { getLocale, getMessages } from "next-intl/server";
import { AuthSessionProvider } from "@/components/AuthSessionProvider";
import { Providers } from "./providers";
import "./globals.css";

/**
 * UI-SPEC Typography:
 * Thai text → Sarabun (closest web-safe equivalent of TH Sarabun New used in PDF/receipts)
 * Latin text → Inter (clean back-office sans-serif)
 * Font-family stack: 'Sarabun', 'Inter', system-ui, sans-serif (defined in globals.css body)
 *
 * 06-UI-SPEC "Architecture Change Required": root layout stays theme-neutral
 * — html/body + font variable classes + providers only. The (app) route
 * group renders AppShell (slate/blue, unchanged); the (public) route group
 * renders its own warm-themed wrapper. No AppShell here.
 */
const sarabun = Sarabun({
  subsets: ["thai", "latin"],
  weight: ["400", "600"],
  variable: "--font-sarabun",
  display: "swap",
});

const inter = Inter({
  subsets: ["latin"],
  weight: ["400", "600"],
  variable: "--font-inter",
  display: "swap",
});

export const metadata: Metadata = {
  title: "DonaRec — ระบบออกใบเสร็จบริจาค",
  description: "ระบบออกใบเสร็จรับเงินบริจาคสำหรับโรงพยาบาล",
};

export default async function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  // next-intl: read locale from cookie (set by LocaleSwitcher → setLocale server action)
  const locale = await getLocale();
  const messages = await getMessages();

  return (
    <html
      lang={locale}
      className={`${sarabun.variable} ${inter.variable}`}
    >
      <body>
        {/*
         * NextIntlClientProvider makes translations available in Client Components.
         * Server Components use getTranslations() directly.
         */}
        <NextIntlClientProvider locale={locale} messages={messages}>
          <AuthSessionProvider>
            {/*
             * Providers: mounts the TanStack Query QueryClientProvider (D-R1)
             * so client components under AppShell/(public) can drive data
             * through the same-origin BFF routes.
             */}
            <Providers>{children}</Providers>
          </AuthSessionProvider>
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
