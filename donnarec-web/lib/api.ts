import { getServerSession } from "next-auth/next";
import { authOptions } from "./auth";

/**
 * Go API base URL.  Set NEXT_PUBLIC_API_BASE_URL in .env.local.
 * Default falls back to the local dev address of donnarec-api.
 */
const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8000";

// ---------------------------------------------------------------------------
// Error types
// ---------------------------------------------------------------------------

export type ApiErrorType =
  | "forbidden"  // 403 generic
  | "sod"        // 403 Segregation-of-Duties violation
  | "statusConflict" // 409 double-approve / stale state
  | "validation" // 422 field validation error
  | "network";   // other / connectivity

export interface ApiError {
  type: ApiErrorType;
  status: number;
  message: string;
  details?: unknown;
}

export class DonnaRecApiError extends Error {
  constructor(public readonly error: ApiError) {
    super(error.message);
    this.name = "DonnaRecApiError";
  }
}

// ---------------------------------------------------------------------------
// Token helper
// ---------------------------------------------------------------------------

/**
 * Retrieves the Keycloak access token from the NextAuth server session.
 * Returns null if no session is available (unauthenticated or called
 * from a context where cookies/headers are not accessible).
 */
async function getAccessToken(): Promise<string | null> {
  try {
    const session = await getServerSession(authOptions);
    return session?.accessToken ?? null;
  } catch {
    // Not in a server context or no session
    return null;
  }
}

// ---------------------------------------------------------------------------
// Core fetch wrapper
// ---------------------------------------------------------------------------

/**
 * Authenticated fetch wrapper for the Go API (donnarec-api).
 *
 * Attaches `Authorization: Bearer <token>` from the Keycloak session.
 * Maps HTTP error status codes to typed ApiError variants:
 *   403 → "sod" (SOD_VIOLATION code) or "forbidden"
 *   409 → "statusConflict"
 *   422 → "validation"
 *   other → "network"
 *
 * Usage (server component / server action):
 *   const data = await apiFetch<DonationList>("/api/v1/donations");
 */
export async function apiFetch<T>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const token = await getAccessToken();

  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(options.headers as Record<string, string> | undefined),
  };

  if (token) {
    // T-03-05: bearer token sourced from Keycloak OIDC session — never hardcoded
    headers["Authorization"] = `Bearer ${token}`;
  }

  let res: Response;
  try {
    res = await fetch(`${API_BASE_URL}${path}`, {
      ...options,
      headers,
    });
  } catch (err) {
    throw new DonnaRecApiError({
      type: "network",
      status: 0,
      message:
        "บันทึกไม่สำเร็จ — กรุณาตรวจสอบการเชื่อมต่อและลองอีกครั้ง",
      details: err,
    });
  }

  if (!res.ok) {
    let body: Record<string, unknown> = {};
    try {
      body = (await res.json()) as Record<string, unknown>;
    } catch {
      // non-JSON error body — keep empty object
    }

    const apiError: ApiError = (() => {
      switch (res.status) {
        case 403:
          // SoD violation: Go API returns code="SOD_VIOLATION"
          if (body?.code === "SOD_VIOLATION") {
            return {
              type: "sod" as const,
              status: 403,
              message:
                (body.message as string) ??
                "คุณเป็นผู้สร้างรายการนี้ — ผู้อนุมัติต้องเป็นบุคคลอื่น (หลักการแยกหน้าที่)",
              details: body,
            };
          }
          return {
            type: "forbidden" as const,
            status: 403,
            message: (body.message as string) ?? "ไม่มีสิทธิ์ดำเนินการ",
            details: body,
          };

        case 409:
          return {
            type: "statusConflict" as const,
            status: 409,
            message:
              (body.message as string) ??
              "รายการนี้ได้รับการดำเนินการแล้ว — กรุณาโหลดหน้าใหม่เพื่อดูสถานะล่าสุด",
            details: body,
          };

        case 422:
          return {
            type: "validation" as const,
            status: 422,
            message: (body.message as string) ?? "ข้อมูลไม่ถูกต้อง",
            details: body,
          };

        default:
          return {
            type: "network" as const,
            status: res.status,
            message:
              (body.message as string) ??
              "บันทึกไม่สำเร็จ — กรุณาตรวจสอบการเชื่อมต่อและลองอีกครั้ง",
            details: body,
          };
      }
    })();

    throw new DonnaRecApiError(apiError);
  }

  return res.json() as Promise<T>;
}
