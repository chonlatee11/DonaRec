import { getServerSession } from "next-auth/next";
import { authOptions } from "./auth";

/**
 * decodeAccessTokenRoles — extracts the Keycloak `realm_access.roles` claim from
 * an access token WITHOUT verifying its signature.
 *
 * This mirrors donnarec-api/internal/auth/claims.go's documented Pitfall 1: roles
 * live under `realm_access.roles`, NEVER a top-level "roles" claim.
 *
 * UX-layer only (T-04-25): this drives nav-link visibility (AppShell) and a
 * server-component route redirect (app/admin/settings/page.tsx) — the real
 * authorization authority is Go's `adminGroup.Use(RequireRoles(RoleAdmin))`
 * (04-07), which independently re-verifies the token's signature and re-checks
 * the role on every request. A forged/stale client-side hint here can, at worst,
 * show a nav link that 403s on the next request — it can never grant access.
 */
export function decodeAccessTokenRoles(accessToken: string): string[] {
  try {
    const parts = accessToken.split(".");
    if (parts.length < 2 || !parts[1]) return [];
    const json = Buffer.from(parts[1], "base64url").toString("utf8");
    const payload = JSON.parse(json) as {
      realm_access?: { roles?: string[] };
    };
    return payload.realm_access?.roles ?? [];
  } catch {
    return [];
  }
}

/**
 * getViewerRoles — server-side helper resolving the CURRENT session's Keycloak
 * realm roles. Returns an empty array when there is no session/access token.
 */
export async function getViewerRoles(): Promise<string[]> {
  const session = await getServerSession(authOptions);
  const accessToken = session?.accessToken;
  if (!accessToken) return [];
  return decodeAccessTokenRoles(accessToken);
}

/**
 * isAdminViewer — true when the current session's realm_access.roles includes
 * "admin" (donnarec-api/internal/auth/rbac.go's RoleAdmin = "admin").
 */
export async function isAdminViewer(): Promise<boolean> {
  const roles = await getViewerRoles();
  return roles.includes("admin");
}

/**
 * isCheckerOrAdminViewer — true when the current session's realm_access.roles
 * includes "checker" OR "admin" (parallel structure to isAdminViewer).
 *
 * UX-layer only (T-05-06-UXGATE), same discipline as isAdminViewer's doc
 * comment: drives the `/e-donation` nav-link visibility and the Screen 7
 * server-component route redirect. The real authorization authority is Go's
 * `edonationGroup.Use(RequireAnyRole(RoleChecker, RoleAdmin))` (05-02), which
 * independently re-verifies the token's signature and re-checks the role on
 * every request — a forged/stale client-side hint here can, at worst, show a
 * nav link that 403s on the next request.
 */
export async function isCheckerOrAdminViewer(): Promise<boolean> {
  const roles = await getViewerRoles();
  return roles.includes("checker") || roles.includes("admin");
}
