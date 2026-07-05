"use client";

import { useRef, useState, useCallback } from "react";
import { useTranslations } from "next-intl";
import { ImageIcon, Loader2, UploadCloud, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { ImageSlot } from "@/lib/settings";

/** 2 MB — client-side UX pre-check (server magic-byte + size cap is the authority, 04-07). */
const MAX_IMAGE_BYTES = 2 * 1024 * 1024;
const ALLOWED_TYPES = ["image/jpeg", "image/png"];

interface ImageUploadSlotProps {
  slot: ImageSlot;
  label: string;
  ariaLabel: string;
  /**
   * True when this slot currently has an object key — either from the
   * last-saved config (GET /settings) or a fresh upload this session.
   *
   * NOTE: 04-07 built no GET-by-key/presigned-view endpoint for template
   * images (only upload), so a previously-saved image cannot be rendered as
   * an actual pixel thumbnail here — only as a "has image" populated state.
   * A freshly uploaded/selected file in THIS session IS rendered as a real
   * thumbnail via localPreviewUrl (object URL), since the browser already
   * holds those bytes.
   */
  hasImage: boolean;
  /** Object URL (from URL.createObjectURL) for a file selected/uploaded this session, or null. */
  localPreviewUrl: string | null;
  uploading?: boolean;
  error?: string | null;
  disabled?: boolean;
  onSelectFile: (file: File) => void;
  onRemove: () => void;
}

/**
 * ImageUploadSlot — 96x96 thumbnail upload tile for a single brand asset
 * (letterhead/seal/signature/watermark). UI-SPEC Screen 6 Tab 2: reuses
 * SlipUploadZone's drag-drop/magic-byte-error visual language at thumbnail
 * scale (Phase 3 precedent).
 */
export function ImageUploadSlot({
  slot,
  label,
  ariaLabel,
  hasImage,
  localPreviewUrl,
  uploading = false,
  error,
  disabled = false,
  onSelectFile,
  onRemove,
}: ImageUploadSlotProps) {
  const t = useTranslations("settings.images");
  const inputRef = useRef<HTMLInputElement>(null);
  const [clientError, setClientError] = useState<string | null>(null);
  const [isDragOver, setIsDragOver] = useState(false);

  const displayError = clientError ?? error ?? null;

  const handleFile = useCallback(
    (file: File) => {
      setClientError(null);
      if (file.size > MAX_IMAGE_BYTES) {
        setClientError(t("rejected"));
        return;
      }
      if (!ALLOWED_TYPES.includes(file.type)) {
        setClientError(t("rejected"));
        return;
      }
      onSelectFile(file);
    },
    [onSelectFile, t]
  );

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) handleFile(file);
    e.target.value = "";
  };

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (!disabled) setIsDragOver(true);
  };
  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragOver(false);
  };
  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragOver(false);
    if (disabled) return;
    const file = e.dataTransfer.files?.[0];
    if (file) handleFile(file);
  };

  const populated = hasImage || !!localPreviewUrl;

  return (
    <div className="flex flex-col items-center gap-2" data-slot={slot}>
      <div
        role="button"
        tabIndex={disabled || uploading ? -1 : 0}
        aria-label={ariaLabel}
        aria-disabled={disabled || uploading}
        // FI-01: gate on `uploading` too, so a second file can't be selected
        // for this slot mid-flight (which would start a racing upload whose
        // last-wins resolution + early `finally` clears uploadingSlot). Mirrors
        // BW-03 on the server side.
        onClick={() => !disabled && !uploading && inputRef.current?.click()}
        onKeyDown={(e) => {
          if (!disabled && !uploading && (e.key === "Enter" || e.key === " ")) {
            e.preventDefault();
            inputRef.current?.click();
          }
        }}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        className={[
          "flex h-24 w-24 cursor-pointer flex-col items-center justify-center gap-1 overflow-hidden",
          "rounded-md border-2 border-dashed transition-colors",
          disabled
            ? "cursor-not-allowed border-slate-200 bg-slate-50"
            : isDragOver
            ? "border-blue-400 bg-blue-50"
            : "border-slate-300 bg-white hover:border-blue-400 hover:bg-blue-50/30",
        ].join(" ")}
      >
        {uploading ? (
          <Loader2 className="h-5 w-5 animate-spin text-slate-400" />
        ) : localPreviewUrl ? (
          // eslint-disable-next-line @next/next/no-img-element -- local blob: URL, next/image cannot optimize it
          <img
            src={localPreviewUrl}
            alt={label}
            className="h-full w-full object-contain"
          />
        ) : populated ? (
          <>
            <ImageIcon className="h-6 w-6 text-slate-400" />
            <span className="text-[11px] text-slate-500">{t("hasImage")}</span>
          </>
        ) : (
          <>
            <UploadCloud
              className={["h-5 w-5", isDragOver ? "text-blue-500" : "text-slate-400"].join(" ")}
            />
            <span className="text-[11px] text-slate-400">{t("empty")}</span>
          </>
        )}
      </div>

      <input
        ref={inputRef}
        type="file"
        accept="image/jpeg,image/png"
        className="sr-only"
        onChange={handleInputChange}
        aria-hidden="true"
        tabIndex={-1}
        disabled={disabled}
      />

      <p className="text-[14px] text-slate-700">{label}</p>

      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="min-h-[44px] text-[13px] text-slate-500 hover:text-slate-700"
          onClick={() => inputRef.current?.click()}
          disabled={disabled || uploading}
        >
          {populated ? t("replace") : t("upload")}
        </Button>
        {populated && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="min-h-[44px] text-[13px] text-slate-500 hover:text-red-600"
            onClick={onRemove}
            disabled={disabled || uploading}
          >
            <X className="mr-1 h-3.5 w-3.5" />
            {t("remove")}
          </Button>
        )}
      </div>

      {displayError && (
        <p className="text-[14px] text-red-600" role="alert">
          {displayError}
        </p>
      )}
    </div>
  );
}

export type { ImageSlot };
