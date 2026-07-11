import { AppShell } from "@/components/AppShell";

/**
 * (app) route group layout — wraps every authenticated back-office page in
 * AppShell (sidebar + header + SignOutButton, role checks inside AppShell).
 *
 * Route groups do not affect URLs: /donations, /e-donation, /reports,
 * /admin keep their exact paths after this move (06-UI-SPEC "Architecture
 * Change Required" step 2). This layout intentionally touches NO CSS
 * tokens — (app) keeps resolving the existing :root-scoped slate/blue
 * variables exactly as before this phase.
 */
export default function AppLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <AppShell>{children}</AppShell>;
}
