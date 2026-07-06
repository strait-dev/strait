/// <reference types="vite/client" />

/**
 * Central metadata helper for route `head` definitions.
 *
 * Every page passes a short `title`; the helper appends the site name and emits
 * a consistent set of title, Open Graph, and Twitter card tags. The OG image is
 * resolved to an absolute URL because link unfurlers (Slack, iMessage, Twitter,
 * Facebook) do not follow root-relative image paths.
 */

const SITE_NAME = "Strait";

const TRAILING_SLASH = /\/$/;
const LEADING_SLASH = /^\//;

const DEFAULT_DESCRIPTION =
  "Strait is a production-grade job orchestration platform for scheduling, executing, and monitoring distributed workloads.";

// VITE_BASE_URL is the app's public origin, inlined at build time for both the
// server and client bundles. Fall back to the local dev origin when unset.
const BASE_URL = (
  import.meta.env.VITE_BASE_URL ?? "http://localhost:5173"
).replace(TRAILING_SLASH, "");

const OG_IMAGE_URL = `${BASE_URL}/og.png`;
const OG_IMAGE_WIDTH = "4800";
const OG_IMAGE_HEIGHT = "2500";
const OG_IMAGE_ALT = `${SITE_NAME} — job orchestration platform`;

type MetaTag =
  | { title: string }
  | { name: string; content: string }
  | { property: string; content: string };

type SeoOptions = {
  /** Page-specific title. Rendered as `${title} · Strait`; omit for the site default. */
  title?: string;
  /** Overrides the default site description for this page. */
  description?: string;
  /** Absolute or root-relative image URL; defaults to the shared OG image. */
  image?: string;
};

function toAbsolute(image: string): string {
  if (image.startsWith("http://") || image.startsWith("https://")) {
    return image;
  }
  return `${BASE_URL}/${image.replace(LEADING_SLASH, "")}`;
}

/**
 * Build the meta tag array for a route's `head`. Later routes override the root
 * defaults because Router de-duplicates meta by `title`, `name`, and `property`.
 */
export function seo({ title, description, image }: SeoOptions = {}): MetaTag[] {
  const pageTitle = title ? `${title} · ${SITE_NAME}` : SITE_NAME;
  const pageDescription = description ?? DEFAULT_DESCRIPTION;
  const ogImage = image ? toAbsolute(image) : OG_IMAGE_URL;

  return [
    { title: pageTitle },
    { name: "description", content: pageDescription },
    { property: "og:title", content: pageTitle },
    { property: "og:description", content: pageDescription },
    { property: "og:type", content: "website" },
    { property: "og:site_name", content: SITE_NAME },
    { property: "og:image", content: ogImage },
    { property: "og:image:width", content: OG_IMAGE_WIDTH },
    { property: "og:image:height", content: OG_IMAGE_HEIGHT },
    { property: "og:image:alt", content: OG_IMAGE_ALT },
    { name: "twitter:card", content: "summary_large_image" },
    { name: "twitter:title", content: pageTitle },
    { name: "twitter:description", content: pageDescription },
    { name: "twitter:image", content: ogImage },
  ];
}
