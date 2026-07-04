import { DonationForm } from "@/components/DonationForm";

/**
 * Create donation page — Screen 2 (create mode).
 *
 * 03-13: DonationForm now owns its own create/update/submit/slip mutations
 * via useMutation against the BFF (D-R1) — this page no longer needs to
 * define Server Actions, it just renders the client form.
 *
 * FR-07: Maker creates a new donation draft.
 * D-44: national_id mandatory — enforced by the Go service (not FE-only).
 * D-49: consent snapshot captured on create (consent_given + version).
 */
export default function NewDonationPage() {
  return (
    <div className="mx-auto max-w-[680px]">
      <DonationForm mode="create" />
    </div>
  );
}
