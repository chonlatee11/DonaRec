import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ExportPanel } from "@/components/ExportPanel";
import { isCheckerOrAdminViewer } from "@/lib/session-role";

/**
 * e-Donation Export & Keyed-Status Tracking page — Screen 7 (FR-30/FR-31,
 * D-63, plan 05-06).
 *
 * Server Component: guards the route to Checker/Admin (redirects otherwise),
 * then renders the two-tab layout — mirrors app/admin/settings/page.tsx's
 * server-wrapper + client-fetch split.
 *
 * T-05-06-UXGATE: this redirect is a UX convenience only — Go's
 * edonationGroup.Use(RequireAnyRole(RoleChecker, RoleAdmin)) (05-02) is the
 * real authorization authority; every BFF call ExportPanel/AgingTable makes
 * is independently re-checked server-side regardless of what this guard
 * decides.
 */
export default async function EdonationPage() {
  const allowed = await isCheckerOrAdminViewer();
  if (!allowed) {
    redirect("/donations");
  }

  const t = await getTranslations("eDonationExport");

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-[28px] font-semibold leading-tight text-slate-900">
        {t("pageTitle")}
      </h1>

      <Tabs defaultValue="export">
        <TabsList className="mb-4">
          <TabsTrigger value="export">{t("tabExport")}</TabsTrigger>
          <TabsTrigger value="aging">{t("tabAging")}</TabsTrigger>
        </TabsList>

        <TabsContent value="export">
          <ExportPanel />
        </TabsContent>

        <TabsContent value="aging">
          {/* Wired to AgingTable in plan 05-06 Task 3 */}
        </TabsContent>
      </Tabs>
    </div>
  );
}
