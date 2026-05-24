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
import { ChevronLeftIcon, ChevronRightIcon } from "@/lib/icons";

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

export type CursorPaginationProps = {
  pageSize: number;
  hasMore: boolean;
  canGoBack: boolean;
  onNext: () => void;
  onPrev: () => void;
  onPageSizeChange: (n: number) => void;
};

type DataTablePaginationProps<TData> = {
  table: Table<TData>;
  cursorPagination?: CursorPaginationProps;
};

export const DataTablePagination = <TData,>({
  table,
  cursorPagination,
}: DataTablePaginationProps<TData>) => {
  if (cursorPagination) {
    return <CursorPagination cursor={cursorPagination} table={table} />;
  }

  return <ClientPagination table={table} />;
};

const CursorPagination = <TData,>({
  cursor,
  table,
}: {
  cursor: CursorPaginationProps;
  table: Table<TData>;
}) => {
  const selectedCount = table.getFilteredSelectedRowModel().rows.length;
  const visibleCount = table.getRowModel().rows.length;
  const pageSizeStr = String(cursor.pageSize);

  return (
    <div className="flex w-full flex-col gap-2 self-center sm:flex-row sm:items-center sm:justify-between">
      <div className="flex-1 text-muted-foreground text-sm tabular-nums">
        {selectedCount > 0
          ? `${selectedCount} of ${visibleCount} row(s) selected`
          : `${visibleCount} row${visibleCount === 1 ? "" : "s"}`}
      </div>

      <div className="flex items-center justify-between gap-4">
        <p className="text-muted-foreground text-sm">Results per page</p>
        <Select
          onValueChange={(value) => {
            if (!value) {
              return;
            }
            cursor.onPageSizeChange(Number(value));
          }}
          value={pageSizeStr}
        >
          <SelectTrigger className="w-[70px]">
            <SelectValue placeholder={pageSizeStr} />
          </SelectTrigger>
          <SelectContent side="top">
            {PAGE_SIZE_OPTIONS.map((size) => (
              <SelectItem key={size} value={`${size}`}>
                {size}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="flex items-center space-x-2">
        <Button
          aria-label="Go to previous page"
          className="size-8"
          disabled={!cursor.canGoBack}
          onClick={cursor.onPrev}
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
          disabled={!cursor.hasMore}
          onClick={cursor.onNext}
          size="icon"
          variant="outline"
        >
          <HugeiconsIcon
            aria-hidden="true"
            className="size-4"
            icon={ChevronRightIcon}
          />
        </Button>
      </div>
    </div>
  );
};

const ClientPagination = <TData,>({ table }: { table: Table<TData> }) => {
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
            {PAGE_SIZE_OPTIONS.map((size: number) => (
              <SelectItem key={size} value={`${size}`}>
                {size}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="flex items-center space-x-2">
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
      </div>
    </div>
  );
};
