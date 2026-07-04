"use client";

import { signOut, useSession } from "next-auth/react";
import { cn } from "@/lib/utils";

/**
 * Sign-out control shown in the AppShell header when a session exists.
 * Renders nothing while unauthenticated / session still loading.
 */
export function SignOutButton({ className }: { className?: string }) {
  const { data: session, status } = useSession();

  if (status !== "authenticated" || !session) return null;

  return (
    <button
      type="button"
      onClick={() => signOut({ callbackUrl: "/auth/signin" })}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-md px-3 py-1.5",
        "text-sm font-medium text-slate-600",
        "border border-slate-200 bg-white",
        "hover:bg-slate-50 hover:text-slate-900",
        "focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-600 focus-visible:ring-offset-2",
        "transition-colors",
        // UI-SPEC: 44px min touch target height
        "min-h-[44px]",
        className
      )}
    >
      {session.user?.name ?? session.user?.email ?? "ออกจากระบบ"}
      <span aria-hidden="true">·</span>
      <span>ออกจากระบบ</span>
    </button>
  );
}
