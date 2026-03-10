import path from "node:path";
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  turbopack: {
    root: path.resolve(import.meta.dirname, "../.."),
  },
  reactStrictMode: true,
  reactCompiler: true, // Enable React Compiler for automatic memoization
  compiler: {
    removeConsole: process.env.NODE_ENV !== "development", // Remove console.log in production
  },
  experimental: {
    // Tree-shake barrel imports for better bundle size
    optimizePackageImports: [
      "@hugeicons/core-free-icons",
      "@hugeicons/react",
      "date-fns",
    ],
  },
  images: {
    dangerouslyAllowSVG: true,
    remotePatterns: [
      {
        protocol: "https",
        hostname: "mwesulbn1k.ufs.sh",
        port: "",
        pathname: "/f/**",
      },
      { hostname: "assets.basehub.com" },
      { hostname: "basehub.earth" },
      { hostname: "api.basehub.com" },
    ],
  },
  transpilePackages: ["@strait/ui"],
  headers() {
    return [
      {
        source: "/:path*",
        headers: [
          {
            key: "Link",
            value: [
              "<https://assets.basehub.com>; rel=preconnect; crossorigin",
              "<https://basehub.earth>; rel=preconnect; crossorigin",
            ].join(", "),
          },
          {
            key: "X-Robots-Tag",
            value: "all",
          },
        ],
      },
    ];
  },
};

export default nextConfig;
