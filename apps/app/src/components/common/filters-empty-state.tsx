import { HugeiconsIcon } from "@hugeicons/react";
import { SearchIcon, XCircleIcon } from "@/lib/icons";

type FiltersEmptyStateProps = {
  title?: string;
  description?: string;
  icon?: "search" | "x";
};

const FiltersEmptyState = ({
  title = "No results found",
  description = "No results found for your search. Try changing the filters or search for another term.",
  icon = "search",
}: FiltersEmptyStateProps) => (
  <div className="flex h-[300px] flex-col items-center justify-center gap-4 rounded-xl border border-muted-foreground/10 border-dashed p-8 text-center">
    <div>
      <div className="flex aspect-square h-14 items-center justify-center rounded-xl bg-muted/70">
        {icon === "search" ? (
          <HugeiconsIcon
            className="size-6 text-muted-foreground"
            icon={SearchIcon}
          />
        ) : (
          <HugeiconsIcon
            className="size-6 text-muted-foreground"
            icon={XCircleIcon}
          />
        )}
      </div>
    </div>

    <div className="flex max-w-xs flex-col items-center gap-2 text-center">
      <h2 className="text-balance font-normal text-lg text-secondary-foreground tracking-tight">
        {title}
      </h2>
      <p className="text-pretty text-muted-foreground text-sm">{description}</p>
    </div>
  </div>
);

export default FiltersEmptyState;
