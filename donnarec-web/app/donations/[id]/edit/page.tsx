import { notFound } from "next/navigation";
import { getDonation } from "@/lib/donations";
import { DonnaRecApiError } from "@/lib/api";
import { DonationForm } from "@/components/DonationForm";

interface EditDonationPageProps {
  params: Promise<{ id: string }>;
}

/**
 * Edit donation page — Screen 2 (edit mode).
 * Server Component: fetches draft data server-side, then hands off to the
 * client DonationForm which owns its own create/update/submit/slip mutations
 * via useMutation against the BFF (D-R1, 03-13) — no Server Action props.
 *
 * FR-09: Maker edits an existing draft (must be in "draft" status).
 * Server enforces status guard — ErrInvalidTransition → 409 if not draft.
 *
 * The amber return-from-checker alert is shown when review_history contains a
 * "return" entry, extracted from the donation's review_history in DonationForm.
 */
export default async function EditDonationPage({
  params,
}: EditDonationPageProps) {
  const { id } = await params;

  // ── Fetch existing donation ───────────────────────────────────────────────

  let donation: Awaited<ReturnType<typeof getDonation>>;
  try {
    donation = await getDonation(id);
  } catch (err) {
    if (err instanceof DonnaRecApiError && err.error.status === 404) {
      notFound();
    }
    throw err;
  }

  // Only draft records are editable; redirect to detail for other statuses
  // (server also enforces this — redirect is UX-only)
  if (donation.status !== "draft") {
    // Re-use notFound to avoid exposing internal redirect logic;
    // the detail page handles non-draft statuses correctly.
    notFound();
  }

  // ── Render ────────────────────────────────────────────────────────────────

  return (
    <div className="mx-auto max-w-[680px]">
      <DonationForm
        mode="edit"
        donationId={id}
        initialData={{
          donor_name: donation.donor_name,
          address: donation.address,
          email: donation.email,
          // D-R2/03-09: DonationDetail.amount is now a numeric string; the form
          // model expects a number.
          amount: Number(donation.amount),
          donated_at: donation.donated_at,
          note: donation.note,
          slip_url: donation.slip_url,
          review_history: donation.review_history,
          donor_language: donation.donor_language,
        }}
      />
    </div>
  );
}
