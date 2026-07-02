import type { Metadata } from "next";
import { Sarabun, Inter } from "next/font/google";
import { NextIntlClientProvider } from "next-intl";
import { getLocale, getMessages } from "next-intl/server";
import { AppShell } from "@/components/AppShell";
import { AuthSessionProvider } from "@/components/AuthSessionProvider";
import "./globals.css";

/**
 * UI-SPEC Typography:
 * Thai text → Sarabun (closest web-safe equivalent of TH Sarabun New used in PDF/receipts)
 * Latin text → Inter (clean back-office sans-serif)
 * Font-family stack: 'Sarabun', 'Inter', system-ui, sans-serif (defined in globals.css body)
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
            {/* AppShell: slate-100 sidebar + slate-50 content + header with LocaleSwitcher */}
            <AppShell>{children}</AppShell>
          </AuthSessionProvider>
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
