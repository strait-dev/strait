
import { ArrowLeft01Icon, ArrowRight01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { cn } from "@strait/ui/utils";

type PaginationProps = {
  currentPage: number;
  totalPages: number;
  basePath: string;
  className?: string;
};

const createPageNumbers = (
  currentPage: number,
  totalPages: number
): (number | "ellipsis-start" | "ellipsis-end")[] => {
  const MAX_VISIBLE = 5;

  if (totalPages <= MAX_VISIBLE) {
    return Array.from({ length: totalPages }, (_, i) => i + 1);
  }

  const pages: (number | "ellipsis-start" | "ellipsis-end")[] = [1];
  const showStartEllipsis = currentPage > 3;
  const showEndEllipsis = currentPage < totalPages - 2;

  if (showStartEllipsis) {
    pages.push("ellipsis-start");
  }

  const start = Math.max(2, currentPage - 1);
  const end = Math.min(totalPages - 1, currentPage + 1);

  for (let i = start; i <= end; i++) {
    pages.push(i);
  }

  if (showEndEllipsis) {
    pages.push("ellipsis-end");
  }

  if (!pages.includes(totalPages)) {
    pages.push(totalPages);
  }

  return pages;
};

const Pagination = ({
  currentPage,
  totalPages,
  basePath,
  className,
}: PaginationProps) => {
  const createPageUrl = (page: number) => {
    if (page === 1) {
      return basePath;
    }
    return `${basePath}/page/${String(page)}`;
  };

  const hasPreviousPage = currentPage > 1;
  const hasNextPage = currentPage < totalPages;
  const pageNumbers = createPageNumbers(currentPage, totalPages);

  if (totalPages <= 1) {
    return null;
  }

  return (
    <nav
      aria-label="Blog pagination"
      className={cn("flex items-center justify-center gap-2", className)}
    >
      {hasPreviousPage ? (
        <Button
          className="cursor-default gap-1.5 text-foreground"
          render={<a href={createPageUrl(currentPage - 1)} />}
          variant="ghost"
        >
          <HugeiconsIcon className="size-4" icon={ArrowLeft01Icon} />
          <span className="hidden sm:inline">Previous</span>
        </Button>
      ) : (
        <Button
          className="pointer-events-auto cursor-not-allowed gap-1.5 text-foreground"
          disabled
          variant="ghost"
        >
          <HugeiconsIcon className="size-4" icon={ArrowLeft01Icon} />
          <span className="hidden sm:inline">Previous</span>
        </Button>
      )}

      <div className="flex items-center gap-1">
        {pageNumbers.map((page) => {
          if (typeof page === "string") {
            return (
              <span
                className="flex size-10 items-center justify-center text-muted-foreground"
                key={page}
              >
                ...
              </span>
            );
          }

          if (page === currentPage) {
            return (
              <Button
                className="pointer-events-none min-w-10 cursor-default"
                key={page}
                variant="default"
              >
                {page}
              </Button>
            );
          }

          return (
            <Button
              className="min-w-10 cursor-default"
              key={page}
              render={<a href={createPageUrl(page)} />}
              variant="ghost"
            >
              {page}
            </Button>
          );
        })}
      </div>

      {hasNextPage ? (
        <Button
          className="cursor-default gap-1.5 text-foreground"
          render={<a href={createPageUrl(currentPage + 1)} />}
          variant="ghost"
        >
          <span className="hidden sm:inline">Next</span>
          <HugeiconsIcon className="size-4" icon={ArrowRight01Icon} />
        </Button>
      ) : (
        <Button
          className="pointer-events-auto cursor-not-allowed gap-1.5 text-foreground"
          disabled
          variant="ghost"
        >
          <span className="hidden sm:inline">Next</span>
          <HugeiconsIcon className="size-4" icon={ArrowRight01Icon} />
        </Button>
      )}
    </nav>
  );
};

export default Pagination;
