import { Badge } from "@strait/ui/components/badge";
import { fragmentOn } from "basehub";
import { RichText } from "basehub/react-rich-text";
import Image from "next/image";
import { Suspense } from "react";

import Shell from "@/components/layout/shell.tsx";
import PostAuthorCompact from "./post-author-compact.tsx";
import PostCtaSidebar from "./post-cta-sidebar.tsx";
import PostNavigation from "./post-navigation.tsx";
import PostRelated from "./post-related.tsx";
import PostShare from "./post-share.tsx";
import PostTocClient from "./post-toc.client.tsx";
import { richTextComponents } from "./rich-text.tsx";
import {
  calculateReadingTime,
  extractHeadingsFromRichText,
  formatReadingTime,
} from "./utils.ts";

export const PostFragment = fragmentOn("BlogPostComponent", {
  _id: true,
  _title: true,
  _slug: true,
  _slugPath: true,
  ogImage: {
    url: true,
  },
  image: {
    dark: {
      url: true,
    },
    light: {
      url: true,
    },
  },
  description: true,
  publishedAt: true,
  categories: true,
  keywords: true,
  modifiedDate: true,
  ogTitle: true,
  ogDescription: true,
  twitterTitle: true,
  twitterDescription: true,
  authors: {
    _id: true,
    _title: true,
    _slug: true,
    role: true,
    x: true,
    image: {
      url: true,
    },
    company: {
      _title: true,
    },
  },
  body: { json: { content: true } },
});

export type PostFragment = fragmentOn.infer<typeof PostFragment>;

const Post = (post: PostFragment) => {
  const title = post._title;
  const description = post.description;
  const publishedAt = post.publishedAt;
  const categories = post.categories;
  const ogImage = post.ogImage;
  const body = post.body;
  const authors = post.authors;
  const slug = post._slug;

  const headings = extractHeadingsFromRichText(
    body.json.content as Parameters<typeof extractHeadingsFromRichText>[0]
  );
  const readingTime = calculateReadingTime(
    body.json.content as Parameters<typeof calculateReadingTime>[0]
  );

  return (
    <div className="relative pb-20">
      {/* Hero Header */}
      <header className="relative isolate overflow-hidden pt-32 pb-8 sm:pt-40 sm:pb-12">
        <div className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_var(--primary)/0.06,_transparent_40%)]" />
        <div className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_transparent,_var(--background)_70%)]" />
        <div className="paper-texture absolute inset-0 -z-10 opacity-[0.02]" />

        <Shell variant="wide">
          <div className="mx-auto max-w-[800px] text-center">
            {/* Categories, Date, and Reading Time */}
            <div className="mb-4 flex flex-wrap items-center justify-center gap-2 text-sm">
              {categories?.length > 0 ? (
                categories.map((category) => (
                  <Badge key={category} variant="outline">
                    {category}
                  </Badge>
                ))
              ) : (
                <Badge variant="outline">Blog</Badge>
              )}
              <span aria-hidden="true" className="text-muted-foreground">
                &bull;
              </span>
              <time className="text-muted-foreground" dateTime={publishedAt}>
                {new Date(publishedAt).toLocaleDateString("en-US", {
                  day: "numeric",
                  month: "long",
                  year: "numeric",
                })}
              </time>
              <span aria-hidden="true" className="text-muted-foreground">
                &bull;
              </span>
              <span className="text-muted-foreground">
                {formatReadingTime(readingTime)}
              </span>
            </div>

            <h1 className="mb-4 text-balance text-4xl tracking-tight sm:text-5xl lg:text-6xl">
              {title}
            </h1>

            {description ? (
              <p className="mb-6 text-balance text-base text-muted-foreground/70 leading-relaxed sm:text-lg">
                {description}
              </p>
            ) : null}

            {/* Author + Share */}
            <div className="flex flex-col items-center justify-center gap-4 sm:flex-row sm:gap-6">
              <PostAuthorCompact authors={authors} />
              <div className="hidden h-4 w-px bg-border sm:block" />
              <PostShare slug={slug} title={title} />
            </div>
          </div>
        </Shell>
      </header>

      {/* Cover Image */}
      {ogImage ? (
        <Shell variant="wide">
          <div className="mb-10">
            <div className="relative aspect-[16/9] overflow-hidden rounded-2xl bg-muted shadow-lg">
              <Image
                alt={`Cover image: ${title}`}
                className="object-cover"
                fill
                priority
                sizes="(max-width: 768px) 100vw, (max-width: 1200px) 80vw, 1200px"
                src={ogImage.url}
              />
            </div>
          </div>
        </Shell>
      ) : null}

      {/* Main Content — 3-column layout */}
      <Shell variant="wide">
        <div className="relative lg:grid lg:grid-cols-[1fr_minmax(0,800px)_1fr] lg:gap-8">
          {/* Left sidebar — Table of Contents */}
          <aside className="hidden lg:block">
            <PostTocClient headings={headings} />
          </aside>

          {/* Center — Article */}
          <main className="min-w-0">
            {/* Mobile TOC */}
            <div className="lg:hidden">
              <PostTocClient headings={headings} />
            </div>

            <article className="mx-auto max-w-none">
              <RichText components={{ ...richTextComponents }}>
                {body.json.content}
              </RichText>
            </article>

            <Suspense fallback={null}>
              <PostNavigation currentSlug={slug} />
            </Suspense>
          </main>

          {/* Right sidebar — CTA */}
          <aside className="hidden lg:block">
            <Suspense fallback={null}>
              <PostCtaSidebar />
            </Suspense>
          </aside>
        </div>

        {/* Related Posts */}
        <Suspense fallback={null}>
          <PostRelated categories={categories ?? []} currentPostId={post._id} />
        </Suspense>
      </Shell>
    </div>
  );
};

export default Post;
