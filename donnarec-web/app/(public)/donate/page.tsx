import { PublicDonationForm } from "@/components/PublicDonationForm";

/**
 * /donate — the public, unauthenticated donation form (Screen 9, Flow B).
 *
 * Lives in the (public) route group, so it is already wrapped by
 * app/(public)/layout.tsx's PublicHeader + .theme-public warm-token scope +
 * public fonts (plan 06-05). middleware.ts deliberately does NOT match /donate,
 * so it stays reachable without a session (D-78).
 */
export default function DonatePage() {
  return <PublicDonationForm />;
}
