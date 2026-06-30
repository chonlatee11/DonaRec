/**
 * Type declaration for plain CSS file side-effect imports.
 * Next.js handles CSS natively; this declaration lets `tsc --noEmit` accept
 * `import "./globals.css"` in app/layout.tsx.
 */
declare module "*.css" {}
