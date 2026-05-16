import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Checkbox } from "@strait/ui/components/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@strait/ui/components/dropdown-menu";
import type { ColumnDef, Row } from "@tanstack/react-table";
import { MoreVerticalIcon } from "@/lib/icons";

export function createSelectColumn<T>(): ColumnDef<T> {
  return {
    id: "select",
    header: ({ table }) => (
      <Checkbox
        aria-label="Select all"
        checked={table.getIsAllPageRowsSelected()}
        indeterminate={
          table.getIsSomePageRowsSelected() && !table.getIsAllPageRowsSelected()
        }
        onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
      />
    ),
    cell: ({ row }) => (
      // biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noNoninteractiveElementInteractions lint/a11y/noStaticElementInteractions: stopPropagation isolates checkbox from row-click delegation
      <div onClick={(e) => e.stopPropagation()}>
        <Checkbox
          aria-label="Select row"
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
        />
      </div>
    ),
    enableSorting: false,
    enableHiding: false,
  };
}

type ActionItem<T> = {
  label: string;
  icon?: any;
  onClick: (row: Row<T>) => void;
  variant?: "default" | "destructive";
};

export function createActionsColumn<T>(actions: ActionItem<T>[]): ColumnDef<T> {
  return {
    id: "actions",
    cell: ({ row }) => (
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button aria-label="Row actions" size="icon" variant="ghost" />
          }
        >
          <HugeiconsIcon className="size-4" icon={MoreVerticalIcon} />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {actions.map((action) => (
            <DropdownMenuItem
              key={action.label}
              onClick={() => action.onClick(row)}
            >
              {action.icon && (
                <HugeiconsIcon className="mr-2 size-3.5" icon={action.icon} />
              )}
              {action.label}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    ),
    enableSorting: false,
    enableHiding: false,
  };
}
