import { revalidatePath } from "next/cache";
import { cancelDonation, reissueDonation } from "@/lib/donations";
import { DonationDetailView } from "@/components/DonationDetailView";
import type { CancelDonationRequest } from "@/lib/donations";

interface DonationDetailPageProps {
  params: Promise<{ id: string }>;
}

/**
 * Donation detail + review page — Screen 3 (server shell, 03-12).
 *
 * The record fetch and approve/return/reject mutations now run client-side
 * inside DonationDetailView via useQuery/useMutation against the BFF (D-R1).
 * Cancel / void-and-reissue (issued-receipt actions) still run as inline
 * Server Actions here, passed down as props — they migrate to the BFF +
 * TanStack pattern in 03-13 alongside create/edit/slip.
 */
export default async function DonationDetailPage({
  params,
}: DonationDetailPageProps) {
  const { id } = await params;

  /**
   * Cancel (void) an issued receipt.
   * T-03-36: rd_confirmation_reason required when edonation_keyed=true.
   * Server returns 409 ErrEDonationKeyedConfirmation if missing.
   */
  async function handleCancel(
    body: CancelDonationRequest
  ): Promise<{ error?: string } | null> {
    "use server";
    try {
      await cancelDonation(id, body);
      revalidatePath(`/donations/${id}`);
      revalidatePath("/donations");
      return null;
    } catch (err) {
      const e = err as { error?: { message?: string }; message?: string };
      return { error: e?.error?.message ?? e?.message ?? "ยกเลิกไม่สำเร็จ" };
    }
  }

  /**
   * Void & Reissue — cancel original + create replacement draft (D-50).
   * New draft earns receipt number only via normal Submit → Approve path.
   */
  async function handleReissue(
    body: CancelDonationRequest
  ): Promise<{ error?: string; newId?: string } | null> {
    "use server";
    try {
      const result = await reissueDonation(id, body);
      revalidatePath(`/donations/${id}`);
      revalidatePath(`/donations/${result.id}`);
      revalidatePath("/donations");
      return { newId: result.id };
    } catch (err) {
      const e = err as { error?: { message?: string }; message?: string };
      return {
        error: e?.error?.message ?? e?.message ?? "สร้างรายการใหม่ไม่สำเร็จ",
      };
    }
  }

  return (
    <DonationDetailView
      id={id}
      onCancel={handleCancel}
      onReissue={handleReissue}
    />
  );
}
