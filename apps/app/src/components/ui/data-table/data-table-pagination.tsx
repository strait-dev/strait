import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import type { Table } from "@tanstack/react-table";
import {
  ChevronLeftIcon,
  ChevronRightIcon,
  ChevronsLeftIcon,
  ChevronsRightIcon,
} from "@/lib/icons";

const PAGE_SIZE_TEN = 10;
const PAGE_SIZE_TWENTY = 20;
const PAGE_SIZE_THIRTY = 30;
const PAGE_SIZE_FOURTY = 40;
const PAGE_SIZE_FIFTY = 50;

const PAGE_SIZE_OPTIONS: number[] = [
  PAGE_SIZE_TEN,
  PAGE_SIZE_TWENTY,
  PAGE_SIZE_THIRTY,
  PAGE_SIZE_FOURTY,
  PAGE_SIZE_FIFTY,
];

type DataTablePaginationProps<TData> = {
  table: Table<TData>;
};

export const DataTablePagination = <TData,>({
  table,
}: DataTablePaginationProps<TData>) => {
  const pageSizeOptions = PAGE_SIZE_OPTIONS;
  const pageSize = table.getState().pagination.pageSize;
  const localPageSize = String(pageSize);

  return (
    <div className="flex w-full flex-col gap-2 self-center sm:flex-row sm:items-center sm:justify-between">
      <div className="flex-1 text-muted-foreground text-sm tabular-nums">
        {table.getFilteredSelectedRowModel().rows.length} of{" "}
        {table.getFilteredRowModel().rows.length} row(s) selected
      </div>

      <div className="flex items-center justify-between gap-4">
        <p className="text-muted-foreground text-sm">Results per page</p>
        <Select
          onValueChange={(value) => {
            if (!value) {
              return;
            }
            table.setPageSize(Number(value));
          }}
          value={localPageSize}
        >
          <SelectTrigger className="w-[70px]">
            <SelectValue placeholder={localPageSize} />
          </SelectTrigger>
          <SelectContent side="top">
            {pageSizeOptions.map((pageSize: number) => (
              <SelectItem key={pageSize} value={`${pageSize}`}>
                {pageSize}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="flex justify-between gap-4">
        <div className="flex items-center justify-center text-muted-foreground text-sm tabular-nums">
          Page {table.getState().pagination.pageIndex + 1} of{" "}
          {table.getPageCount()}
        </div>
        <div className="flex items-center space-x-2">
          <Button
            aria-label="Go to first page"
            className="hidden size-8 p-0 lg:flex"
            disabled={!table.getCanPreviousPage()}
            onClick={() => table.setPageIndex(0)}
            variant="outline"
          >
            <HugeiconsIcon
              aria-hidden="true"
              className="size-4"
              icon={ChevronsLeftIcon}
            />
          </Button>
          <Button
            aria-label="Go to previous page"
            className="size-8"
            disabled={!table.getCanPreviousPage()}
            onClick={() => table.previousPage()}
            size="icon"
            variant="outline"
          >
            <HugeiconsIcon
              aria-hidden="true"
              className="size-4"
              icon={ChevronLeftIcon}
            />
          </Button>
          <Button
            aria-label="Go to next page"
            className="size-8"
            disabled={!table.getCanNextPage()}
            onClick={() => table.nextPage()}
            size="icon"
            variant="outline"
          >
            <HugeiconsIcon
              aria-hidden="true"
              className="size-4"
              icon={ChevronRightIcon}
            />
          </Button>
          <Button
            aria-label="Go to last page"
            className="hidden size-8 lg:flex"
            disabled={!table.getCanNextPage()}
            onClick={() => table.setPageIndex(table.getPageCount() - 1)}
            size="icon"
            variant="outline"
          >
            <HugeiconsIcon
              aria-hidden="true"
              className="size-4"
              icon={ChevronsRightIcon}
            />
          </Button>
        </div>
      </div>
    </div>
  );
};
