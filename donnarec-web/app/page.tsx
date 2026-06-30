import { getTranslations } from "next-intl/server";

/**
 * Root placeholder page.
 * Later phases (03-07 / 03-08) will replace this with the donation list.
 */
export default async function Home() {
  const t = await getTranslations("app");

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-2">
      <h1 className="text-page-heading text-slate-900">{t("title")}</h1>
      <p className="text-body text-slate-600">{t("subtitle")}</p>
    </main>
  );
}
