import { dirname } from "path";
import { fileURLToPath } from "url";
import { FlatCompat } from "@eslint/eslintrc";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const compat = new FlatCompat({
  baseDirectory: __dirname,
});

const eslintConfig = [
  // Ignore Next.js generated files, build output, node_modules, and shadcn-generated components
  {
    ignores: [
      ".next/**",
      "node_modules/**",
      "next-env.d.ts",
      "components/ui/**",
      "hooks/**",
    ],
  },
  ...compat.extends("next/core-web-vitals", "next/typescript"),
];

export default eslintConfig;
