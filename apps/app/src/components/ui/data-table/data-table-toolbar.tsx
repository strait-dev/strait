import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@strait/ui/components/dropdown-menu";
import { Input } from "@strait/ui/components/input";
import { Separator } from "@strait/ui/components/separator";
import type { Table } from "@tanstack/react-table";
import { useCallback, useEffect, useRef, useState } from "react";
import { FilterIcon, SearchIcon, XCircleIcon } from "@/lib/icons";
import type { DataTableFilterField } from "@/types/index";
import { DataTableViewOptions } from "./data-table-view-options";

const SEARCH_DEBOUNCE_MS = 150;

type DataTableToolbarProps<TData> = {
  table: Table<TData>;
  filterFields?: DataTableFilterField<TData>[];
};

const EMPTY_FILTER_FIELDS: DataTableToolbarProps<unknown>["filterFields"] = [];

export function DataTableToolbar<TData>({
  table,
  filterFields = EMPTY_FILTER_FIELDS,
}: DataTableToolbarProps<TData>) {
  const isFiltered = table.getState().columnFilters.length > 0;

  const searchFields = (filterFields ?? []).filter((field) => !field.options);
  const facetedFields = (filterFields ?? []).filter((field) => field.options);

  const primarySearch = searchFields[0];

  const columnFilterValue =
    (table.getColumn(String(primarySearch?.id))?.getFilterValue() as string) ??
    "";
  const [searchValue, setSearchValue] = useState(columnFilterValue);
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(null);

  useEffect(() => {
    setSearchValue(columnFilterValue);
  }, [columnFilterValue]);

  const handleSearchChange = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      const value = event.target.value;
      setSearchValue(value);

      if (debounceRef.current) {
        clearTimeout(debounceRef.current);
      }

      debounceRef.current = setTimeout(() => {
        table.getColumn(String(primarySearch?.id))?.setFilterValue(value);
      }, SEARCH_DEBOUNCE_MS);
    },
    [table, primarySearch?.id]
  );

  useEffect(() => {
    return () => {
      if (debounceRef.current) {
        clearTimeout(debounceRef.current);
      }
    };
  }, []);

  const activeFilterCount = table
    .getState()
    .columnFilters.filter(
      (f) => Array.isArray(f.value) && f.value.length > 0
    ).length;

  return (
    <div className="flex items-center justify-between pb-1">
      <div className="flex flex-1 items-center gap-2">
        {primarySearch ? (
          <div className="relative">
            <HugeiconsIcon
              aria-hidden="true"
              className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
              icon={SearchIcon}
            />
            <Input
              className="h-8 w-full pl-8 sm:w-96 lg:w-128"
              onChange={handleSearchChange}
              placeholder={
                primarySearch.placeholder ??
                `Filter ${primarySearch.label.toLowerCase()}...`
              }
              value={searchValue}
            />
          </div>
        ) : null}
        {isFiltered ? (
          <Button
            className="px-2 text-muted-foreground lg:px-3"
            onClick={() => table.resetColumnFilters()}
            variant="ghost"
          >
            Reset
            <HugeiconsIcon
              aria-hidden="true"
              className="ml-1 size-4"
              icon={XCircleIcon}
            />
          </Button>
        ) : null}
      </div>
      <div className="flex items-center gap-2">
        {facetedFields.length > 0 ? (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={<Button aria-label="Filter results" variant="outline" />}
            >
              <HugeiconsIcon
                aria-hidden="true"
                className="size-4"
                icon={FilterIcon}
              />
              Filters
              {activeFilterCount > 0 ? (
                <>
                  <Separator className="mx-1 h-4" orientation="vertical" />
                  <Badge variant="secondary-light">{activeFilterCount}</Badge>
                </>
              ) : null}
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-52">
              {facetedFields.map((field) => {
                const column = table.getColumn(String(field.id));
                if (!(column && field.options)) {
                  return null;
                }
                const selectedValues = new Set(
                  column.getFilterValue() as string[]
                );
                return (
                  <DropdownMenuSub key={String(field.id)}>
                    <DropdownMenuSubTrigger>
                      {field.label}
                      {selectedValues.size > 0 ? (
                        <Badge className="ml-auto" variant="secondary-light">
                          {selectedValues.size}
                        </Badge>
                      ) : null}
                    </DropdownMenuSubTrigger>
                    <DropdownMenuSubContent>
                      {field.options.map((option) => (
                        <DropdownMenuCheckboxItem
                          checked={selectedValues.has(option.value)}
                          key={option.value}
                          onCheckedChange={(checked) => {
                            if (checked) {
                              selectedValues.add(option.value);
                            } else {
                              selectedValues.delete(option.value);
                            }
                            const filterValues = Array.from(selectedValues);
                            column.setFilterValue(
                              filterValues.length > 0 ? filterValues : undefined
                            );
                          }}
                        >
                          {option.label}
                        </DropdownMenuCheckboxItem>
                      ))}
                    </DropdownMenuSubContent>
                  </DropdownMenuSub>
                );
              })}
              {activeFilterCount > 0 ? (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuCheckboxItem
                    checked={false}
                    className="justify-center text-center text-muted-foreground"
                    onCheckedChange={() => {
                      for (const field of facetedFields) {
                        table
                          .getColumn(String(field.id))
                          ?.setFilterValue(undefined);
                      }
                    }}
                  >
                    Clear all filters
                  </DropdownMenuCheckboxItem>
                </>
              ) : null}
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null}
        <DataTableViewOptions table={table} />
      </div>
    </div>
  );
}
