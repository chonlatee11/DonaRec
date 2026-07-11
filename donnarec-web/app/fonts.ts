import { Trirong, IBM_Plex_Sans_Thai, IBM_Plex_Mono } from "next/font/google";

/**
 * (public)-only font instances (06-UI-SPEC "Dual-Theme Architecture" §3).
 * next/font/google requires the loader function to be called once, at
 * module scope — this plain (non-route-file) module is that single call
 * site. app/(public)/layout.tsx imports these and applies their .variable
 * className on its warm-scope wrapper div; nothing else references them.
 *
 * Deviation note (Rule 3 — blocking issue): the plan's action text said to
 * add these directly in app/layout.tsx, but Next.js validates layout.tsx
 * exports against a fixed shape (default component + metadata/
 * generateMetadata/etc.) and rejects arbitrary extra named exports
 * ("X is not a valid Layout export field"). Moving the instantiation into
 * this shared module is the standard next/font/google pattern for sharing
 * a font across files without violating that export-shape check.
 */
export const trirong = Trirong({
  // UI-SPEC: weights 500+600 (normal), plus italic 400 for the single
  // <em>-style hero accent pattern — next/font/google generates the full
  // weight x style cross product, so 400 is included to get 400-italic.
  subsets: ["thai", "latin"],
  weight: ["400", "500", "600"],
  style: ["normal", "italic"],
  variable: "--font-trirong",
  display: "swap",
});

export const ibmPlexSansThai = IBM_Plex_Sans_Thai({
  subsets: ["thai", "latin"],
  weight: ["400", "600"],
  variable: "--font-ibm-plex-sans-thai",
  display: "swap",
});

export const ibmPlexMono = IBM_Plex_Mono({
  subsets: ["latin"],
  weight: ["400", "500"],
  variable: "--font-ibm-plex-mono",
  display: "swap",
});
