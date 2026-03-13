import { ArrowDown01Icon, ArrowUp01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button.tsx";
import { Checkbox } from "@strait/ui/components/checkbox.tsx";
import { cn } from "@strait/ui/utils/index.ts";
import { useCallback } from "react";

interface DataTableColumnHeaderProps
  extends React.HTMLAttributes<HTMLDivElement> {
  checked?: boolean;
  indeterminate?: boolean;
  isCheckbox?: boolean;
  isSortable?: boolean;
  onCheckedChange?: (value: boolean) => void;
  onSort?: (direction: "asc" | "desc" | null) => void;
  sortDirection?: "asc" | "desc" | null;
  title: string;
}

export const DataTableColumnHeader = ({
  title,
  className,
  isSortable = false,
  sortDirection = null,
  onSort,
  isCheckbox = false,
  checked = false,
  indeterminate = false,
  onCheckedChange,
  ...props
}: DataTableColumnHeaderProps) => {
  const handleSort = useCallback(() => {
    if (!(onSort && isSortable)) {
      return;
    }

    if (sortDirection === "asc") {
      // If currently ascending, switch to descending
      onSort("desc");
    } else if (sortDirection === "desc") {
      // If currently descending, clear sort
      onSort(null);
    } else {
      // If not sorted on this column, set to ascending
      onSort("asc");
    }
  }, [sortDirection, onSort, isSortable]);

  if (isCheckbox) {
    return (
      <div className={cn(className)} {...props}>
        <Checkbox
          checked={checked}
          indeterminate={indeterminate}
          onCheckedChange={onCheckedChange}
        />
      </div>
    );
  }

  if (isSortable) {
    return (
      <div className={cn(className)} {...props}>
        <Button
          className="space-x-2 p-0 hover:bg-transparent"
          onClick={handleSort}
          variant="ghost"
        >
          <span className="font-medium text-foreground text-sm">{title}</span>
          {sortDirection === "asc" && (
            <HugeiconsIcon
              className="size-4 text-foreground"
              icon={ArrowDown01Icon}
            />
          )}
          {sortDirection === "desc" && (
            <HugeiconsIcon
              className="size-4 text-foreground"
              icon={ArrowUp01Icon}
            />
          )}
        </Button>
      </div>
    );
  }

  return (
    <div className={cn(className)} {...props}>
      <span className="font-medium text-foreground text-sm">{title}</span>
    </div>
  );
};
