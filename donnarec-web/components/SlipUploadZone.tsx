"use client";

import { useRef, useState, useCallback } from "react";
import { useTranslations } from "next-intl";
import { ExternalLink, FileText, UploadCloud, X } from "lucide-react";
import { Button } from "@/components/ui/button";

/** 10 MB in bytes — client-side UX pre-check (T-03-35: server is authority) */
const MAX_FILE_BYTES = 10 * 1024 * 1024;

interface SlipUploadZoneProps {
  /**
   * Presigned URL of an existing server-side slip (from DonationDetail.slip_url).
   * When set, shows the "ดูสลิป" + "เปลี่ยนสลิป" UI.
   */
  existingSlipUrl?: string | null;
  /** Called when user selects a new local file (pending upload) */
  onFileChange: (file: File | null) => void;
  /**
   * Called when user clicks "ลบสลิป" on an existing server-side slip.
   * Only provided on edit forms (no server slip to remove on create).
   */
  onRemoveExisting?: () => Promise<void>;
  /** True while an upload is in-flight (shows spinner) */
  uploading?: boolean;
  /** Error message from server (422 magic-byte / 413 size / network) */
  serverError?: string;
  disabled?: boolean;
  /**
   * When true, the empty-zone label drops the "(ไม่บังคับ)" hint and renders a
   * required asterisk (used by the public Flow B form where a slip is mandatory,
   * D-80). Default false keeps Flow A's optional behavior unchanged.
   */
  required?: boolean;
  /**
   * Overrides the empty-zone label text. When omitted, falls back to
   * t("slip.zoneLabel"). Lets the public form supply its own (required) label
   * without this shared component reaching into a caller-specific i18n namespace.
   */
  label?: string;
}

/**
 * SlipUploadZone — drag-drop / click-to-browse slip attachment zone.
 *
 * UI-SPEC §Screen 2 §Section 3 + §Screen 5 (Slip Viewer):
 *   - Dashed-border drop zone, 56px min-height, full width
 *   - Accepts JPG / PNG / PDF ≤ 10 MB
 *   - On file selected locally: filename + size + "ลบสลิป" ghost button
 *   - On existing server slip: PDF icon + "ดูสลิป" link (new tab) + "เปลี่ยนสลิป" ghost
 *
 * T-03-35: client size pre-check is UX-only — server magic-byte validation is authority.
 * D-54: "remove" calls soft-delete API (existing slip) not hard delete.
 */
export function SlipUploadZone({
  existingSlipUrl,
  onFileChange,
  onRemoveExisting,
  uploading = false,
  serverError,
  disabled = false,
  required = false,
  label,
}: SlipUploadZoneProps) {
  const t = useTranslations();
  const zoneLabel = label ?? t("slip.zoneLabel");
  const inputRef = useRef<HTMLInputElement>(null);

  /** Locally selected file (pending upload — not yet on server) */
  const [localFile, setLocalFile] = useState<File | null>(null);
  /** Client-side validation error (size / type) */
  const [clientError, setClientError] = useState<string | null>(null);
  const [isDragOver, setIsDragOver] = useState(false);
  const [isRemovingExisting, setIsRemovingExisting] = useState(false);

  const displayError = clientError ?? serverError ?? null;

  // ── File selection handler ────────────────────────────────────────────────

  const handleFile = useCallback(
    (file: File) => {
      setClientError(null);

      // Client-side size pre-check (UX-only; server enforces magic-byte)
      if (file.size > MAX_FILE_BYTES) {
        setClientError(t("errors.fileTooLarge"));
        return;
      }

      // Basic MIME pre-check (spoofable — server is authority)
      const allowed = ["image/jpeg", "image/png", "application/pdf"];
      if (!allowed.includes(file.type)) {
        setClientError(t("errors.fileTypeRejected"));
        return;
      }

      setLocalFile(file);
      onFileChange(file);
    },
    [onFileChange, t]
  );

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) handleFile(file);
    // Reset input so re-selecting same file triggers onChange
    e.target.value = "";
  };

  // ── Drag-and-drop handlers ────────────────────────────────────────────────

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

  // ── Remove local pending file ─────────────────────────────────────────────

  const clearLocalFile = () => {
    setLocalFile(null);
    setClientError(null);
    onFileChange(null);
    if (inputRef.current) inputRef.current.value = "";
  };

  // ── Remove existing server-side slip ─────────────────────────────────────

  const handleRemoveExisting = async () => {
    if (!onRemoveExisting) return;
    setIsRemovingExisting(true);
    try {
      await onRemoveExisting();
    } finally {
      setIsRemovingExisting(false);
    }
  };

  // ── Format file size for display ──────────────────────────────────────────

  function formatSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }

  // ── State: local file selected (pending upload) ───────────────────────────

  if (localFile) {
    return (
      <div className="flex flex-col gap-2">
        <div className="flex items-center justify-between rounded-md border border-slate-200 bg-slate-50 px-4 py-3">
          <div className="flex items-center gap-2.5 min-w-0">
            <FileText className="h-5 w-5 shrink-0 text-slate-500" />
            <div className="min-w-0">
              <p className="truncate text-[14px] font-medium text-slate-900">
                {localFile.name}
              </p>
              <p className="text-[12px] text-slate-500">
                {formatSize(localFile.size)}
              </p>
            </div>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="ml-2 shrink-0 text-[13px] text-slate-500 hover:text-red-600"
            onClick={clearLocalFile}
            disabled={disabled || uploading}
            aria-label={t("slip.remove")}
          >
            <X className="mr-1 h-3.5 w-3.5" />
            {t("slip.remove")}
          </Button>
        </div>
        {uploading && (
          <p className="text-[13px] text-slate-500">{t("slip.uploading")}</p>
        )}
        {displayError && (
          <p className="text-[14px] text-red-600" role="alert">
            {displayError}
          </p>
        )}
      </div>
    );
  }

  // ── State: existing server-side slip (has URL from DonationDetail) ────────

  if (existingSlipUrl && !localFile) {
    return (
      <div className="flex flex-col gap-2">
        <div className="flex items-center justify-between rounded-md border border-slate-200 bg-slate-50 px-4 py-3">
          <div className="flex items-center gap-2.5">
            <FileText className="h-5 w-5 shrink-0 text-slate-500" />
            <a
              href={existingSlipUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1.5 text-[14px] text-blue-600 hover:underline"
              aria-label={t("slip.view")}
            >
              <ExternalLink className="h-3.5 w-3.5" />
              {t("slip.view")}
            </a>
          </div>
          <div className="flex items-center gap-2">
            {/* Change: select a new file to replace */}
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="text-[13px] text-slate-500 hover:text-slate-700"
              onClick={() => inputRef.current?.click()}
              disabled={disabled || uploading}
            >
              {t("slip.change")}
            </Button>
            {/* Remove existing (soft-delete) */}
            {onRemoveExisting && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="text-[13px] text-slate-500 hover:text-red-600"
                onClick={handleRemoveExisting}
                disabled={disabled || uploading || isRemovingExisting}
                aria-label={t("actions.removeSlip")}
              >
                <X className="mr-1 h-3.5 w-3.5" />
                {t("actions.removeSlip")}
              </Button>
            )}
          </div>
        </div>
        {/* Hidden file input for "เปลี่ยนสลิป" */}
        <input
          ref={inputRef}
          type="file"
          accept=".jpg,.jpeg,.png,.pdf,image/jpeg,image/png,application/pdf"
          className="sr-only"
          onChange={handleInputChange}
          aria-hidden="true"
          tabIndex={-1}
        />
        {serverError && (
          <p className="text-[14px] text-red-600" role="alert">
            {serverError}
          </p>
        )}
      </div>
    );
  }

  // ── State: empty drop zone ────────────────────────────────────────────────

  return (
    <div className="flex flex-col gap-2">
      {/* Accessible label above zone */}
      <p className="text-[14px] text-slate-600">
        {zoneLabel}
        {required && (
          <span className="text-red-600" aria-hidden="true">
            {" "}
            *
          </span>
        )}
      </p>

      <div
        role="button"
        tabIndex={disabled ? -1 : 0}
        aria-label={zoneLabel}
        aria-required={required || undefined}
        onClick={() => !disabled && inputRef.current?.click()}
        onKeyDown={(e) => {
          if (!disabled && (e.key === "Enter" || e.key === " ")) {
            e.preventDefault();
            inputRef.current?.click();
          }
        }}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        className={[
          "flex min-h-[56px] w-full cursor-pointer flex-col items-center justify-center gap-1.5",
          "rounded-md border-2 border-dashed px-4 py-5 transition-colors",
          disabled
            ? "cursor-not-allowed border-slate-200 bg-slate-50"
            : isDragOver
            ? "border-blue-400 bg-blue-50"
            : "border-slate-300 bg-white hover:border-blue-400 hover:bg-blue-50/30",
        ].join(" ")}
      >
        <UploadCloud
          className={[
            "h-7 w-7",
            isDragOver ? "text-blue-500" : "text-slate-400",
          ].join(" ")}
        />
        <p className="text-[14px] text-slate-600">
          {t("slip.dragOrClick")}{" "}
          <span className="font-medium text-blue-600 underline">
            {t("slip.browse")}
          </span>
        </p>
        <p className="text-[12px] text-slate-400">{t("slip.formatHint")}</p>
      </div>

      {/* Hidden file input */}
      <input
        ref={inputRef}
        type="file"
        accept=".jpg,.jpeg,.png,.pdf,image/jpeg,image/png,application/pdf"
        className="sr-only"
        onChange={handleInputChange}
        aria-hidden="true"
        tabIndex={-1}
        disabled={disabled}
      />

      {displayError && (
        <p className="text-[14px] text-red-600" role="alert">
          {displayError}
        </p>
      )}
    </div>
  );
}
