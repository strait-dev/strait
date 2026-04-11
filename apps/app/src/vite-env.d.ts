/// <reference types="vite/client" />

interface ImportMetaEnv {
  /**
   * Strait edition — `cloud` (default, for strait.dev production) or
   * `community` (self-host Docker image + Deploy to Cloudflare flow).
   * Gates billing/Stripe out of the self-host build. See
   * `src/lib/edition.ts` for the read site.
   */
  readonly VITE_STRAIT_EDITION?: "cloud" | "community";
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
