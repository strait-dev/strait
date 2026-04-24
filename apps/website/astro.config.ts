import cloudflare from "@astrojs/cloudflare";
import react from "@astrojs/react";
import sitemap from "@astrojs/sitemap";
import { defineConfig } from "astro/config";

export default defineConfig({
  site: "https://strait.dev",
  output: "static",
  trailingSlash: "never",
  compressHTML: true,
  devToolbar: { enabled: false },

  markdown: {
    syntaxHighlight: "prism",
  },

  adapter: cloudflare({
    imageService: "compile",
  }),

  prefetch: {
    prefetchAll: true,
    defaultStrategy: "viewport",
  },

  image: {
    domains: [
      "assets.basehub.com",
      "basehub.earth",
      "api.basehub.com",
      "mwesulbn1k.ufs.sh",
    ],
  },

  security: {
    csp: true,
  },

  experimental: {
    svgo: true,
  },

  integrations: [
    react(),
    sitemap({
      changefreq: "weekly",
      priority: 0.7,
      lastmod: new Date(),
    }),
  ],
});
