"use client";

import { useCallback, useEffect, useId, useRef, useState } from "react";
import { Menu, X } from "lucide-react";
import { Button } from "@/components/ui/button";

/**
 * MobileNavDrawer — the `<768px` slide-in wrapper around AppShell's existing
 * `<aside>` nav markup (UI-SPEC §"Mobile Navigation Retrofit", NFR-06).
 *
 * Renders BOTH the hamburger trigger (in the AppShell header, `md:hidden`) and
 * the fixed-position slide-in drawer + backdrop, sharing a single `open` state
 * so the header trigger and the overlay stay in sync. On `md` and up the whole
 * unit is `hidden` — the desktop 256px sidebar in AppShell is unchanged.
 *
 * Accessibility Contract (UI-SPEC §Accessibility):
 *   - Drawer: role="dialog" aria-modal="true" aria-label="เมนูนำทาง",
 *     focus trapped while open, focus returns to the hamburger trigger on close.
 *   - Hamburger: aria-expanded reflects open/closed, aria-controls → drawer id.
 *
 * T-06-28: the drawer renders the SAME nav markup (passed as `children`) behind
 * the SAME middleware/role guards as the desktop sidebar — no new route or
 * capability is exposed by the responsive variant.
 *
 * Slate/blue back-office theme only — no warm public theme here.
 */
export function MobileNavDrawer({
  children,
}: {
  /** The shared sidebar content (brand + role-gated nav links) from AppShell. */
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(false);
  // `entered` drives the slide-in: the panel mounts at -translate-x-full, then
  // flips to translate-x-0 on the next frame so transition-transform animates.
  const [entered, setEntered] = useState(false);
  const drawerId = useId();
  const hamburgerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);

  const close = useCallback(() => setOpen(false), []);

  // ── Escape to close + Tab focus trap while open ────────────────────────────
  useEffect(() => {
    if (!open) return;

    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        close();
        return;
      }
      if (e.key !== "Tab") return;

      const panel = panelRef.current;
      if (!panel) return;
      const focusable = panel.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])'
      );
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const active = document.activeElement as HTMLElement | null;

      if (e.shiftKey) {
        if (active === first || !panel.contains(active)) {
          e.preventDefault();
          last.focus();
        }
      } else {
        if (active === last || !panel.contains(active)) {
          e.preventDefault();
          first.focus();
        }
      }
    }

    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [open, close]);

  // ── Slide-in + focus into the drawer on open; lock body scroll while open ──
  useEffect(() => {
    if (!open) {
      setEntered(false);
      return;
    }
    // Next frame: flip -translate-x-full → translate-x-0 (animate) and move
    // focus onto the close button.
    const raf = requestAnimationFrame(() => {
      setEntered(true);
      closeRef.current?.focus();
    });
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      cancelAnimationFrame(raf);
      document.body.style.overflow = prevOverflow;
    };
  }, [open]);

  // ── Return focus to the hamburger after the drawer closes ──────────────────
  const wasOpen = useRef(false);
  useEffect(() => {
    if (wasOpen.current && !open) {
      hamburgerRef.current?.focus();
    }
    wasOpen.current = open;
  }, [open]);

  return (
    <div className="md:hidden">
      {/* Hamburger trigger — lives at the header's left edge, 44px target */}
      <Button
        ref={hamburgerRef}
        type="button"
        variant="ghost"
        size="icon"
        aria-label="เมนู"
        aria-expanded={open}
        aria-controls={drawerId}
        onClick={() => setOpen(true)}
        className="h-11 w-11 text-slate-700"
      >
        <Menu className="h-5 w-5" />
      </Button>

      {open && (
        <>
          {/* Backdrop — click to close */}
          <div
            className="fixed inset-0 z-40 bg-slate-900/40"
            aria-hidden="true"
            onClick={close}
          />

          {/* Slide-in panel */}
          <div
            ref={panelRef}
            id={drawerId}
            role="dialog"
            aria-modal="true"
            aria-label="เมนูนำทาง"
            className={[
              "fixed inset-y-0 left-0 z-50 flex w-[280px] max-w-[85vw] flex-col",
              "bg-slate-100 border-r border-slate-200 shadow-xl",
              "transition-transform duration-200 ease-out",
              entered ? "translate-x-0" : "-translate-x-full",
            ].join(" ")}
          >
            {/* X close button — top-right, 44px target */}
            <Button
              ref={closeRef}
              type="button"
              variant="ghost"
              size="icon"
              aria-label="ปิดเมนู"
              onClick={close}
              className="absolute right-2 top-2 z-10 h-11 w-11 text-slate-700"
            >
              <X className="h-5 w-5" />
            </Button>

            {/* Shared sidebar content (brand + role-gated nav links) */}
            {children}
          </div>
        </>
      )}
    </div>
  );
}
