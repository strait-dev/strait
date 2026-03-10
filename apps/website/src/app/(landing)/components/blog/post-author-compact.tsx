import Image from "next/image";
import Link from "next/link";

type Author = {
  _id: string;
  _title: string;
  _slug: string;
  role: string | null;
  image: { url: string } | null;
  company: { _title: string } | null;
};

type PostAuthorCompactProps = {
  authors: Author[];
};

const PostAuthorCompact = ({ authors }: PostAuthorCompactProps) => {
  if (authors.length === 0) {
    return null;
  }

  const displayedAuthors = authors.slice(0, 2);
  const remainingCount = authors.length - 2;

  return (
    <div className="flex items-center gap-3">
      <div className="flex -space-x-2">
        {displayedAuthors.map((author) => (
          <Link
            className="relative size-8 overflow-hidden rounded-full border-2 border-background transition-transform hover:z-10 hover:scale-110"
            href={`/blog/author/${author._slug}`}
            key={author._id}
          >
            {author.image?.url ? (
              <Image
                alt={author._title}
                className="object-cover"
                fill
                sizes="32px"
                src={author.image.url}
              />
            ) : (
              <div className="flex size-full items-center justify-center bg-muted text-muted-foreground text-xs">
                {author._title.charAt(0)}
              </div>
            )}
          </Link>
        ))}
      </div>
      <div className="text-sm">
        <span className="text-muted-foreground">by </span>
        <span className="font-medium text-foreground">
          {displayedAuthors.map((author, index) => (
            <span key={author._id}>
              {index > 0 && ", "}
              <Link
                className="hover:text-primary hover:underline"
                href={`/blog/author/${author._slug}`}
              >
                {author._title}
              </Link>
            </span>
          ))}
          {remainingCount > 0 && ` +${remainingCount}`}
        </span>
      </div>
    </div>
  );
};

export default PostAuthorCompact;
