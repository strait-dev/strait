import { Badge } from "@strait/ui/components/badge";
import { RichText } from "basehub/react-rich-text";
import PostAuthorCompact from "@/components/landing-page/blog/post-author-compact.tsx";
import PostShare from "@/components/landing-page/blog/post-share.tsx";
import PostTocClient from "@/components/landing-page/blog/post-toc.client.tsx";
import { richTextComponents } from "@/components/landing-page/blog/rich-text.tsx";
import {
  calculateReadingTime,
  extractHeadingsFromRichText,
  formatReadingTime,
} from "@/components/landing-page/blog/utils.ts";
import Shell from "@/components/layout/shell.tsx";

type Author = {
  _id: string;
  _title: string;
  _slug: string;
  role: string | null;
  x: string | null;
  image: { url: string } | null;
  company: { _title: string } | null;
};

type PostContentProps = {
  _title: string;
  _slug: string;
  description: string | null;
  publishedAt: string;
  categories: string[] | null;
  ogImage: { url: string } | null;
  authors: Author[] | null;
  body: { json: { content: unknown } };
  children?: React.ReactNode;
};

const PostContent = (props: PostContentProps) => {
  const {
    _title: title,
    _slug: slug,
    description,
    publishedAt,
    categories,
    ogImage,
    authors,
    body,
    children,
  } = props;

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
              {categories && categories.length > 0 ? (
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

            <h1 className="mb-4 text-balance text-4xl sm:text-5xl lg:text-6xl">
              {title}
            </h1>

            {description ? (
              <p className="mb-6 text-balance text-base text-muted-foreground leading-relaxed sm:text-lg">
                {description}
              </p>
            ) : null}

            {/* Author + Share */}
            <div className="flex flex-col items-center justify-center gap-4 sm:flex-row sm:gap-6">
              {authors && <PostAuthorCompact authors={authors} />}
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
              <img
                alt={`Cover image: ${title}`}
                className="absolute inset-0 h-full w-full object-cover"
                decoding="async"
                height={675}
                loading="lazy"
                src={ogImage.url}
                width={1200}
              />
            </div>
          </div>
        </Shell>
      ) : null}

      {/* Main Content -- 3-column layout */}
      <Shell variant="wide">
        <div className="relative lg:grid lg:grid-cols-[1fr_minmax(0,800px)_1fr] lg:gap-8">
          {/* Left sidebar -- Table of Contents */}
          <aside className="hidden lg:block">
            <PostTocClient headings={headings} />
          </aside>

          {/* Center -- Article */}
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
          </main>

          {/* Right sidebar -- rendered by parent .astro page */}
          {children && <aside className="hidden lg:block">{children}</aside>}
        </div>
      </Shell>
    </div>
  );
};

export default PostContent;
