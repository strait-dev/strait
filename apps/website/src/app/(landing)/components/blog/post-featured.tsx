import { ArrowRight01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import Image from "next/image";
import Link from "next/link";
import type { PostFragment } from "./post.tsx";

const PostFeatured = ({
  _title,
  ogImage,
  publishedAt,
  description,
  ogDescription,
  twitterDescription,
  categories,
  _slug,
}: PostFragment) => {
  const shortDescription = ogDescription || twitterDescription || description;
  return (
    <article aria-labelledby="featured-article-title" className="w-full">
      <Link
        aria-label={`Read featured article: ${_title}`}
        className="group block cursor-default"
        href={`/blog/${_slug}`}
      >
        <div className="grid grid-cols-1 gap-6 overflow-hidden rounded-2xl border border-border/60 bg-card transition-colors hover:border-foreground/20 hover:shadow-lg lg:grid-cols-2 lg:gap-0">
          <figure className="relative aspect-[16/9] overflow-hidden lg:aspect-auto lg:h-full">
            {ogImage?.url ? (
              <Image
                alt={`Featured article cover: ${_title}`}
                className="object-cover transition-transform duration-300 group-hover:scale-[1.02]"
                fill
                priority
                sizes="(max-width: 1024px) 100vw, 50vw"
                src={ogImage.url}
              />
            ) : (
              <div className="flex h-full min-h-[240px] items-center justify-center bg-muted">
                <span className="text-muted-foreground">No image</span>
              </div>
            )}
          </figure>

          <div className="flex flex-col justify-center gap-4 p-6 lg:p-10">
            <div className="flex flex-wrap items-center gap-2 text-sm">
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
              <time
                className="text-muted-foreground text-sm"
                dateTime={String(publishedAt)}
              >
                {new Date(publishedAt).toLocaleDateString("en-US", {
                  month: "long",
                  day: "numeric",
                  year: "numeric",
                })}
              </time>
            </div>

            <h2
              className="font-semibold text-2xl transition-colors group-hover:text-foreground sm:text-3xl lg:text-4xl"
              id="featured-article-title"
            >
              {_title}
            </h2>

            <p className="line-clamp-3 text-muted-foreground text-sm leading-relaxed sm:text-base lg:text-lg">
              {shortDescription}
            </p>

            <div className="mt-2">
              <span className="inline-flex items-center gap-1 font-medium text-foreground text-sm transition-colors group-hover:underline">
                Read article
                <HugeiconsIcon
                  className="size-4 transition-transform group-hover:translate-x-0.5"
                  icon={ArrowRight01Icon}
                />
              </span>
            </div>
          </div>
        </div>
      </Link>
    </article>
  );
};

export default PostFeatured;
