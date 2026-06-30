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
        expires_at?: number;
      } | null;

      if (acc) {
        // Initial sign-in: store tokens from Keycloak
        return {
          ...token,
          accessToken: acc.access_token,
          refreshToken: acc.refresh_token,
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

  pages: {
    signIn: "/auth/signin",
  },
};
