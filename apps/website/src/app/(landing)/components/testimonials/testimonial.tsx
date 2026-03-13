import { QuoteUpIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { cn } from "@strait/ui/utils";
import Image from "next/image";

export type TestimonialItem = {
  _id: string;
  _title: string;
  text: string | null;
  authorName: string | null;
  authorCompany: string | null;
  authorPosition: string | null;
  avatar: { url: string } | null;
};

type TestimonialProps = {
  testimonial: TestimonialItem;
  className?: string;
  featured?: boolean;
};

// Diagonal stripe pattern (same as feature showcase)
const patternClasses =
  "relative before:absolute before:inset-0 before:bg-[image:repeating-linear-gradient(315deg,_var(--color-primary)_0,_var(--color-primary)_1px,_transparent_0,_transparent_50%)] before:bg-[size:10px_10px] before:opacity-30";

const Testimonial = ({
  testimonial,
  className,
  featured = false,
}: TestimonialProps) => {
  const initials = testimonial.authorName
    ?.split(" ")
    .filter(Boolean)
    .map((namePart: string) => namePart[0]?.toUpperCase())
    .join("");

  return (
    <article
      className={cn(
        "hover-lift group relative flex h-full flex-col bg-card transition-colors hover:bg-muted/30",
        className
      )}
      key={testimonial._id}
    >
      {/* Quote content */}
      <div className={cn("relative flex-1 p-8", featured && "lg:p-10")}>
        {/* Large decorative quote mark */}
        <HugeiconsIcon
          className="absolute top-6 right-6 size-10 text-primary/20"
          icon={QuoteUpIcon}
        />

        <blockquote>
          <p
            className={cn(
              "text-foreground italic leading-relaxed",
              featured ? "text-xl lg:text-2xl" : "text-lg"
            )}
          >
            &ldquo;{testimonial.text}&rdquo;
          </p>
        </blockquote>
      </div>

      {/* Author info - connected style with pattern */}
      <div className="flex items-stretch border-border/50 border-t">
        {/* Avatar with diagonal pattern background */}
        <div
          className={cn(
            "flex aspect-square w-14 shrink-0 items-center justify-center overflow-hidden border-border/50 border-r",
            patternClasses
          )}
        >
          {testimonial.avatar?.url ? (
            <Image
              alt={`Photo of ${testimonial.authorName ?? "testimonial author"}`}
              className="relative z-10 size-9 rounded-md object-cover"
              height={36}
              src={testimonial.avatar.url}
              width={36}
            />
          ) : (
            <span className="relative z-10 font-semibold text-foreground text-sm">
              {initials || "SW"}
            </span>
          )}
        </div>

        {/* Name and role */}
        <div className="flex flex-1 flex-col justify-center px-4 py-2">
          <span className="font-semibold text-foreground text-sm">
            — {testimonial.authorName}
          </span>
          <span className="text-muted-foreground text-xs">
            {[testimonial.authorPosition, testimonial.authorCompany]
              .filter(Boolean)
              .join(" at ")}
          </span>
        </div>
      </div>
    </article>
  );
};

export default Testimonial;
