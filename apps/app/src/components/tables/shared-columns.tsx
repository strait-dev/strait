import { HugeiconsIcon } from "@hugeicons/react";
import { Button, buttonVariants } from "@strait/ui/components/button";
import { Checkbox } from "@strait/ui/components/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@strait/ui/components/dropdown-menu";
import type { ColumnDef, Row } from "@tanstack/react-table";
import type { ReactNode } from "react";
import { Fragment } from "react";
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
  hidden?: boolean;
  onClick?: (row: Row<T>) => void;
  render?: (row: Row<T>) => ReactNode;
  variant?: "default" | "destructive";
};

export function createActionsColumn<T>(actions: ActionItem<T>[]): ColumnDef<T> {
  const visibleActions = actions.filter((action) => !action.hidden);

  return {
    id: "actions",
    cell: ({ row }) => (
      // biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noNoninteractiveElementInteractions lint/a11y/noStaticElementInteractions: stopPropagation isolates row actions from row-click delegation
      <div
        className="flex items-center justify-end gap-1"
        data-no-row-click
        onClick={(event) => event.stopPropagation()}
      >
        {visibleActions
          .filter((action) => !action.render)
          .map((action) => (
            <button
              aria-label={action.label}
              className={buttonVariants({
                size: "icon-sm",
                variant:
                  action.variant === "destructive" ? "destructive" : "ghost",
              })}
              key={action.label}
              onClick={(event) => {
                event.stopPropagation();
                if (action.variant !== "destructive" || event.detail === 0) {
                  action.onClick?.(row);
                }
              }}
              onClickCapture={(event) => {
                event.stopPropagation();
                if (action.variant !== "destructive" || event.detail === 0) {
                  action.onClick?.(row);
                }
              }}
              onMouseDown={(event) => {
                event.stopPropagation();
                if (action.variant === "destructive") {
                  window.setTimeout(() => action.onClick?.(row), 50);
                }
              }}
              onMouseUp={(event) => {
                event.stopPropagation();
                if (action.variant === "destructive") {
                  window.setTimeout(() => action.onClick?.(row), 0);
                }
              }}
              onPointerDownCapture={(event) => {
                event.stopPropagation();
                if (action.variant === "destructive") {
                  window.setTimeout(() => action.onClick?.(row), 50);
                }
              }}
              onPointerUp={(event) => {
                event.stopPropagation();
                if (action.variant === "destructive") {
                  window.setTimeout(() => action.onClick?.(row), 0);
                }
              }}
              type="button"
            >
              {action.icon ? (
                <HugeiconsIcon
                  aria-hidden="true"
                  className="size-3.5"
                  icon={action.icon}
                />
              ) : (
                <span className="text-xs">{action.label.slice(0, 1)}</span>
              )}
            </button>
          ))}
        <DropdownMenu>
          <DropdownMenuTrigger render={<Button size="icon" variant="ghost" />}>
            <span className="sr-only">Row actions</span>
            <HugeiconsIcon
              aria-hidden="true"
              className="size-4"
              icon={MoreVerticalIcon}
            />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {visibleActions.map((action) => (
              <Fragment key={action.label}>
                {action.render ? (
                  action.render(row)
                ) : (
                  <DropdownMenuItem
                    onClick={() => action.onClick?.(row)}
                    variant={action.variant}
                  >
                    {action.icon && (
                      <HugeiconsIcon
                        className="mr-2 size-3.5"
                        icon={action.icon}
                      />
                    )}
                    {action.label}
                  </DropdownMenuItem>
                )}
              </Fragment>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    ),
    enableSorting: false,
    enableHiding: false,
  };
}
