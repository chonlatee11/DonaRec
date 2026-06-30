import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";

/**
 * next-intl plugin — registers `i18n/request.ts` as the request config.
 * Locale is resolved per-request from a cookie (non-routing approach).
 * Default locale: th (Thai), second: en.
 */
const withNextIntl = createNextIntlPlugin("./i18n/request.ts");

const nextConfig: NextConfig = {
  // Strict mode for better development experience
  reactStrictMode: true,
};

export default withNextIntl(nextConfig);
