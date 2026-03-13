import { ArrowRight01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import Image from "next/image";
import Link from "next/link";
import type { PostFragment } from "./post.tsx";

const PostPreview = ({
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
    <article aria-labelledby={`article-${_slug}`} className="h-full">
      <Link
        aria-label={`Read article: ${_title}`}
        className="group block h-full"
        href={`/blog/${_slug}`}
      >
        <div className="flex h-full flex-col overflow-hidden rounded-2xl border border-border/60 bg-card transition-colors hover:border-primary hover:shadow-md">
          {/* Image section */}
          <figure className="relative aspect-[16/9] overflow-hidden">
            {ogImage?.url ? (
              <Image
                alt={`Article cover: ${_title}`}
                className="object-cover transition-transform duration-300 group-hover:scale-[1.02]"
                fill
                sizes="(max-width: 768px) 100vw, 50vw"
                src={ogImage.url}
              />
            ) : null}
          </figure>

          {/* Content section */}
          <div className="flex flex-1 flex-col gap-3 p-6">
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
              className="font-semibold text-xl tracking-tight transition-colors group-hover:text-primary sm:text-2xl"
              id={`article-${_slug}`}
            >
              {_title}
            </h2>

            <p className="line-clamp-2 text-muted-foreground text-sm leading-relaxed sm:text-base">
              {shortDescription}
            </p>

            <div className="mt-auto pt-2">
              <span className="inline-flex items-center gap-1 font-medium text-primary text-sm transition-colors group-hover:underline">
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

export default PostPreview;
