import { DonationDetailView } from "@/components/DonationDetailView";

interface DonationDetailPageProps {
  params: Promise<{ id: string }>;
}

/**
 * Donation detail + review page — Screen 3 (server shell, 03-12).
 *
 * The record fetch, approve/return/reject, and cancel/void-and-reissue
 * mutations all run client-side inside DonationDetailView via
 * useQuery/useMutation against the BFF (D-R1). This shell only supplies the
 * route param — 03-13 moved cancel/reissue off Server Actions here into the
 * same client-mutation pattern as the rest of the review actions.
 */
export default async function DonationDetailPage({
  params,
}: DonationDetailPageProps) {
  const { id } = await params;

  return <DonationDetailView id={id} />;
}
