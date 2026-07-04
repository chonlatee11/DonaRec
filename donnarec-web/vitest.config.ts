import { defineConfig } from "vitest/config";
import path from "path";

/**
 * Vitest config for the BFF route-handler trust-boundary tests (03-12).
 * `environment: "node"` mirrors the Next.js Route Handler runtime (no DOM
 * needed). The `@` alias mirrors tsconfig.json's `@/*` path so route files
 * under app/api/bff/** can resolve `@/lib/bff` and `@/lib/auth`.
 */
export default defineConfig({
  test: {
    environment: "node",
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "."),
    },
  },
});
