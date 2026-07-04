import type { NextAuthOptions, Session } from "next-auth";
import type { JWT } from "next-auth/jwt";
import KeycloakProvider from "next-auth/providers/keycloak";

/**
 * NextAuth configuration with Keycloak OIDC provider.
 * Extracts the Keycloak access_token and stores it in the session
 * so lib/api.ts can attach it as Authorization: Bearer <token>.
 *
 * Required env vars (do NOT hardcode):
 *   KEYCLOAK_CLIENT_ID     - Keycloak client ID (e.g. "donnarec-web")
 *   KEYCLOAK_CLIENT_SECRET - Keycloak client secret
 *   KEYCLOAK_ISSUER        - Keycloak realm URL (e.g. http://localhost:8080/realms/donnarec)
 *   NEXTAUTH_SECRET        - Random secret for session encryption
 *   NEXTAUTH_URL           - App URL (e.g. http://localhost:3000)
 */
export const authOptions: NextAuthOptions = {
  providers: [
    KeycloakProvider({
      clientId: process.env.KEYCLOAK_CLIENT_ID!,
      clientSecret: process.env.KEYCLOAK_CLIENT_SECRET!,
      issuer: process.env.KEYCLOAK_ISSUER!,
    }),
  ],

  callbacks: {
    /**
     * Persist the Keycloak access_token in the JWT token on sign-in.
     * On subsequent requests the token is refreshed when expired.
     */
    async jwt({ token, account }: { token: JWT; account: unknown }) {
      const acc = account as {
        access_token?: string;
        refresh_token?: string;
        id_token?: string;
        expires_at?: number;
      } | null;

      if (acc) {
        // Initial sign-in: store tokens from Keycloak.
        // idToken is kept for RP-initiated (federated) logout — it is the
        // `id_token_hint` Keycloak needs to end the SSO session on sign-out.
        return {
          ...token,
          accessToken: acc.access_token,
          refreshToken: acc.refresh_token,
          idToken: acc.id_token,
          expiresAt: acc.expires_at,
        };
      }

      // Token still valid
      const expiresAt = token.expiresAt as number | undefined;
      if (expiresAt && Date.now() < expiresAt * 1000) {
        return token;
      }

      // Token expired — mark error; refresh logic can be added here
      return { ...token, error: "RefreshAccessTokenError" };
    },

    /**
     * Expose the access_token on the client-visible Session object.
     * The token is used by lib/api.ts to call the Go API.
     */
    async session({
      session,
      token,
    }: {
      session: Session;
      token: JWT;
    }): Promise<Session> {
      return {
        ...session,
        accessToken: token.accessToken as string | undefined,
        error: token.error as string | undefined,
      };
    },
  },

  events: {
    /**
     * RP-initiated (federated) logout — Phase 3 UAT bug fix.
     *
     * NextAuth's signOut() only clears the local app session cookie; the Keycloak
     * SSO session stays alive, so the next visit to /auth/signin silently
     * re-authenticates the SAME user (you can never switch Maker↔Checker, and
     * "ออกจากระบบ" appears to do nothing). Here we end the Keycloak session
     * server-side via the OIDC end_session endpoint using the stored id_token as
     * the id_token_hint. Best-effort: the local session is already cleared, so a
     * Keycloak hiccup must not block sign-out.
     */
    async signOut({ token }: { token: JWT }): Promise<void> {
      const idToken = (token as JWT & { idToken?: string }).idToken;
      const issuer = process.env.KEYCLOAK_ISSUER;
      if (!idToken || !issuer) return;
      const endSession =
        `${issuer}/protocol/openid-connect/logout` +
        `?id_token_hint=${encodeURIComponent(idToken)}`;
      try {
        await fetch(endSession, { method: "GET" });
      } catch {
        // Local session already cleared by NextAuth — don't block logout.
      }
    },
  },

  pages: {
    signIn: "/auth/signin",
  },
};
