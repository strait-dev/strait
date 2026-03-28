import type { SiteConfig } from "@/types/index.ts";

const websiteUrl = import.meta.env.PUBLIC_WEBSITE_URL || "https://trystrait.ai";

export const siteConfig: SiteConfig = {
  name: "Strait",
  title: "Strait - Open Source Job Orchestration",
  description:
    "Strait is an open-source job orchestration platform for background jobs, workflows, and AI agents. Queue, orchestrate, and observe every async workload from one platform.",
  url: websiteUrl,
  ogImage: "/og.png",
  logo: {
    src: "/strait.svg",
    alt: "Strait Logo",
    width: 32,
    height: 32,
  },
  links: {
    twitter: "https://twitter.com/leonardomso",
    github: "https://github.com/leonardomso/strait",
    linkedin: "https://linkedin.com/company/strait",
    instagram: "https://instagram.com/trystrait",
  },
  metadata: {
    keywords: [
      "Go job orchestration",
      "PostgreSQL job queue",
      "background job processing",
      "workflow DAG engine",
      "Postgres-backed job queue",
      "job retries and dead letter queue",
      "workflow approvals",
      "AI agent runtime",
      "job scheduler",
      "developer job platform",
    ],
    author: "Leonardo Maldonado",
    themeColor: "#2563EB",
    locale: "en_US",
    siteName: "Strait — Open Source Job Orchestration",
  },
  openGraph: {
    type: "website",
    locale: "en_US",
    url: websiteUrl,
    title: "Strait — Open Source Job Orchestration",
    description:
      "One platform to queue, orchestrate, and observe background jobs, workflows, and AI agents. Open-source, self-hostable, with SDKs for TypeScript, Python, Go, Ruby, and Rust.",
    siteName: "Strait",
    images: [
      {
        url: "/og.png",
        width: 1200,
        height: 630,
        alt: "Strait — Open Source Job Orchestration",
      },
    ],
  },
  twitter: {
    card: "summary_large_image",
    creator: "@leonardomso",
    images: ["/og.png"],
  },
  icons: {
    icon: "/favicon.ico",
    shortcut: "/favicon-16x16.png",
    apple: "/apple-touch-icon.png",
  },
  manifest: "/site.webmanifest",
};
