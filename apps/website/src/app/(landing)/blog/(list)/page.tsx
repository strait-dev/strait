import { Skeleton } from "@strait/ui/components/skeleton";
import { Pump } from "basehub/react-pump";
import type { Metadata } from "next";
import { draftMode } from "next/headers";
import { Suspense } from "react";
import BlogHero from "@/app/(landing)/components/blog/blog-hero";
import Pagination from "@/app/(landing)/components/blog/pagination";
import { PostFragment } from "@/app/(landing)/components/blog/post";
import PostFeatured from "@/app/(landing)/components/blog/post-featured";
import PostPreview from "@/app/(landing)/components/blog/post-preview";
import CTA from "@/app/(landing)/components/common/cta/cta";
import Shell from "@/components/layout/shell";
import { fetchPageMetadata } from "@/lib/basehub-metadata";
import { generateMetadata as generatePageMetadata } from "@/lib/metadata";
import {
  getBreadcrumbSchema,
  getCollectionPageSchema,
  JsonLd,
} from "@/lib/structured-data";

const BASE_URL = process.env.NEXT_PUBLIC_WEBSITE_URL || "https://trystrait.ai";

const POSTS_PER_PAGE = 8;

const SKELETON_ITEMS = ["a", "b", "c", "d", "e", "f", "g"] as const;

const PostsGridSkeleton = () => (
  <section className="pb-20 sm:pb-28">
    <Shell variant="wide">
      <div className="pb-8">
        <div className="grid grid-cols-1 gap-6 overflow-hidden rounded-2xl border border-border/60 lg:grid-cols-2 lg:gap-0">
          <div className="relative aspect-[16/9] overflow-hidden lg:aspect-auto lg:h-full">
            <Skeleton className="h-full w-full rounded-none" />
          </div>
          <div className="flex flex-col justify-center gap-4 p-6 lg:p-10">
            <div className="flex items-center gap-3">
              <Skeleton className="h-5 w-24 rounded-full" />
              <Skeleton className="h-5 w-32" />
            </div>
            <Skeleton className="h-10 w-3/4" />
            <Skeleton className="h-5 w-full" />
            <Skeleton className="h-5 w-2/3" />
            <Skeleton className="h-5 w-24" />
          </div>
        </div>
      </div>

      <div className="overflow-hidden rounded-2xl border border-border/50">
        <div className="grid grid-cols-1 gap-px bg-border/50 md:grid-cols-2">
          {SKELETON_ITEMS.map((id) => (
            <div className="bg-background p-6" key={id}>
              <div className="flex h-full flex-col overflow-hidden rounded-2xl border border-border/60">
                <div className="relative aspect-[16/9] overflow-hidden">
                  <Skeleton className="h-full w-full" />
                </div>
                <div className="flex flex-1 flex-col gap-3 p-6">
                  <div className="flex items-center gap-3">
                    <Skeleton className="h-4 w-20 rounded-full" />
                    <Skeleton className="h-4 w-28" />
                  </div>
                  <Skeleton className="h-6 w-full" />
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="h-4 w-3/4" />
                  <Skeleton className="mt-auto h-4 w-24" />
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    </Shell>
  </section>
);

type BlogIndexProps = {
  searchParams: Promise<{ page?: string }>;
};

export async function generateMetadata({
  searchParams,
}: BlogIndexProps): Promise<Metadata> {
  const [metadataEntry, params] = await Promise.all([
    fetchPageMetadata("blog"),
    searchParams,
  ]);
  const currentPage = Math.max(1, Number(params.page) || 1);

  const title = metadataEntry?.title ?? "Strait — Blog";
  const description =
    metadataEntry?.description ??
    "Tips, insights, and ideas on AI-powered writing and productivity.";

  return generatePageMetadata({
    title: currentPage > 1 ? `${title} - Page ${currentPage}` : title,
    description,
    path: "/blog",
    ogImage: metadataEntry?.ogImage,
    keywords: metadataEntry?.keywords,
    siteName: metadataEntry?.siteName,
    locale: metadataEntry?.locale,
    appendSiteTitle: !metadataEntry?.title,
    canonical: `${BASE_URL}/blog`,
  });
}

const BlogIndex = async ({ searchParams }: BlogIndexProps) => {
  const [draft, params] = await Promise.all([draftMode(), searchParams]);
  const currentPage = Math.max(1, Number(params.page) || 1);
  const skip = (currentPage - 1) * POSTS_PER_PAGE;

  const blogUrl = `${BASE_URL}/blog`;

  const breadcrumbSchema = getBreadcrumbSchema([
    { name: "Home", url: BASE_URL },
    { name: "Blog", url: blogUrl },
  ]);

  const collectionSchema = getCollectionPageSchema({
    name: "Strait Blog",
    description:
      "Tips, insights, and ideas on AI-powered writing and productivity.",
    url: blogUrl,
  });

  return (
    <main className="flex flex-col">
      <JsonLd data={breadcrumbSchema} />
      <JsonLd data={collectionSchema} />

      <BlogHero />

      <Suspense fallback={<PostsGridSkeleton />}>
        <Pump
          draft={draft.isEnabled}
          next={{ revalidate: 60 }}
          queries={[
            {
              website: {
                blog: {
                  posts: {
                    __args: {
                      orderBy: "publishedAt__DESC",
                      first: POSTS_PER_PAGE,
                      skip,
                    },
                    items: PostFragment,
                    _meta: {
                      totalCount: true,
                    },
                  },
                },
              },
            },
          ]}
        >
          {/* biome-ignore lint/suspicious/useAwait: basehub */}
          {async ([{ website }]) => {
            "use server";

            const posts = website.blog.posts.items;
            const totalCount = website.blog.posts._meta.totalCount;
            const totalPages = Math.ceil(totalCount / POSTS_PER_PAGE);

            if (posts.length === 0) {
              return (
                <section className="py-20 sm:py-28">
                  <Shell variant="wide">
                    <p className="text-center text-lg text-muted-foreground">
                      No posts found. Check back soon!
                    </p>
                  </Shell>
                </section>
              );
            }

            const isFirstPage = currentPage === 1;
            const featuredPost = isFirstPage ? posts[0] : null;
            const remainingPosts = isFirstPage ? posts.slice(1) : posts;

            return (
              <section className="pb-20 sm:pb-28">
                <Shell variant="wide">
                  {featuredPost && (
                    <div className="pb-8">
                      <PostFeatured {...featuredPost} />
                    </div>
                  )}

                  {remainingPosts.length > 0 && (
                    <div className="overflow-hidden rounded-2xl border border-border/50">
                      <div className="grid grid-cols-1 gap-px bg-border/50 md:grid-cols-2">
                        {remainingPosts.map((post) => (
                          <div className="bg-background p-6" key={post._id}>
                            <PostPreview {...post} />
                          </div>
                        ))}
                      </div>

                      {totalPages > 1 && (
                        <div className="border-border/50 border-t bg-background py-8">
                          <Suspense fallback={null}>
                            <Pagination
                              currentPage={currentPage}
                              totalPages={totalPages}
                            />
                          </Suspense>
                        </div>
                      )}
                    </div>
                  )}

                  {remainingPosts.length === 0 && totalPages > 1 && (
                    <div className="py-8">
                      <Suspense fallback={null}>
                        <Pagination
                          currentPage={currentPage}
                          totalPages={totalPages}
                        />
                      </Suspense>
                    </div>
                  )}
                </Shell>
              </section>
            );
          }}
        </Pump>
      </Suspense>

      <CTA />
    </main>
  );
};

export default BlogIndex;
