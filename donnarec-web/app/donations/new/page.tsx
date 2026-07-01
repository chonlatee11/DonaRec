import { revalidatePath } from "next/cache";
import { createDonation, submitDonation, uploadSlip } from "@/lib/donations";
import { DonationForm } from "@/components/DonationForm";
import type { CreateDonationRequest, UpdateDraftRequest } from "@/lib/donations";

/**
 * Create donation page — Screen 2 (create mode).
 * Server Component: defines server actions and passes them to DonationForm.
 *
 * FR-07: Maker creates a new donation draft.
 * D-44: national_id mandatory — enforced by Go service (not at frontend only).
 * D-49: consent snapshot captured on create (consent_given + version).
 */
export default function NewDonationPage() {
  // ── Server actions ──────────────────────────────────────────────────────

  async function handleSaveDraft(
    data: CreateDonationRequest | UpdateDraftRequest
  ): Promise<{ id?: string; error?: string } | null> {
    "use server";
    try {
      // In create mode, data always conforms to CreateDonationRequest
      const result = await createDonation(data as CreateDonationRequest);
      revalidatePath("/donations");
      return { id: result.id };
    } catch (err) {
      const e = err as { error?: { message?: string }; message?: string };
      return { error: e?.error?.message ?? e?.message ?? "บันทึกไม่สำเร็จ" };
    }
  }

  async function handleSubmitForReview(
    id: string
  ): Promise<{ error?: string } | null> {
    "use server";
    try {
      await submitDonation(id);
      revalidatePath(`/donations/${id}`);
      revalidatePath("/donations");
      return null;
    } catch (err) {
      const e = err as { error?: { message?: string }; message?: string };
      return { error: e?.error?.message ?? e?.message ?? "ส่งรอตรวจสอบไม่สำเร็จ" };
    }
  }

  async function handleUploadSlip(
    id: string,
    formData: FormData
  ): Promise<{ error?: string } | null> {
    "use server";
    try {
      await uploadSlip(id, formData);
      revalidatePath(`/donations/${id}`);
      return null;
    } catch (err) {
      const e = err as { error?: { message?: string }; message?: string };
      return { error: e?.error?.message ?? e?.message ?? "อัปโหลดไฟล์ไม่สำเร็จ" };
    }
  }

  // ── Render ──────────────────────────────────────────────────────────────

  return (
    <div className="mx-auto max-w-[680px]">
      <DonationForm
        mode="create"
        onSaveDraft={handleSaveDraft}
        onSubmitForReview={handleSubmitForReview}
        onUploadSlip={handleUploadSlip}
      />
    </div>
  );
}
