import type { APIRoute } from "astro";
import { basehub } from "basehub";
import { getAllComparisonSlugs } from "@/app/(landing)/compare/data.ts";
import { getAllFeatureSlugs } from "@/app/(landing)/features/data.ts";
import { getAllUseCaseSlugs } from "@/app/(landing)/use-cases/data.ts";

type SitemapEntry = {
  url: string;
  lastModified: Date;
  changeFrequency: string;
  priority: number;
};

export const GET: APIRoute = async () => {
  const baseUrl = import.meta.env.PUBLIC_WEBSITE_URL || "https://trystrait.ai";

  const staticPages: SitemapEntry[] = [
    {
      url: baseUrl,
      lastModified: new Date(),
      changeFrequency: "weekly",
      priority: 1.0,
    },
    {
      url: `${baseUrl}/pricing`,
      lastModified: new Date(),
      changeFrequency: "monthly",
      priority: 0.9,
    },
    {
      url: `${baseUrl}/blog`,
      lastModified: new Date(),
      changeFrequency: "weekly",
      priority: 0.7,
    },
    {
      url: `${baseUrl}/privacy`,
      lastModified: new Date(),
      changeFrequency: "yearly",
      priority: 0.3,
    },
    {
      url: `${baseUrl}/terms`,
      lastModified: new Date(),
      changeFrequency: "yearly",
      priority: 0.3,
    },
  ];

  let blogPages: SitemapEntry[] = [];
  let authorPages: SitemapEntry[] = [];

  try {
    const data = await basehub().query({
      website: {
        blog: {
          posts: {
            items: {
              _slug: true,
              publishedAt: true,
              authors: {
                _slug: true,
              },
            },
          },
        },
      },
    });

    const posts = (data as any).website.blog.posts.items.filter(
      (post: any) => post._slug !== "nos-somos-a-strait"
    );

    blogPages = posts.map((post: any) => ({
      url: `${baseUrl}/blog/${post._slug}`,
      lastModified: post.publishedAt ? new Date(post.publishedAt) : new Date(),
      changeFrequency: "weekly",
      priority: 0.6,
    }));

    const authorSlugs = new Set<string>();
    for (const post of posts) {
      for (const author of post.authors ?? []) {
        if (author._slug) {
          authorSlugs.add(author._slug);
        }
      }
    }

    authorPages = [...authorSlugs].map((slug) => ({
      url: `${baseUrl}/blog/author/${slug}`,
      lastModified: new Date(),
      changeFrequency: "monthly",
      priority: 0.5,
    }));
  } catch (error) {
    console.error("Failed to fetch blog posts for sitemap:", error);
  }

  const featurePages: SitemapEntry[] = [
    {
      url: `${baseUrl}/features`,
      lastModified: new Date(),
      changeFrequency: "monthly",
      priority: 0.85,
    },
    ...getAllFeatureSlugs().map((slug) => ({
      url: `${baseUrl}/features/${slug}`,
      lastModified: new Date(),
      changeFrequency: "monthly" as const,
      priority: 0.8,
    })),
  ];

  const comparisonPages: SitemapEntry[] = getAllComparisonSlugs().map(
    (slug) => ({
      url: `${baseUrl}/compare/${slug}`,
      lastModified: new Date(),
      changeFrequency: "monthly" as const,
      priority: 0.7,
    })
  );

  const useCasePages: SitemapEntry[] = getAllUseCaseSlugs().map((slug) => ({
    url: `${baseUrl}/use-cases/${slug}`,
    lastModified: new Date(),
    changeFrequency: "monthly" as const,
    priority: 0.75,
  }));

  const allPages = [
    ...staticPages,
    ...featurePages,
    ...comparisonPages,
    ...useCasePages,
    ...blogPages,
    ...authorPages,
  ];

  const urlEntries = allPages
    .map(
      (page) => `  <url>
    <loc>${page.url}</loc>
    <lastmod>${page.lastModified.toISOString()}</lastmod>
    <changefreq>${page.changeFrequency}</changefreq>
    <priority>${page.priority.toFixed(1)}</priority>
  </url>`
    )
    .join("\n");

  const sitemap = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
${urlEntries}
</urlset>`;

  return new Response(sitemap, {
    headers: {
      "Content-Type": "application/xml; charset=utf-8",
      "Cache-Control": "public, max-age=3600, s-maxage=3600",
    },
  });
};
