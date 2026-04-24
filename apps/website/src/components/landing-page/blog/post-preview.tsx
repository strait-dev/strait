import { ArrowRight01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
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
      <a
        aria-label={`Read article: ${_title}`}
        className="group block h-full"
        href={`/blog/${_slug}`}
      >
        <div className="flex h-full flex-col overflow-hidden rounded-2xl border border-border/60 bg-card transition-colors hover:border-foreground/20 hover:shadow-md">
          {/* Image section */}
          <figure className="relative aspect-[16/9] overflow-hidden">
            {ogImage?.url ? (
              <img
                alt={`Article cover: ${_title}`}
                className="absolute inset-0 h-full w-full object-cover transition-transform duration-300 group-hover:scale-[1.02]"
                decoding="async"
                loading="lazy"
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
              className="font-semibold text-2xl transition-colors group-hover:text-foreground sm:text-3xl lg:text-4xl"
              id={`article-${_slug}`}
            >
              {_title}
            </h2>

            <p className="line-clamp-2 text-muted-foreground text-sm leading-relaxed sm:text-base">
              {shortDescription}
            </p>

            <div className="mt-auto pt-2">
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
      </a>
    </article>
  );
};

export default PostPreview;
