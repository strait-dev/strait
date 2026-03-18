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
});
