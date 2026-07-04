import { apiFetch, DonnaRecApiError } from "@/lib/api";

// ---------------------------------------------------------------------------
// Types — mirrors donnarec-api/internal/settings/model.go's ReceiptSettings /
// PreviewRequest JSON contracts exactly (04-07-SUMMARY.md "Next Phase
// Readiness" field-name list).
// ---------------------------------------------------------------------------

export type DeductionMultiplier = "1x" | "2x";
export type NumberFormatYear = "BE4" | "CE4";
export type ImageSlot = "letterhead" | "seal" | "signature" | "watermark";

export interface ReceiptSettings {
  template_html: string;
  template_html_en: string;
  section6_text_th: string;
  section6_text_en: string;
  deduction_multiplier: DeductionMultiplier;
  letterhead_object_key: string | null;
  seal_object_key: string | null;
  signature_object_key: string | null;
  watermark_object_key: string | null;
  updated_at: string;
  updated_by: string;
  separator: string;
  running_no_padding: number;
  year_format: NumberFormatYear;
  prefix: string;
}

/**
 * SettingsFormValues — the mutable subset of ReceiptSettings the admin edits
 * (excludes server-owned audit fields updated_at/updated_by, which the Go
 * service ignores on PUT and always recomputes — model.go's doc comment).
 */
export type SettingsFormValues = Omit<ReceiptSettings, "updated_at" | "updated_by">;

/** PreviewRequest — mirrors settings/model.go's PreviewRequest (never a donation id or donor field, D-61). */
export interface PreviewRequest {
  template_html: string;
  template_html_en: string;
  section6_text_th: string;
  section6_text_en: string;
  deduction_multiplier: string;
  letterhead_object_key: string | null;
  seal_object_key: string | null;
  signature_object_key: string | null;
  watermark_object_key: string | null;
  language: "th" | "en";
}

/** Builds a PreviewRequest from the current in-memory form state (D-61: reflects unsaved edits). */
export function buildPreviewRequest(
  values: SettingsFormValues,
  language: "th" | "en"
): PreviewRequest {
  return {
    template_html: values.template_html,
    template_html_en: values.template_html_en,
    section6_text_th: values.section6_text_th,
    section6_text_en: values.section6_text_en,
    deduction_multiplier: values.deduction_multiplier,
    letterhead_object_key: values.letterhead_object_key,
    seal_object_key: values.seal_object_key,
    signature_object_key: values.signature_object_key,
    watermark_object_key: values.watermark_object_key,
    language,
  };
}

// ---------------------------------------------------------------------------
// Server-side fetcher (Admin-guarded page.tsx seed — direct-to-Go, mirrors
// lib/donations.ts's getDonation server-side pattern)
// ---------------------------------------------------------------------------

/** GET /api/admin/settings — server-side seed for app/admin/settings/page.tsx. */
export async function getSettings(): Promise<ReceiptSettings> {
  return apiFetch<ReceiptSettings>("/api/admin/settings");
}

// ---------------------------------------------------------------------------
// Client-side BFF fetchers (SettingsTabs — TanStack Query / plain mutations)
// ---------------------------------------------------------------------------

async function bffClientFetch<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, {
      ...init,
      headers: {
        Accept: "application/json",
        ...(init?.body ? { "Content-Type": "application/json" } : {}),
        ...(init?.headers as Record<string, string> | undefined),
      },
    });
  } catch (err) {
    throw new DonnaRecApiError({
      type: "network",
      status: 0,
      message: "บันทึกการตั้งค่าไม่สำเร็จ — กรุณาตรวจสอบการเชื่อมต่อและลองอีกครั้ง",
      details: err,
    });
  }

  const text = await res.text();
  let parsed: unknown = null;
  if (text) {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = null;
    }
  }

  if (!res.ok) {
    const body = (parsed ?? {}) as Record<string, unknown>;
    const message =
      res.status === 422
        ? "บันทึกไม่สำเร็จ — เทมเพลตมีข้อผิดพลาด กรุณาตรวจสอบตัวแปร/รูปแบบ HTML"
        : res.status === 403
        ? "ไม่มีสิทธิ์ดำเนินการ"
        : "บันทึกการตั้งค่าไม่สำเร็จ — กรุณาตรวจสอบการเชื่อมต่อและลองอีกครั้ง";
    throw new DonnaRecApiError({
      type: res.status === 422 ? "validation" : res.status === 403 ? "forbidden" : "network",
      status: res.status,
      message: (body.message as string) ?? message,
      details: body,
    });
  }

  const body = parsed as { data?: T } | null;
  return (body?.data ?? (parsed as T)) as T;
}

/** GET /api/bff/settings — client-side fetch for TanStack Query. */
export async function fetchSettingsClient(): Promise<ReceiptSettings> {
  return bffClientFetch<ReceiptSettings>("/api/bff/settings");
}

/** PUT /api/bff/settings — save ALL tabs' values in one request (D-58). */
export async function saveSettings(values: SettingsFormValues): Promise<void> {
  await bffClientFetch<{ saved: boolean }>("/api/bff/settings", {
    method: "PUT",
    body: JSON.stringify(values),
  });
}

/** POST /api/bff/settings/preview — sandboxed HTML preview (D-61 fast path). */
export async function fetchPreviewHTML(payload: PreviewRequest): Promise<string> {
  const result = await bffClientFetch<{ html: string }>("/api/bff/settings/preview", {
    method: "POST",
    body: JSON.stringify(payload),
  });
  return result.html;
}

/** POST /api/bff/settings/preview/pdf — real-PDF preview via the production sandbox pipeline. */
export async function fetchPreviewPDFBlob(payload: PreviewRequest): Promise<Blob> {
  let res: Response;
  try {
    res = await fetch("/api/bff/settings/preview/pdf", {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/pdf" },
      body: JSON.stringify(payload),
    });
  } catch {
    throw new Error("สร้างตัวอย่าง PDF ไม่สำเร็จ — กรุณาลองใหม่ หรือดูตัวอย่างแบบ HTML แทน");
  }
  if (!res.ok) {
    let message = "สร้างตัวอย่าง PDF ไม่สำเร็จ — กรุณาลองใหม่ หรือดูตัวอย่างแบบ HTML แทน";
    try {
      const body = (await res.json()) as { message?: string; error?: string };
      if (body?.message) message = body.message;
    } catch {
      // keep default message
    }
    throw new Error(message);
  }
  return res.blob();
}

/** POST /api/bff/settings/images/:slot — brand-image upload (magic-byte/2MB validated server-side). */
export async function uploadTemplateImage(
  slot: ImageSlot,
  file: File
): Promise<{ slot: string; object_key: string }> {
  const formData = new FormData();
  formData.set("file", file);

  let res: Response;
  try {
    res = await fetch(`/api/bff/settings/images/${slot}`, {
      method: "POST",
      body: formData,
    });
  } catch (err) {
    throw new DonnaRecApiError({
      type: "network",
      status: 0,
      message: "บันทึกการตั้งค่าไม่สำเร็จ — กรุณาตรวจสอบการเชื่อมต่อและลองอีกครั้ง",
      details: err,
    });
  }

  const text = await res.text();
  let parsed: unknown = null;
  if (text) {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = null;
    }
  }

  if (!res.ok) {
    const body = (parsed ?? {}) as Record<string, unknown>;
    throw new DonnaRecApiError({
      type: res.status === 415 || res.status === 413 ? "validation" : "network",
      status: res.status,
      message:
        (body.detail as string) ??
        (body.error as string) ??
        "ไฟล์ไม่รองรับ — รองรับเฉพาะ JPG, PNG ขนาดไม่เกิน 2 MB",
      details: body,
    });
  }

  const body = parsed as { data?: { slot: string; object_key: string } } | null;
  if (!body?.data) {
    throw new Error("อัปโหลดรูปภาพไม่สำเร็จ — กรุณาลองอีกครั้ง");
  }
  return body.data;
}

/** apiErrorMessage — extracts a user-facing Thai message from any error thrown by these calls. */
export function settingsErrorMessage(err: unknown): string {
  if (err instanceof DonnaRecApiError) return err.error.message;
  if (err instanceof Error) return err.message;
  return "เกิดข้อผิดพลาด";
}
