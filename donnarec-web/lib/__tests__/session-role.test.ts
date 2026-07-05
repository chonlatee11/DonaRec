import { describe, it, expect } from "vitest";
import { decodeAccessTokenRoles } from "@/lib/session-role";

/**
 * decodeAccessTokenRoles — hermetic unit tests (04-08).
 *
 * Mirrors donnarec-api/internal/auth/claims.go's documented Pitfall 1: Keycloak
 * roles live under realm_access.roles, NEVER a top-level "roles" claim. This is
 * the FE's UX-layer-only nav/route-guard hint for Admin gating (T-04-25) — Go's
 * RequireRoles(RoleAdmin) on adminGroup remains the real authority.
 */

function makeToken(payload: object): string {
  const header = Buffer.from(JSON.stringify({ alg: "none", typ: "JWT" })).toString(
    "base64url"
  );
  const body = Buffer.from(JSON.stringify(payload)).toString("base64url");
  return `${header}.${body}.sig`;
}

describe("decodeAccessTokenRoles", () => {
  it("extracts roles from realm_access.roles", () => {
    const token = makeToken({
      sub: "abc-123",
      realm_access: { roles: ["admin", "checker"] },
    });
    expect(decodeAccessTokenRoles(token)).toEqual(["admin", "checker"]);
  });

  it("NEVER reads a top-level roles claim (Pitfall 1)", () => {
    const token = makeToken({
      roles: ["admin"], // top-level — must be ignored
      realm_access: { roles: ["maker"] },
    });
    expect(decodeAccessTokenRoles(token)).toEqual(["maker"]);
  });

  it("returns an empty array when realm_access is absent", () => {
    const token = makeToken({ sub: "abc-123" });
    expect(decodeAccessTokenRoles(token)).toEqual([]);
  });

  it("returns an empty array for a malformed/non-JWT token instead of throwing", () => {
    expect(decodeAccessTokenRoles("not-a-jwt")).toEqual([]);
    expect(decodeAccessTokenRoles("")).toEqual([]);
    expect(decodeAccessTokenRoles("a.b")).toEqual([]);
  });
});
