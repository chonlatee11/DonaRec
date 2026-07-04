import { redirect } from "next/navigation";
import { SettingsTabs } from "@/components/SettingsTabs";
import { isAdminViewer } from "@/lib/session-role";

/**
 * Admin Settings page — Screen 6 (D-58/D-59/D-61, NFR-09, plan 04-08).
 *
 * Server Component: guards the route to Admin (redirects otherwise), then
 * renders the SettingsTabs client component, which fetches the seed itself
 * via TanStack Query against the BFF (mirrors app/donations/page.tsx's
 * server-wrapper + client-fetch split).
 *
 * T-04-25: this redirect is a UX convenience only — Go's
 * adminGroup.Use(RequireRoles(RoleAdmin)) (04-07) is the real authorization
 * authority; every BFF call SettingsTabs makes is independently re-checked
 * server-side regardless of what this guard decides.
 */
export default async function AdminSettingsPage() {
  const isAdmin = await isAdminViewer();
  if (!isAdmin) {
    redirect("/donations");
  }

  return (
    <div className="flex flex-col gap-6">
      <SettingsTabs />
    </div>
  );
}
