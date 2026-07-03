import { getServerSession } from "next-auth/next";
import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";
import { authOptions } from "./auth";

/**
 * Go API base URL (server-side). Same source as lib/api.ts.
 * Set NEXT_PUBLIC_API_BASE_URL in the environment; defaults to local dev.
 */
const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8000";

/**
 * bffForward — server-side proxy helper for the BFF Route Handlers.
 *
 * D-R1: TanStack Query (client) calls Next.js Route Handlers under
 * `app/api/bff/**`. Those handlers run ON THE SERVER, obtain the Keycloak
 * access token via getServerSession(authOptions), and forward it as a Bearer
 * to the Go API. The access token therefore NEVER reaches the browser — the
 * client only ever talks to the same-origin BFF route.
 *
 * Behaviour:
 *   - No server session / no access token → 401 JSON (client is unauthenticated).
 *   - Otherwise fetch `${API_BASE_URL}${goPath}` with Authorization: Bearer,
 *     forwarding the incoming method/body, and pass the Go response through
 *     unchanged (status + parsed JSON body).
 *
 * The Go API re-verifies the Bearer (RequireAuth) — the BFF is a proxy, not the
 * authorization authority (threat T-10-02).
 */
export async function bffForward(
  request: NextRequest,
  goPath: string
): Promise<NextResponse> {
  const session = await getServerSession(authOptions);
  const accessToken = session?.accessToken;

  if (!accessToken) {
    return NextResponse.json(
      { error: "unauthenticated", message: "ไม่พบเซสชัน — กรุณาเข้าสู่ระบบใหม่" },
      { status: 401 }
    );
  }

  const method = request.method ?? "GET";

  const headers: Record<string, string> = {
    // T-10-01: token added server-side only; it is never serialized back to the
    // browser response body.
    Authorization: `Bearer ${accessToken}`,
  };

  // Forward a body only for methods that carry one.
  let body: string | undefined;
  if (method !== "GET" && method !== "HEAD") {
    body = await request.text();
    if (body) {
      headers["Content-Type"] =
        request.headers.get("content-type") ?? "application/json";
    }
  }

  let goRes: Response;
  try {
    goRes = await fetch(`${API_BASE_URL}${goPath}`, {
      method,
      headers,
      body,
      cache: "no-store",
    });
  } catch {
    return NextResponse.json(
      {
        error: "network",
        message:
          "ไม่สามารถเชื่อมต่อระบบได้ — กรุณาตรวจสอบการเชื่อมต่อและลองอีกครั้ง",
      },
      { status: 502 }
    );
  }

  // Pass the Go response through: preserve status + parsed JSON body.
  if (goRes.status === 204) {
    return new NextResponse(null, { status: 204 });
  }

  let payload: unknown = null;
  const text = await goRes.text();
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      // Non-JSON body from Go — wrap it so the client still gets structured JSON.
      payload = { message: text };
    }
  }

  return NextResponse.json(payload, { status: goRes.status });
}
