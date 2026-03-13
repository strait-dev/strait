import type { MetadataRoute } from "next";

export default function robots(): MetadataRoute.Robots {
  const baseUrl = process.env.NEXT_PUBLIC_WEBSITE_URL || "https://trystrait.ai";

  return {
    rules: [
      // Default rule for all crawlers
      {
        userAgent: "*",
        allow: "/",
        disallow: ["/api/", "/_next/"],
      },
      // AI search engine crawlers — explicitly allowed for GEO visibility
      {
        userAgent: "GPTBot",
        allow: "/",
        disallow: ["/api/"],
      },
      {
        userAgent: "ChatGPT-User",
        allow: "/",
        disallow: ["/api/"],
      },
      {
        userAgent: "PerplexityBot",
        allow: "/",
        disallow: ["/api/"],
      },
      {
        userAgent: "Google-Extended",
        allow: "/",
        disallow: ["/api/"],
      },
      {
        userAgent: "ClaudeBot",
        allow: "/",
        disallow: ["/api/"],
      },
      {
        userAgent: "anthropic-ai",
        allow: "/",
        disallow: ["/api/"],
      },
    ],
    sitemap: `${baseUrl}/sitemap.xml`,
  };
}
