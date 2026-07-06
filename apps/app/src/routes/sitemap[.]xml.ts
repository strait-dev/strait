import { createFileRoute } from "@tanstack/react-router";

// Public, linkable entry pages. The authenticated dashboard is intentionally
// excluded: it is behind auth and disallowed in robots.txt.
const PUBLIC_PATHS = ["/login", "/signup", "/forgot-password"];

function buildSitemap(origin: string): string {
  const urls = PUBLIC_PATHS.map(
    (path) => `  <url>\n    <loc>${origin}${path}</loc>\n  </url>`
  ).join("\n");
  return `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${urls}\n</urlset>\n`;
}

export const Route = createFileRoute("/sitemap.xml")({
  server: {
    handlers: {
      GET: ({ request }) => {
        const { origin } = new URL(request.url);
        return new Response(buildSitemap(origin), {
          headers: {
            "Content-Type": "application/xml; charset=utf-8",
            "Cache-Control": "public, max-age=3600",
          },
        });
      },
    },
  },
});
