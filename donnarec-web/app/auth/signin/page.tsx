"use client";

import { useEffect } from "react";
import { signIn } from "next-auth/react";

/**
 * Sign-in entry point (configured via `authOptions.pages.signIn`).
 *
 * DonaRec has a single auth provider (Keycloak), so instead of showing
 * NextAuth's generic provider-picker page, this immediately kicks off the
 * Keycloak OAuth (auth-code + PKCE) redirect — one step for the user.
 *
 * `callbackUrl` is read from the query string (set by middleware.ts when it
 * redirects an unauthenticated request here) so the user lands back on the
 * page they originally requested after logging in. Read via
 * `window.location.search` inside the effect (client-only) rather than
 * `useSearchParams()` to avoid the Suspense-boundary requirement for a page
 * this simple.
 */
export default function SignInPage() {
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const callbackUrl = params.get("callbackUrl") || "/donations";
    signIn("keycloak", { callbackUrl });
  }, []);

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-2">
      <p className="text-body text-slate-600">
        กำลังนำท่านไปยังหน้าเข้าสู่ระบบ…
      </p>
    </main>
  );
}
