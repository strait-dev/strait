
import { Menu01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { cn } from "@strait/ui/utils";
import { useCallback, useEffect, useState } from "react";
import type { TocHeading } from "./utils.ts";

type PostTocClientProps = {
  headings: TocHeading[];
};

const PostTocClient = ({ headings }: PostTocClientProps) => {
  const [activeId, setActiveId] = useState<string>("");
  const [isOpen, setIsOpen] = useState(false);

  useEffect(() => {
    const headingElements = headings
      .map((h) => document.getElementById(h.id))
      .filter(Boolean) as HTMLElement[];

    if (headingElements.length === 0) {
      return;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            setActiveId(entry.target.id);
            break;
          }
        }
      },
      { rootMargin: "-80px 0px -80% 0px", threshold: 0 }
    );

    for (const el of headingElements) {
      observer.observe(el);
    }

    return () => observer.disconnect();
  }, [headings]);

  const handleClick = useCallback(
    (e: React.MouseEvent<HTMLAnchorElement>, id: string) => {
      e.preventDefault();
      const element = document.getElementById(id);
      if (element) {
        const offset = 100;
        const elementPosition = element.getBoundingClientRect().top;
        const offsetPosition = elementPosition + window.scrollY - offset;

        window.scrollTo({ top: offsetPosition, behavior: "smooth" });
        setActiveId(id);
        setIsOpen(false);
      }
    },
    []
  );

  if (headings.length === 0) {
    return null;
  }

  return (
    <>
      <div className="mb-6 lg:hidden">
        <Button
          className="w-full justify-between"
          onClick={() => setIsOpen(!isOpen)}
          variant="outline"
        >
          <span className="flex items-center gap-2">
            <HugeiconsIcon className="size-4" icon={Menu01Icon} />
            Table of Contents
          </span>
          <span className="text-muted-foreground text-sm">
            {headings.length} sections
          </span>
        </Button>

        {isOpen && (
          <nav className="mt-2 rounded-lg border border-border bg-card p-4">
            <ul className="space-y-2">
              {headings.map((heading) => (
                <li key={heading.id}>
                  <a
                    className={cn(
                      "block text-sm transition-colors hover:text-foreground",
                      heading.level === 3 && "pl-4",
                      activeId === heading.id
                        ? "font-medium text-foreground"
                        : "text-muted-foreground"
                    )}
                    href={`#${heading.id}`}
                    onClick={(e) => handleClick(e, heading.id)}
                  >
                    {heading.text}
                  </a>
                </li>
              ))}
            </ul>
          </nav>
        )}
      </div>

      <nav
        aria-label="Table of Contents"
        className="sticky top-24 hidden max-h-[calc(100vh-8rem)] overflow-y-auto lg:block"
      >
        <p className="mb-4 font-semibold text-foreground text-sm">
          On this page
        </p>
        <ul className="space-y-2 border-border border-l">
          {headings.map((heading) => (
            <li key={heading.id}>
              <a
                className={cn(
                  "-ml-px block border-l-2 py-1 pl-4 text-sm transition-colors hover:border-foreground/20 hover:text-foreground",
                  heading.level === 3 && "pl-8",
                  activeId === heading.id
                    ? "border-foreground font-medium text-foreground"
                    : "border-transparent text-muted-foreground"
                )}
                href={`#${heading.id}`}
                onClick={(e) => handleClick(e, heading.id)}
              >
                {heading.text}
              </a>
            </li>
          ))}
        </ul>
      </nav>
    </>
  );
};

export default PostTocClient;
