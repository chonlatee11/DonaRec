"use client";

import { SessionProvider } from "next-auth/react";

/**
 * Client-side wrapper around NextAuth's SessionProvider.
 * Required so client components (e.g. SignOutButton) can call
 * useSession()/signOut() without each one re-fetching /api/auth/session.
 */
export function AuthSessionProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  return <SessionProvider>{children}</SessionProvider>;
}
