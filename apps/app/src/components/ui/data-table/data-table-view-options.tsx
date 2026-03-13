import { Settings05Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button.tsx";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@strait/ui/components/dropdown-menu.tsx";
import type { Table } from "@tanstack/react-table";

const CAMEL_CASE_BOUNDARY = /([A-Z])/g;
const FIRST_CHAR = /^./;

function formatColumnLabel(id: string): string {
  return id
    .replace(CAMEL_CASE_BOUNDARY, " $1")
    .replace(FIRST_CHAR, (c) => c.toUpperCase())
    .trim();
}

type DataTableViewOptionsProps<TData> = {
  table: Table<TData>;
};

export function DataTableViewOptions<TData>({
  table,
}: DataTableViewOptionsProps<TData>) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            aria-label="Toggle columns"
            className="ml-auto hidden lg:flex"
            variant="outline"
          />
        }
      >
        <HugeiconsIcon
          aria-hidden="true"
          className="size-4"
          icon={Settings05Icon}
        />
        View
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-44">
        <DropdownMenuGroup>
          <DropdownMenuLabel>Toggle columns</DropdownMenuLabel>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        {table
          .getAllColumns()
          .filter(
            (column) =>
              typeof column.accessorFn !== "undefined" && column.getCanHide()
          )
          .map((column) => (
            <DropdownMenuCheckboxItem
              checked={column.getIsVisible()}
              key={column.id}
              onCheckedChange={(value) => column.toggleVisibility(!!value)}
            >
              {formatColumnLabel(column.id)}
            </DropdownMenuCheckboxItem>
          ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
