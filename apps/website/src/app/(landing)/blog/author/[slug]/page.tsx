import { basehub } from "basehub";
import { Pump } from "basehub/react-pump";
import type { Metadata } from "next";
import { draftMode } from "next/headers";
import Image from "next/image";
import { notFound } from "next/navigation";
import { Suspense } from "react";
import Pagination from "@/app/(landing)/components/blog/pagination.tsx";
import type { PostFragment } from "@/app/(landing)/components/blog/post.tsx";
import { PostFragment as PostFragmentQuery } from "@/app/(landing)/components/blog/post.tsx";
import PostFeatured from "@/app/(landing)/components/blog/post-featured.tsx";
import PostPreview from "@/app/(landing)/components/blog/post-preview.tsx";
import Shell from "@/components/layout/shell.tsx";
import { siteConfig } from "@/config/site.ts";
import { generateMetadata as generatePageMetadata } from "@/lib/metadata.ts";
import {
  getBreadcrumbSchema,
  getPersonSchema,
  JsonLd,
} from "@/lib/structured-data.tsx";

const BASE_URL = process.env.NEXT_PUBLIC_WEBSITE_URL || "https://trystrait.ai";

const POSTS_PER_PAGE = 8;

export const dynamic = "force-static";

type Author = {
  _id: string;
  _title: string;
  _slug: string;
  role: string | null;
  image: { url: string } | null;
  company: { _title: string } | null;
  x: string | null;
};

type AuthorPageProps = {
  params: Promise<{ slug: string }>;
  searchParams: Promise<{ page?: string }>;
};

const findAuthorFromPosts = (
  posts: { authors?: Author[] }[],
  slug: string
): Author | null => {
  for (const post of posts) {
    const found = post.authors?.find((a) => a._slug === slug);
    if (found) {
      return found;
    }
  }
  return null;
};

const AuthorHeader = ({
  author,
  totalPosts,
}: {
  author: Author;
  totalPosts: number;
}) => (
  <section className="relative isolate overflow-hidden pt-32 pb-12 sm:pt-40 sm:pb-16">
    <div className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_var(--primary)/0.06,_transparent_40%)]" />
    <div className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_transparent,_var(--background)_70%)]" />
    <div className="paper-texture absolute inset-0 -z-10 opacity-[0.02]" />

    <Shell variant="wide">
      <div className="mx-auto flex max-w-3xl flex-col items-center text-center">
        {author.image?.url && (
          <div className="relative mb-6 size-24 overflow-hidden rounded-full border-4 border-background shadow-lg sm:size-32">
            <Image
              alt={author._title}
              className="object-cover"
              fill
              priority
              sizes="(max-width: 640px) 96px, 128px"
              src={author.image.url}
            />
          </div>
        )}

        <h1 className="text-balance text-3xl text-foreground tracking-tight sm:text-4xl lg:text-5xl">
          {author._title}
        </h1>

        {author.role && (
          <p className="mt-2 text-lg text-muted-foreground">
            {author.role}
            {author.company?._title && ` at ${author.company._title}`}
          </p>
        )}

        <p className="mt-6 text-muted-foreground text-sm">
          {totalPosts} {totalPosts === 1 ? "article" : "articles"} published
        </p>
      </div>
    </Shell>
  </section>
);

const PostsGrid = ({
  posts,
  currentPage,
  totalPages,
}: {
  posts: PostFragment[];
  currentPage: number;
  totalPages: number;
}) => (
  <Shell variant="wide">
    <div className="overflow-hidden rounded-2xl border border-border/50">
      <div className="grid grid-cols-1 gap-px bg-border/50 md:grid-cols-2">
        {posts.map((post) => (
          <div className="bg-background p-6" key={post._id}>
            <PostPreview {...post} />
          </div>
        ))}
      </div>

      {totalPages > 1 && (
        <div className="border-border/50 border-t bg-background py-8">
          <Suspense fallback={null}>
            <Pagination currentPage={currentPage} totalPages={totalPages} />
          </Suspense>
        </div>
      )}
    </div>
  </Shell>
);

export const generateStaticParams = async () => {
  try {
    const data = await basehub({ cache: "no-store" }).query({
      website: {
        blog: {
          posts: {
            __args: { first: 100 },
            items: { authors: { _slug: true } },
          },
        },
      },
    });

    const slugs = new Set<string>();
    for (const post of data.website.blog.posts.items) {
      for (const author of post.authors ?? []) {
        if (author._slug) {
          slugs.add(author._slug);
        }
      }
    }

    return [...slugs].map((slug) => ({ slug }));
  } catch {
    return [];
  }
};

export const generateMetadata = async ({
  params: _params,
  searchParams: _searchParams,
}: AuthorPageProps): Promise<Metadata | undefined> => {
  try {
    const [{ slug }, { page }, draft] = await Promise.all([
      _params,
      _searchParams,
      draftMode(),
    ]);
    const currentPage = Math.max(1, Number(page) || 1);

    const data = await basehub({ draft: draft.isEnabled }).query({
      website: {
        blog: {
          posts: {
            __args: { first: 100 },
            items: {
              authors: {
                _id: true,
                _title: true,
                _slug: true,
                role: true,
                image: { url: true },
                company: { _title: true },
                x: true,
              },
            },
          },
        },
      },
    });

    const author = findAuthorFromPosts(data.website.blog.posts.items, slug);
    if (!author) {
      return;
    }

    const baseTitle = `${author._title} - Author at Strait`;
    const title =
      currentPage > 1 ? `${baseTitle} - Page ${currentPage}` : baseTitle;
    const description = author.role
      ? `${author._title} is a ${author.role}${author.company?._title ? ` at ${author.company._title}` : ""}. Read their articles on the Strait blog.`
      : `Read articles by ${author._title} on the Strait blog.`;

    return generatePageMetadata({
      title,
      description,
      path: `/blog/author/${author._slug}`,
      ogImage: author.image?.url ?? siteConfig.ogImage,
      canonical: `${BASE_URL}/blog/author/${author._slug}`,
    });
  } catch {
    return;
  }
};

const AuthorPage = async ({
  params: _params,
  searchParams: _searchParams,
}: AuthorPageProps) => {
  const [{ slug }, { page }] = await Promise.all([_params, _searchParams]);
  const currentPage = Math.max(1, Number(page) || 1);

  return (
    <Pump
      queries={[
        {
          website: {
            blog: {
              posts: {
                __args: { orderBy: "publishedAt__DESC", first: 100 },
                items: PostFragmentQuery,
              },
            },
          },
        },
      ]}
    >
      {/* biome-ignore lint/suspicious/useAwait: BaseHub Pump requires async callback */}
      {async ([{ website }]) => {
        "use server";

        const allPosts = website.blog.posts.items;
        const author = findAuthorFromPosts(allPosts, slug);

        if (!author) {
          return notFound();
        }

        const authorPosts = allPosts.filter((post) =>
          post.authors?.some((a) => a._slug === slug)
        );

        const totalPosts = authorPosts.length;
        const totalPages = Math.ceil(totalPosts / POSTS_PER_PAGE);
        const startIndex = (currentPage - 1) * POSTS_PER_PAGE;
        const paginatedPosts = authorPosts.slice(
          startIndex,
          startIndex + POSTS_PER_PAGE
        );

        const isFirstPage = currentPage === 1;
        const featuredPost = isFirstPage ? paginatedPosts[0] : null;
        const remainingPosts = isFirstPage
          ? paginatedPosts.slice(1)
          : paginatedPosts;

        const authorUrl = `${BASE_URL}/blog/author/${author._slug}`;

        return (
          <main className="flex flex-col">
            <JsonLd
              data={getPersonSchema({
                name: author._title,
                url: authorUrl,
                image: author.image?.url,
                jobTitle: author.role ?? undefined,
                worksFor: author.company?._title ?? "Strait",
                sameAs: author.x ? [`https://x.com/${author.x}`] : undefined,
              })}
            />
            <JsonLd
              data={getBreadcrumbSchema([
                { name: "Home", url: BASE_URL },
                { name: "Blog", url: `${BASE_URL}/blog` },
                { name: author._title, url: authorUrl },
              ])}
            />

            <AuthorHeader author={author} totalPosts={totalPosts} />

            {paginatedPosts.length > 0 && (
              <section className="pb-20 sm:pb-28">
                {featuredPost && (
                  <Shell variant="wide">
                    <div className="pb-12">
                      <PostFeatured {...featuredPost} />
                    </div>
                  </Shell>
                )}

                {remainingPosts.length > 0 && (
                  <PostsGrid
                    currentPage={currentPage}
                    posts={remainingPosts}
                    totalPages={totalPages}
                  />
                )}

                {remainingPosts.length === 0 && totalPages > 1 && (
                  <Shell variant="wide">
                    <div className="py-8">
                      <Suspense fallback={null}>
                        <Pagination
                          currentPage={currentPage}
                          totalPages={totalPages}
                        />
                      </Suspense>
                    </div>
                  </Shell>
                )}
              </section>
            )}

            {paginatedPosts.length === 0 && (
              <section className="pb-20 sm:pb-28">
                <Shell variant="wide">
                  <div className="text-center">
                    <p className="text-lg text-muted-foreground">
                      No articles published yet. Check back soon!
                    </p>
                  </div>
                </Shell>
              </section>
            )}
          </main>
        );
      }}
    </Pump>
  );
};

export default AuthorPage;
