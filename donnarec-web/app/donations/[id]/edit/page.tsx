import { notFound } from "next/navigation";
import { revalidatePath } from "next/cache";
import {
  getDonation,
  updateDraft,
  submitDonation,
  uploadSlip,
  removeSlip,
} from "@/lib/donations";
import { DonnaRecApiError } from "@/lib/api";
import { DonationForm } from "@/components/DonationForm";
import type { UpdateDraftRequest } from "@/lib/donations";

interface EditDonationPageProps {
  params: Promise<{ id: string }>;
}

/**
 * Edit donation page — Screen 2 (edit mode).
 * Server Component: fetches draft data, defines server actions, passes to DonationForm.
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

  // ── Server actions ────────────────────────────────────────────────────────

  async function handleSaveDraft(
    data: UpdateDraftRequest
  ): Promise<{ id?: string; error?: string } | null> {
    "use server";
    try {
      await updateDraft(id, data);
      revalidatePath(`/donations/${id}`);
      revalidatePath("/donations");
      return { id };
    } catch (err) {
      const e = err as { error?: { message?: string }; message?: string };
      return { error: e?.error?.message ?? e?.message ?? "บันทึกไม่สำเร็จ" };
    }
  }

  async function handleSubmitForReview(
    donationId: string
  ): Promise<{ error?: string } | null> {
    "use server";
    try {
      await submitDonation(donationId);
      revalidatePath(`/donations/${donationId}`);
      revalidatePath("/donations");
      return null;
    } catch (err) {
      const e = err as { error?: { message?: string }; message?: string };
      return { error: e?.error?.message ?? e?.message ?? "ส่งรอตรวจสอบไม่สำเร็จ" };
    }
  }

  async function handleUploadSlip(
    donationId: string,
    formData: FormData
  ): Promise<{ error?: string } | null> {
    "use server";
    try {
      await uploadSlip(donationId, formData);
      revalidatePath(`/donations/${donationId}`);
      return null;
    } catch (err) {
      const e = err as { error?: { message?: string }; message?: string };
      return { error: e?.error?.message ?? e?.message ?? "อัปโหลดไฟล์ไม่สำเร็จ" };
    }
  }

  async function handleRemoveSlip(
    donationId: string
  ): Promise<{ error?: string } | null> {
    "use server";
    try {
      await removeSlip(donationId);
      revalidatePath(`/donations/${donationId}`);
      return null;
    } catch (err) {
      const e = err as { error?: { message?: string }; message?: string };
      return { error: e?.error?.message ?? e?.message ?? "ลบสลิปไม่สำเร็จ" };
    }
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
          amount: donation.amount,
          donated_at: donation.donated_at,
          note: donation.note,
          slip_url: donation.slip_url,
          review_history: donation.review_history,
        }}
        onSaveDraft={handleSaveDraft}
        onSubmitForReview={handleSubmitForReview}
        onUploadSlip={handleUploadSlip}
        onRemoveSlip={handleRemoveSlip}
      />
    </div>
  );
}
