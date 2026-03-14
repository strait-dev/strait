"use client";

import {
  Add01Icon,
  ArrowDown01Icon,
  Tick01Icon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import React from "react";

import { cn } from "../utils/index";
import { Button } from "./button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "./command";
import { Label } from "./label";
import { Popover, PopoverContent, PopoverTrigger } from "./popover";

export type SelectOptionWithSearchAndButton = {
  value: string;
  label: string;
};

type SelectWithSearchAndButtonProps = {
  options: SelectOptionWithSearchAndButton[];
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
  buttonText?: string;
  onButtonClick?: () => void;
  buttonIcon?: React.ReactNode;
};

export function SelectWithSearchAndButton({
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
  buttonText = "Add new",
  onButtonClick,
  buttonIcon = (
    <HugeiconsIcon
      aria-hidden="true"
      className="-ms-2 me-2 size-4 opacity-60"
      icon={Add01Icon}
    />
  ),
}: SelectWithSearchAndButtonProps) {
  const [open, setOpen] = React.useState(false);
  const [selectedValue, setSelectedValue] = React.useState(value || "");

  React.useEffect(() => {
    if (value !== undefined) {
      setSelectedValue(value);
    }
  }, [value]);

  const handleSelect = React.useCallback(
    (currentValue: string) => {
      const newValue = currentValue === selectedValue ? "" : currentValue;
      setSelectedValue(newValue);
      onValueChange?.(newValue);
      setOpen(false);
    },
    [selectedValue, onValueChange]
  );

  return (
    <div className={cn("flex flex-col gap-2", className)}>
      {label ? (
        <Label className={cn(error ? "text-destructive" : null)} htmlFor={name}>
          {label}
          {required ? <span className="ml-1 text-destructive">*</span> : null}
        </Label>
      ) : null}
      <Popover onOpenChange={setOpen} open={open}>
        <PopoverTrigger
          render={
            <Button
              aria-expanded={open}
              className={cn(
                "w-full justify-between bg-background px-3 font-normal outline-offset-0 hover:bg-background focus-visible:border-ring focus-visible:outline-[3px] focus-visible:outline-ring/20",
                !selectedValue && "text-muted-foreground",
                error ? "border-destructive" : null,
                disabled ? "cursor-not-allowed opacity-50" : null
              )}
              disabled={disabled}
              id={name}
              variant="outline"
            />
          }
        >
          <span className="truncate">
            {selectedValue
              ? (options.find((option) => option.value === selectedValue)
                  ?.label ?? "")
              : (placeholder ?? "")}
          </span>
          <HugeiconsIcon
            aria-hidden="true"
            className="size-4 shrink-0 text-muted-foreground/80"
            icon={ArrowDown01Icon}
          />
        </PopoverTrigger>
        <PopoverContent
          align="start"
          className="w-full min-w-[var(--radix-popper-anchor-width)] border-input p-0"
        >
          <Command>
            <CommandInput placeholder={searchPlaceholder} />
            <CommandList>
              <CommandEmpty>{noResultsText}</CommandEmpty>
              <CommandGroup>
                {options.map((option) => (
                  <CommandItem
                    key={option.value}
                    onSelect={handleSelect}
                    value={option.value}
                  >
                    {option.label}
                    {selectedValue === option.value ? (
                      <HugeiconsIcon
                        className="ml-auto size-4"
                        icon={Tick01Icon}
                      />
                    ) : null}
                  </CommandItem>
                ))}
              </CommandGroup>
              {onButtonClick ? (
                <>
                  <CommandSeparator />
                  <CommandGroup>
                    <Button
                      className="w-full justify-start font-normal"
                      onClick={() => {
                        onButtonClick();
                        setOpen(false);
                      }}
                      variant="ghost"
                    >
                      {buttonIcon}
                      {buttonText}
                    </Button>
                  </CommandGroup>
                </>
              ) : null}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  );
}
