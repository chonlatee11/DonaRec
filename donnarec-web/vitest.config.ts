import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "path";

/**
 * Vitest config.
 *
 * `environment: "node"` is the default, mirroring the Next.js Route Handler
 * runtime for the BFF trust-boundary tests (03-12) — no DOM needed. React
 * component tests (e.g. PublicDonationForm.test.tsx, 06-06) opt into jsdom
 * per-file with a `// @vitest-environment jsdom` docblock.
 *
 * @vitejs/plugin-react transforms JSX/TSX (the project's tsconfig sets
 * jsx: "preserve" for Next.js, which esbuild alone will not parse in tests).
 *
 * The `@` alias mirrors tsconfig.json's `@/*` path so tests can resolve
 * `@/components/*`, `@/lib/*`, `@/messages/*`.
 */
export default defineConfig({
  plugins: [react()],
  test: {
    environment: "node",
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "."),
    },
  },
});
