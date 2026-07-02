import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Pagination,
  PaginationContent,
  PaginationItem,
} from "@strait/ui/components/pagination";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import type { Table } from "@tanstack/react-table";
import { useIsHydrated } from "@/hooks/use-is-hydrated";
import { ChevronLeftIcon, ChevronRightIcon } from "@/lib/icons";

const PAGE_SIZE_OPTIONS = [10, 20, 30, 40, 50] as const;

export type CursorPaginationProps = {
  pageSize: number;
  hasMore: boolean;
  canGoBack: boolean;
  onNext: () => void;
  onPrev: () => void;
  onPageSizeChange: (n: number) => void;
};

type CursorPaginationPropsWithTable<TData> = {
  cursor: CursorPaginationProps;
  table: Table<TData>;
};

export const CursorPagination = <TData,>({
  cursor,
  table,
}: CursorPaginationPropsWithTable<TData>) => {
  const isHydrated = useIsHydrated();
  const selectedCount = table.getFilteredSelectedRowModel().rows.length;
  const visibleCount = table.getRowModel().rows.length;
  const pageSize = String(cursor.pageSize);

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
          value={pageSize}
        >
          <SelectTrigger className="w-[70px]">
            <SelectValue placeholder={pageSize} />
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

      <Pagination className="w-auto" size="default">
        <PaginationContent>
          <PaginationItem>
            <Button
              aria-label="Go to previous page"
              disabled={!(isHydrated && cursor.canGoBack)}
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
          </PaginationItem>
          <PaginationItem>
            <Button
              aria-label="Go to next page"
              disabled={!(isHydrated && cursor.hasMore)}
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
          </PaginationItem>
        </PaginationContent>
      </Pagination>
    </div>
  );
};
