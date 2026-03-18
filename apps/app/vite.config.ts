import { sentryTanstackStart } from "@sentry/tanstackstart-react/vite";
import tailwindcss from "@tailwindcss/vite";
import { devtools } from "@tanstack/devtools-vite";
import { nitroV2Plugin } from "@tanstack/nitro-v2-vite-plugin";
import { tanstackStart } from "@tanstack/react-start/plugin/vite";
import viteReact from "@vitejs/plugin-react";
import { defineConfig } from "vite";
import viteTsConfigPaths from "vite-tsconfig-paths";

export default defineConfig({
  plugins: [
    devtools(),
    viteTsConfigPaths({
      projects: ["./tsconfig.json"],
    }),
    tailwindcss(),
    tanstackStart({
      router: {
        routeToken: "layout",
      },
      srcDirectory: "src",
    }),
    nitroV2Plugin({ preset: "vercel", compatibilityDate: "2025-10-27" }),
    viteReact(),
    sentryTanstackStart({
      org: process.env.SENTRY_ORG,
      project: process.env.SENTRY_PROJECT,
      authToken: process.env.SENTRY_AUTH_TOKEN,
    }),
  ],
  optimizeDeps: {
    include: ["@hugeicons/react"],
    // Exclude PGlite from optimization - it's a test-only dependency
    exclude: ["@electric-sql/pglite", "drizzle-orm/pglite"],
  },
  build: {
    rollupOptions: {
      // Externalize PGlite and test-only dependencies to prevent bundling
      // PGlite is a 20MB in-memory PostgreSQL used only for API tests
      external: [
        "@electric-sql/pglite",
        "drizzle-orm/pglite",
        "drizzle-orm/pglite/migrator",
        "postgres",
      ],
    },
  },
  ssr: {
    external: ["better-auth", "postgres", "drizzle-orm"],
  },
  server: {
    port: 5173,
    host: true,
  },
});
