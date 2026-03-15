"use client";

import { ArrowDown01Icon, Tick01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Virtuoso } from "react-virtuoso";

import { cn } from "../utils/index";
import { Button } from "./button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
} from "./command";
import { Label } from "./label";
import { Popover, PopoverContent, PopoverTrigger } from "./popover";

const TIMEOUT = 50;

export type SelectOption = {
  value: string;
  label: string;
  data?: any;
};

type SelectWithSearchProps = {
  options: SelectOption[];
  value?: string;
  onValueChange?: (value: string) => void;
  placeholder?: string;
  label?: string;
  searchPlaceholder?: string;
  noResultsText?: string;
  className?: string;
  disabled?: boolean;
  error?: boolean;
  required?: boolean;
  name?: string;
  onSearchChange?: (search: string) => void;
  id?: string;
  onOpenChange?: (open: boolean) => void;
  onEndReached?: () => void;
  renderOption?: (option: SelectOption, isSelected: boolean) => React.ReactNode;
  renderSelectedOption?: (option: SelectOption) => React.ReactNode;
  renderLoading?: () => React.ReactNode;
  renderEmpty?: () => React.ReactNode;
  isLoading?: boolean;
  isFetchingNextPage?: boolean;
  size?: "default" | "sm" | "lg" | "icon";
};

export function SelectWithSearch({
  options,
  value,
  onValueChange,
  placeholder = "Select an option",
  label,
  searchPlaceholder = "Search...",
  noResultsText = "No results found.",
  className,
  disabled = false,
  error = false,
  required = false,
  name,
  onSearchChange,
  id,
  onOpenChange,
  onEndReached,
  renderOption,
  renderSelectedOption,
  renderLoading,
  renderEmpty,
  isLoading = false,
  isFetchingNextPage = false,
  size = "default",
}: SelectWithSearchProps) {
  const [open, setOpen] = useState(false);
  const [selectedValue, setSelectedValue] = useState(value || "");
  const [searchInputValue, setSearchInputValue] = useState("");

  const buttonRef = useRef<HTMLButtonElement>(null);
  const virtuosoRef = useRef(null);

  useEffect(() => {
    if (value !== undefined) {
      setSelectedValue(value);
    }
  }, [value]);

  // Scroll to selected item when opening the select
  useEffect(() => {
    if (!(open && selectedValue && virtuosoRef.current)) {
      return;
    }

    // Find the index of the selected item
    const selectedIndex = options.findIndex(
      (option) => option.value === selectedValue
    );

    if (selectedIndex !== -1 && virtuosoRef.current) {
      // Wait for the list to be fully rendered
      setTimeout(() => {
        (virtuosoRef.current as any)?.scrollToIndex({
          index: selectedIndex,
          align: "center",
        });
      }, TIMEOUT);
    }
  }, [open, selectedValue, options]);

  const handleOpenChange = useCallback(
    (newOpen: boolean) => {
      setOpen(newOpen);
      onOpenChange?.(newOpen);
    },
    [onOpenChange]
  );

  const handleSelect = useCallback(
    (currentValue: string) => {
      setSelectedValue(currentValue);
      onValueChange?.(currentValue);
      setOpen(false);
    },
    [onValueChange]
  );

  const handleSearch = useCallback(
    (search: string) => {
      setSearchInputValue(search);
      onSearchChange?.(search);
    },
    [onSearchChange]
  );

  const selectedOption = useMemo(
    () => options.find((option) => option.value === selectedValue),
    [options, selectedValue]
  );

  const buttonContent = useMemo(
    () => (
      <>
        <span
          className={cn(
            "truncate",
            !selectedOption && "text-muted-foreground/70 text-sm"
          )}
        >
          {(() => {
            if (!selectedOption) {
              return placeholder;
            }
            if (renderSelectedOption) {
              return renderSelectedOption(selectedOption);
            }
            return selectedOption.label;
          })()}
        </span>
        <HugeiconsIcon
          aria-hidden="true"
          className="size-4 shrink-0 text-muted-foreground/80"
          icon={ArrowDown01Icon}
        />
      </>
    ),
    [selectedOption, renderSelectedOption, placeholder]
  );

  const buttonClassName = useMemo(
    () =>
      cn(
        "w-full justify-between bg-transparent px-3 font-normal outline-offset-0 hover:bg-transparent focus-visible:border-ring focus-visible:outline-[3px] focus-visible:outline-ring/20",
        !selectedValue && "text-muted-foreground",
        error ? "border-destructive" : undefined,
        disabled ? "cursor-not-allowed opacity-50" : undefined
      ),
    [selectedValue, error, disabled]
  );

  // Item renderer for virtualized list
  const ItemRenderer = useCallback(
    (index: number) => {
      const option = options[index];
      if (!option) {
        return null;
      }

      const isSelected = selectedValue === option.value;

      return (
        <CommandItem
          className={cn("", isSelected ? "bg-primary/10" : null)}
          data-value={option.value}
          key={option.value}
          onSelect={handleSelect}
          value={option.value}
        >
          {renderOption ? (
            renderOption(option, isSelected)
          ) : (
            <>
              {option.label}
              {isSelected ? (
                <HugeiconsIcon
                  aria-hidden="true"
                  className="ml-auto size-4"
                  icon={Tick01Icon}
                />
              ) : null}
            </>
          )}
        </CommandItem>
      );
    },
    [options, selectedValue, handleSelect, renderOption]
  );

  // Footer component for showing loading state
  const FooterComponent = useCallback(() => {
    if (!(isFetchingNextPage && renderLoading)) {
      return null;
    }
    return <div className="p-1">{renderLoading()}</div>;
  }, [isFetchingNextPage, renderLoading]);

  const renderContent = useCallback(() => {
    if (isLoading && renderLoading) {
      return <div className="p-1">{renderLoading()}</div>;
    }

    if (options.length === 0) {
      if (renderEmpty) {
        return (
          <div className="w-full overflow-hidden p-1">{renderEmpty()}</div>
        );
      }
      return <CommandEmpty>{noResultsText}</CommandEmpty>;
    }

    // Calculate dynamic height based on number of items
    // Each item is roughly 40px in height, with a minimum of 50px and maximum of 300px
    const estimatedItemHeight = 40;
    const minHeight = 50; // Minimum height for a single item with some padding
    const maxHeight = 300; // Maximum height as before
    const calculatedHeight = Math.min(
      maxHeight,
      Math.max(
        minHeight,
        Math.min(options.length * estimatedItemHeight, maxHeight)
      )
    );

    // Only include Footer component when actually fetching next page
    const footerComponent =
      isFetchingNextPage && renderLoading ? { Footer: FooterComponent } : {};

    return (
      <CommandGroup className="overflow-visible p-0">
        <Virtuoso
          className="select-virtuoso"
          components={footerComponent}
          data={options}
          endReached={onEndReached}
          increaseViewportBy={200}
          itemContent={(index) => ItemRenderer(index)}
          ref={virtuosoRef}
          style={{ height: `${calculatedHeight}px` }}
        />
      </CommandGroup>
    );
  }, [
    isLoading,
    renderLoading,
    options,
    renderEmpty,
    noResultsText,
    ItemRenderer,
    onEndReached,
    FooterComponent,
    isFetchingNextPage,
  ]);

  const labelElement = useMemo(
    () =>
      label ? (
        <Label
          className={cn(error ? "text-destructive" : null)}
          htmlFor={id || name}
        >
          {label}
          {required ? <span className="ml-1 text-destructive">*</span> : null}
        </Label>
      ) : null,
    [label, id, name, error, required]
  );

  return (
    <div className={cn("flex flex-col gap-2", className)}>
      {labelElement}
      <Popover onOpenChange={handleOpenChange} open={open}>
        <PopoverTrigger
          render={
            <Button
              aria-expanded={open}
              className={buttonClassName}
              disabled={disabled}
              id={id || name}
              ref={buttonRef}
              size={size}
              variant="outline"
            />
          }
        >
          {buttonContent}
        </PopoverTrigger>
        <PopoverContent
          align="start"
          className="w-(--radix-popper-anchor-width) border-input p-0"
          sideOffset={4}
        >
          <Command className="overflow-hidden" shouldFilter={false}>
            <CommandInput
              className="border-0"
              onValueChange={handleSearch}
              placeholder={searchPlaceholder}
              value={searchInputValue}
            />
            <div className="max-h-[300px] overflow-hidden">
              {renderContent()}
            </div>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  );
}
