import "./public-theme.css";
import { cn } from "@/lib/utils";
import { trirong, ibmPlexSansThai, ibmPlexMono } from "../fonts";

/**
 * (public) route group layout — renders unauthenticated, donor-facing pages
 * WITHOUT AppShell: no sidebar, no SignOutButton, no role checks. Wraps
 * {children} in the .theme-public scoped warm-token layer (see
 * public-theme.css) so shadcn primitives reused on the public form
 * automatically pick up the cream/pine/gold palette.
 *
 * 06-UI-SPEC "Architecture Change Required" step 3: this layout must never
 * read the session or gate on a role — it has to render correctly for a
 * fully unauthenticated, first-time visitor.
 *
 * 06-UI-SPEC "Dual-Theme Architecture" §3: the Trirong/IBM Plex Sans
 * Thai/IBM Plex Mono font instances are declared once in ../fonts.ts
 * (next/font/google requires a static module-scope call) — this is the
 * only place their .variable className is actually applied, so the fonts
 * stay unused/harmless everywhere else (e.g. the root layout / (app)).
 *
 * PublicHeader (built in a later plan of this phase) replaces the minimal
 * placeholder header below once it exists.
 */
export default function PublicLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div
      className={cn(
        "theme-public min-h-screen bg-background text-foreground",
        trirong.variable,
        ibmPlexSansThai.variable,
        ibmPlexMono.variable
      )}
    >
      {/* Placeholder header — replaced by PublicHeader in a later plan */}
      <header className="border-b border-border px-4 py-3">
        <span className="font-semibold text-primary">DonaRec</span>
      </header>
      {children}
    </div>
  );
}
