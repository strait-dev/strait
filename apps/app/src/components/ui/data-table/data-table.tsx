import { UserMultiple02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@strait/ui/components/table.tsx";
import { cn } from "@strait/ui/utils/index.ts";
import { flexRender, type Table as TanstackTable } from "@tanstack/react-table";
import type * as React from "react";
import FiltersEmptyState from "@/components/common/filters-empty-state.tsx";
import TableEmptyState from "@/components/common/table-empty-state.tsx";
import { DataTablePagination } from "./data-table-pagination.tsx";

const DEFAULT_EMPTY_FILTER_STATE = (
  <FiltersEmptyState
    description="No results found for the applied filters. Try adjusting the filters."
    icon="search"
    title="No results found"
  />
);

const DEFAULT_EMPTY_STATE = (
  <TableEmptyState
    buttonText="Create"
    description="There is no data available for display."
    href="/app/customers/add"
    icon={
      <HugeiconsIcon
        className="size-6 text-primary"
        icon={UserMultiple02Icon}
      />
    }
    title="No data found"
  />
);

type DataTableProps<TData> = {
  table: TanstackTable<TData>;
  floatingBar?: React.ReactNode | null;
  emptyFilterState?: React.ReactNode | null;
  emptyState: React.ReactNode | null;
};

export const DataTable = <TData,>({
  table,
  floatingBar = null,
  emptyFilterState,
  emptyState,
}: DataTableProps<TData>) => {
  const rows = table.getRowModel().rows;
  const hasFilters =
    table.getState().columnFilters.length > 0 ||
    !!table.getState().globalFilter;

  const resolvedEmptyFilterState =
    emptyFilterState || DEFAULT_EMPTY_FILTER_STATE;
  const resolvedEmptyState = emptyState || DEFAULT_EMPTY_STATE;

  return (
    <div className="flex w-full flex-col gap-2.5 overflow-auto">
      <div className="overflow-x-auto rounded-lg border border-border/70">
        <div className="relative">
          <Table className="min-w-[1200px]">
            <TableHeader className="border-0">
              {table.getHeaderGroups().map((headerGroup) => (
                <TableRow
                  className="h-[45px] border-border/70 border-b hover:bg-transparent"
                  key={headerGroup.id}
                >
                  {headerGroup.headers.map((header) => {
                    const isSelectColumn = header.column.id === "select";
                    const isIdColumn = header.column.id === "id";
                    const isActionsColumn = header.column.id === "actions";

                    return (
                      <TableHead
                        className={cn(
                          "border-0 px-4 py-3",
                          // Midday-style width constraints
                          isSelectColumn ? "w-[50px] min-w-[50px]" : "",
                          isIdColumn ? "w-[120px]" : "",
                          isActionsColumn ? "w-[100px]" : ""
                        )}
                        colSpan={header.colSpan}
                        key={header.id}
                      >
                        {header.isPlaceholder
                          ? null
                          : flexRender(
                              header.column.columnDef.header,
                              header.getContext()
                            )}
                      </TableHead>
                    );
                  })}
                </TableRow>
              ))}
            </TableHeader>
            <TableBody className="border-0">
              {rows.length ? (
                rows.map((row) => (
                  <TableRow
                    className="transition-colors duration-150 hover:bg-muted/40 data-[state=selected]:bg-primary/5"
                    data-state={row.getIsSelected() && "selected"}
                    key={row.id}
                  >
                    {row.getVisibleCells().map((cell) => {
                      const isSelectColumn = cell.column.id === "select";
                      const isIdColumn = cell.column.id === "id";
                      const isActionsColumn = cell.column.id === "actions";

                      return (
                        <TableCell
                          className={cn(
                            "border-0 px-4 py-3",
                            // Midday-style width constraints
                            isSelectColumn ? "w-[50px] min-w-[50px]" : "",
                            isIdColumn ? "w-[120px]" : "",
                            isActionsColumn ? "w-[100px]" : ""
                          )}
                          key={cell.id}
                        >
                          {flexRender(
                            cell.column.columnDef.cell,
                            cell.getContext()
                          )}
                        </TableCell>
                      );
                    })}
                  </TableRow>
                ))
              ) : (
                <TableRow>
                  <TableCell
                    className="h-24 text-center"
                    colSpan={table.getAllColumns().length}
                  >
                    {hasFilters
                      ? (resolvedEmptyFilterState ?? null)
                      : (resolvedEmptyState ?? null)}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      </div>

      <DataTablePagination table={table} />

      {table.getFilteredSelectedRowModel().rows.length > 0 && floatingBar ? (
        <div className="fixed inset-x-0 bottom-4 z-50 mx-auto w-fit">
          {floatingBar}
        </div>
      ) : null}
    </div>
  );
};
