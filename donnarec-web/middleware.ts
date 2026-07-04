import { withAuth } from "next-auth/middleware";

/**
 * Edge middleware — forces authentication on all back-office routes.
 *
 * Unauthenticated requests to a matched path are redirected to
 * `pages.signIn` ("/auth/signin", see lib/auth.ts) with a `callbackUrl`
 * query param pointing back at the originally requested URL.
 *
 * Uses `NEXTAUTH_SECRET` from env (read automatically by withAuth/getToken)
 * to verify the session JWT — no network round-trip to Keycloak per request.
 */
export default withAuth({
  pages: {
    signIn: "/auth/signin",
  },
});

export const config = {
  // Protect the back-office app: root (redirects to /donations) and all
  // donation/queue routes + their children. /api/auth/* and /auth/signin
  // itself are intentionally NOT matched (must stay reachable while
  // unauthenticated).
  matcher: ["/", "/donations/:path*", "/queue/:path*"],
};
