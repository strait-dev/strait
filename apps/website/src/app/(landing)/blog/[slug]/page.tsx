import { basehub } from "basehub";
import { Pump } from "basehub/react-pump";
import type { Metadata } from "next";
import { draftMode } from "next/headers";
import { notFound } from "next/navigation";
import Post, { PostFragment } from "@/app/(landing)/components/blog/post.tsx";
import { countWords } from "@/app/(landing)/components/blog/utils.ts";
import CTA from "@/app/(landing)/components/common/cta/cta.tsx";
import { siteConfig } from "@/config/site.ts";
import { generateMetadata as generatePageMetadata } from "@/lib/metadata.ts";
import {
  getBlogPostingSchema,
  getBreadcrumbSchema,
  JsonLd,
} from "@/lib/structured-data.tsx";

export const dynamic = "force-static";

export const generateStaticParams = async () => {
  try {
    const data = await basehub({ cache: "no-store" }).query({
      website: {
        blog: {
          posts: {
            items: PostFragment,
          },
        },
      },
    });

    return data.website.blog.posts.items
      .filter((post) => post._slug !== "nos-somos-a-strait") // Temporarily skip problematic slug
      .map((post) => ({
        slug: post._slug,
      }));
  } catch (_error) {
    return [];
  }
};

export const generateMetadata = async ({
  params: _params,
}: {
  params: Promise<{ slug: string }>;
}): Promise<Metadata | undefined> => {
  try {
    const [{ slug }, draft] = await Promise.all([_params, draftMode()]);

    // Skip problematic slug
    if (slug === "nos-somos-a-strait") {
      return;
    }

    const data = await basehub({ draft: draft.isEnabled }).query({
      website: {
        blog: {
          posts: {
            __args: {
              filter: {
                _sys_slug: { eq: slug },
              },
              first: 1,
            },
            items: PostFragment,
          },
        },
      },
    });

    const post = data.website.blog.posts.items[0];

    if (!post) {
      return;
    }

    const postKeywords = post.keywords
      ? post.keywords
          .split(",")
          .map((k: string) => k.trim())
          .filter(Boolean)
      : undefined;

    return generatePageMetadata({
      title: post._title ?? "Blog Post",
      description: post.description ?? siteConfig.description,
      path: `/blog/${post._slug}`,
      noIndex: !post.publishedAt,
      ogImage: post.image?.light?.url ?? siteConfig.ogImage,
      ogTitle: post.ogTitle ?? undefined,
      ogDescription: post.ogDescription ?? undefined,
      twitterTitle: post.twitterTitle ?? undefined,
      twitterDescription: post.twitterDescription ?? undefined,
      keywords: postKeywords,
      article: {
        publishedTime: new Date(post.publishedAt).toISOString(),
        ...(post.modifiedDate && {
          modifiedTime: new Date(post.modifiedDate).toISOString(),
        }),
        authors: post.authors?.map((a) => a._title) ?? [],
        section: post.categories?.[0] ?? undefined,
        tags: post.categories ?? undefined,
      },
    });
  } catch (_error) {
    return;
  }
};

const Page = async ({
  params: _params,
}: {
  params: Promise<{ slug: string }>;
}) => {
  const { slug } = await _params;

  // Skip problematic slug
  if (slug === "nos-somos-a-strait") {
    return notFound();
  }

  return (
    <div>
      <Pump
        queries={[
          {
            website: {
              blog: {
                posts: {
                  __args: {
                    filter: {
                      _sys_slug: {
                        eq: slug,
                      },
                    },
                    first: 1,
                  },
                  items: PostFragment,
                },
              },
            },
          },
        ]}
      >
        {/* biome-ignore lint/suspicious/useAwait: basehub */}
        {async ([
          {
            website: {
              blog: { posts },
            },
          },
        ]) => {
          "use server";
          const post = posts.items.at(0);

          if (!post) {
            return notFound();
          }

          const baseUrl =
            process.env.NEXT_PUBLIC_WEBSITE_URL || "https://trystrait.ai";
          const postUrl = `${baseUrl}/blog/${post._slug}`;

          const postKeywords = post.keywords
            ? post.keywords
                .split(",")
                .map((k: string) => k.trim())
                .filter(Boolean)
            : undefined;

          const wordCount = countWords(
            post.body.json.content as Parameters<typeof countWords>[0]
          );

          const articleSchema = getBlogPostingSchema({
            headline: post._title ?? "Blog Post",
            description: post.description ?? "",
            image: post.image?.light?.url ?? `${baseUrl}/og.png`,
            datePublished: new Date(post.publishedAt).toISOString(),
            ...(post.modifiedDate && {
              dateModified: new Date(post.modifiedDate).toISOString(),
            }),
            authors:
              post.authors?.map((a) => ({
                name: a._title,
                ...(a.image?.url && { image: a.image.url }),
              })) ?? [],
            url: postUrl,
            section: post.categories?.[0] ?? undefined,
            keywords: postKeywords,
            wordCount,
          });

          const breadcrumbSchema = getBreadcrumbSchema([
            { name: "Home", url: baseUrl },
            { name: "Blog", url: `${baseUrl}/blog` },
            { name: post._title ?? "Post", url: postUrl },
          ]);

          return (
            <>
              <JsonLd data={articleSchema} />
              <JsonLd data={breadcrumbSchema} />
              <Post {...post} />
            </>
          );
        }}
      </Pump>
      <CTA />
    </div>
  );
};

export default Page;
