import { basehub } from "basehub";
import type { MetadataRoute } from "next";
import { getAllComparisonSlugs } from "./(landing)/compare/data.ts";
import { getAllFeatureSlugs } from "./(landing)/features/data.ts";
import { getAllUseCaseSlugs } from "./(landing)/use-cases/data.ts";

/**
 * Dynamic sitemap generator for SEO optimization.
 *
 * Includes all static marketing pages plus dynamically fetched blog posts
 * from BaseHub CMS. Revalidates hourly to balance performance and freshness.
 *
 * @see https://nextjs.org/docs/app/api-reference/file-conventions/metadata/sitemap
 */

// Revalidate the sitemap every hour
export const revalidate = 3600;

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const baseUrl = import.meta.env.PUBLIC_WEBSITE_URL || "https://trystrait.ai";

  // Static pages with SEO priorities
  const staticPages: MetadataRoute.Sitemap = [
    // Homepage - highest priority
    {
      url: baseUrl,
      lastModified: new Date(),
      changeFrequency: "weekly",
      priority: 1.0,
    },
    // Pricing - key conversion page
    {
      url: `${baseUrl}/pricing`,
      lastModified: new Date(),
      changeFrequency: "monthly",
      priority: 0.9,
    },
    // Social proof and content
    {
      url: `${baseUrl}/blog`,
      lastModified: new Date(),
      changeFrequency: "weekly",
      priority: 0.7,
    },
    // Legal pages - required but low priority
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

  // Fetch blog posts and authors from BaseHub
  let blogPages: MetadataRoute.Sitemap = [];
  let authorPages: MetadataRoute.Sitemap = [];

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

    const posts = data.website.blog.posts.items.filter(
      (post) => post._slug !== "nos-somos-a-strait"
    );

    blogPages = posts.map((post) => ({
      url: `${baseUrl}/blog/${post._slug}`,
      lastModified: post.publishedAt ? new Date(post.publishedAt) : new Date(),
      changeFrequency: "weekly" as const,
      priority: 0.6,
    }));

    // Extract unique author slugs
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
      changeFrequency: "monthly" as const,
      priority: 0.5,
    }));
  } catch (error) {
    // If BaseHub fails, continue with static pages only
    console.error("Failed to fetch blog posts for sitemap:", error);
  }

  // Feature pages
  const featurePages: MetadataRoute.Sitemap = getAllFeatureSlugs().map(
    (slug) => ({
      url: `${baseUrl}/features/${slug}`,
      lastModified: new Date(),
      changeFrequency: "monthly" as const,
      priority: 0.8,
    })
  );

  // Features index
  featurePages.unshift({
    url: `${baseUrl}/features`,
    lastModified: new Date(),
    changeFrequency: "monthly",
    priority: 0.85,
  });

  // Comparison pages
  const comparisonPages: MetadataRoute.Sitemap = getAllComparisonSlugs().map(
    (slug) => ({
      url: `${baseUrl}/compare/${slug}`,
      lastModified: new Date(),
      changeFrequency: "monthly" as const,
      priority: 0.7,
    })
  );

  // Use case pages
  const useCasePages: MetadataRoute.Sitemap = getAllUseCaseSlugs().map(
    (slug) => ({
      url: `${baseUrl}/use-cases/${slug}`,
      lastModified: new Date(),
      changeFrequency: "monthly" as const,
      priority: 0.75,
    })
  );

  return [
    ...staticPages,
    ...featurePages,
    ...comparisonPages,
    ...useCasePages,
    ...blogPages,
    ...authorPages,
  ];
}
