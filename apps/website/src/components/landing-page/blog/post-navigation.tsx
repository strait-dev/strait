import { ArrowLeft01Icon, ArrowRight01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { basehub } from "basehub";

type PostNavigationProps = {
  currentSlug: string;
};

type NavPost = {
  _title: string;
  _slug: string;
};

const PostNavigation = async ({ currentSlug }: PostNavigationProps) => {
  const { website } = await basehub({ draft: false }).query({
    website: {
      blog: {
        posts: {
          __args: {
            orderBy: "publishedAt__DESC",
          },
          items: {
            _title: true,
            _slug: true,
            publishedAt: true,
          },
        },
      },
    },
  });

  const allPosts = website.blog.posts.items;
  const currentIndex = allPosts.findIndex((post) => post._slug === currentSlug);

  if (currentIndex === -1) {
    return null;
  }

  const prevPost: NavPost | null =
    currentIndex < allPosts.length - 1
      ? (allPosts[currentIndex + 1] ?? null)
      : null;
  const nextPost: NavPost | null =
    currentIndex > 0 ? (allPosts[currentIndex - 1] ?? null) : null;

  if (!(prevPost || nextPost)) {
    return null;
  }

  return (
    <nav
      aria-label="Post navigation"
      className="mt-12 grid grid-cols-1 gap-4 border-border border-t pt-8 sm:grid-cols-2"
    >
      {prevPost ? (
        <a
          className="group flex items-center gap-3 rounded-2xl border border-border/60 bg-card p-4 transition-all hover:border-border hover:shadow-md"
          href={`/blog/${prevPost._slug}`}
        >
          <HugeiconsIcon
            className="size-5 shrink-0 text-muted-foreground transition-transform group-hover:-translate-x-1"
            icon={ArrowLeft01Icon}
          />
          <div className="min-w-0">
            <p className="text-muted-foreground text-xs uppercase tracking-wide">
              Previous
            </p>
            <p className="truncate font-medium text-foreground">
              {prevPost._title}
            </p>
          </div>
        </a>
      ) : (
        <div />
      )}

      {nextPost ? (
        <a
          className="group flex items-center justify-end gap-3 rounded-2xl border border-border/60 bg-card p-4 text-right transition-all hover:border-border hover:shadow-md"
          href={`/blog/${nextPost._slug}`}
        >
          <div className="min-w-0">
            <p className="text-muted-foreground text-xs uppercase tracking-wide">
              Next
            </p>
            <p className="truncate font-medium text-foreground">
              {nextPost._title}
            </p>
          </div>
          <HugeiconsIcon
            className="size-5 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-1"
            icon={ArrowRight01Icon}
          />
        </a>
      ) : (
        <div />
      )}
    </nav>
  );
};

export default PostNavigation;
