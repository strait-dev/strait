import path from "node:path";
import { createMDX } from "fumadocs-mdx/next";

const withMDX = createMDX();

export default withMDX({
  turbopack: {
    root: path.resolve(import.meta.dirname, "../.."),
  },
  reactStrictMode: true,
  reactCompiler: true,
  transpilePackages: ["@strait/ui"],
  redirects() {
    return [
      // Old Mintlify root pages -> Getting Started section
      ...[
        "introduction",
        "quickstart",
        "architecture",
        "changelog",
        "migrations",
      ].map((page) => ({
        source: `/${page}`,
        destination:
          page === "introduction"
            ? "/docs/getting-started"
            : page === "migrations"
              ? "/docs/development/migrations"
              : `/docs/getting-started/${page}`,
        permanent: true,
      })),
      // Old Mintlify section paths -> new paths
      ...["concepts", "guides", "operations", "development"].map(
        (section) => ({
          source: `/${section}/:slug`,
          destination: `/docs/${section}/:slug`,
          permanent: true,
        }),
      ),
      // Configuration merged into guides
      {
        source: "/configuration/:slug",
        destination: "/docs/guides/:slug",
        permanent: true,
      },
      // SDKs, CLI, API Reference
      ...["sdks", "cli", "api-reference"].map((section) => ({
        source: `/${section}/:slug`,
        destination: `/docs/${section}/:slug`,
        permanent: true,
      })),
      // Section index pages
      ...["sdks", "cli", "api-reference"].map((section) => ({
        source: `/${section}`,
        destination: `/docs/${section}`,
        permanent: true,
      })),
    ];
  },
});
