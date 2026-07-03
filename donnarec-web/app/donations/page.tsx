import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { getServerSession } from "next-auth/next";
import { Plus } from "lucide-react";
import { authOptions } from "@/lib/auth";
import { DonationFilterBar } from "@/components/DonationFilterBar";
import { DonationListView } from "@/components/DonationListView";
import { Button } from "@/components/ui/button";
import type { SearchFilter, DonationStatus } from "@/lib/donations";

interface SearchParams {
  name?: string;
  status?: string;
  from?: string;
  to?: string;
  receipt_no?: string;
  page?: string;
}

interface DonationsPageProps {
  searchParams: Promise<SearchParams>;
}

/**
 * Donations list page — Screen 1.
 * Server Component: reads searchParams, fetches data, passes to client components.
 *
 * D-53: Filter scope is name / date range / status / receipt_no only.
 * national/tax ID search is intentionally absent (no field, not passed to API).
 */
export default async function DonationsPage({
  searchParams,
}: DonationsPageProps) {
  const t = await getTranslations();
  const params = await searchParams;

  // ── Parse filter from URL query string ─────────────────────────────────────

  const rawStatus = params.status && params.status !== "all" ? params.status : "";
  const filter: SearchFilter = {
    name: params.name,
    status: rawStatus as DonationStatus | "",
    from: params.from,
    to: params.to,
    receipt_no: params.receipt_no,
    page: params.page ? parseInt(params.page, 10) : 1,
  };

  // ── Extract viewer ID from Keycloak access token (for row routing) ─────────
  // Needed to route Maker's own drafts to the edit form (UI-SPEC Screen 1 row rules).
  // We decode the JWT payload without a library — `sub` claim is the Keycloak UUID.

  let viewerId: string | undefined;
  try {
    const session = await getServerSession(authOptions);
    const accessToken = session?.accessToken;
    if (accessToken) {
      const [, payloadB64] = accessToken.split(".");
      if (payloadB64) {
        const json = Buffer.from(payloadB64, "base64url").toString("utf8");
        const payload = JSON.parse(json) as { sub?: string };
        viewerId = payload.sub;
      }
    }
  } catch {
    // Token not available or malformed — proceed without viewer-based routing
  }

  // ── Render ─────────────────────────────────────────────────────────────────
  // D-R1: the list is fetched CLIENT-side through TanStack Query + the BFF route
  // (DonationListView). The server no longer fetches here — this removes the old
  // `result.donations` crash site (bug #5).

  return (
    <div className="flex flex-col gap-6">
      {/* Page heading + CTA */}
      <div className="flex items-center justify-between">
        <h1 className="text-[28px] font-semibold leading-tight text-slate-900">
          {t("donations.title")}
        </h1>
        {/*
         * "สร้างรายการบริจาค" CTA (accent, Plus icon).
         * UI-SPEC: visible to Maker and Admin only.
         * Role gating is authoritative on the server (03-03/03-05).
         * Here the button is always rendered; the create route will redirect
         * unauthorized users. Role-based hiding can be added once roles are
         * available from the session (03-08 scope).
         */}
        <Button
          asChild
          className="min-h-[44px] bg-blue-600 text-white hover:bg-blue-700"
        >
          <Link href="/donations/new">
            <Plus className="mr-2 h-4 w-4" />
            {t("actions.create")}
          </Link>
        </Button>
      </div>

      {/* Filter bar (Client Component) */}
      <DonationFilterBar currentFilter={filter} />

      {/*
       * List view (Client Component): TanStack Query drives the fetch through
       * the same-origin BFF route; handles loading skeleton + error alert.
       */}
      <DonationListView filter={filter} viewerId={viewerId} />
    </div>
  );
}
