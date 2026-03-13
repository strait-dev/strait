import { basehub } from "basehub";
import { draftMode } from "next/headers";
import type { PostFragment } from "./post.tsx";
import PostPreview from "./post-preview.tsx";

type PostRelatedProps = {
  currentPostId: string;
  categories: string[];
};

const PostRelated = async ({ currentPostId, categories }: PostRelatedProps) => {
  const draft = await draftMode();

  const { website } = await basehub({ draft: draft.isEnabled }).query({
    website: {
      blog: {
        posts: {
          __args: {
            orderBy: "publishedAt__DESC",
            first: 4,
          },
          items: {
            _id: true,
            _title: true,
            _slug: true,
            _slugPath: true,
            ogImage: { url: true },
            image: { dark: { url: true }, light: { url: true } },
            description: true,
            publishedAt: true,
            categories: true,
            authors: {
              _id: true,
              _title: true,
              role: true,
              image: { url: true },
              company: { _title: true },
            },
            body: { json: { content: true } },
          },
        },
      },
    },
  });

  const allPosts = website.blog.posts.items as PostFragment[];
  const otherPosts = allPosts.filter((post) => post._id !== currentPostId);

  const categoryMatchedPosts =
    categories.length > 0
      ? otherPosts.filter((post) =>
          post.categories?.some((cat) => categories.includes(cat))
        )
      : [];

  const relatedPosts =
    categoryMatchedPosts.length >= 3
      ? categoryMatchedPosts.slice(0, 3)
      : otherPosts.slice(0, 3);

  if (relatedPosts.length === 0) {
    return null;
  }

  return (
    <section className="mt-20 border-border border-t pt-12">
      <h2 className="mb-8 font-bold text-2xl text-foreground tracking-tight">
        Related Articles
      </h2>
      <div className="overflow-hidden rounded-2xl border border-border/50">
        <div className="grid grid-cols-1 gap-px bg-border/50 md:grid-cols-3">
          {relatedPosts.map((post) => (
            <div className="bg-background p-6" key={post._id}>
              <PostPreview {...post} />
            </div>
          ))}
        </div>
      </div>
    </section>
  );
};

export default PostRelated;
