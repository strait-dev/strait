"use client";

import { Calendar03Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useId } from "react";

import { cn } from "../utils/index.ts";
import { Button } from "./button.tsx";
import { Calendar } from "./calendar.tsx";
import { Popover, PopoverContent, PopoverTrigger } from "./popover.tsx";

type DatePickerProps = {
  value?: Date;
  onChange?: (date: Date | undefined) => void;
  disabled?: boolean;
  isRequired?: boolean;
  label?: string;
  className?: string;
  error?: boolean;
};

function DatePicker({
  value,
  onChange,
  disabled,
  isRequired,
  label,
  className,
  error,
}: DatePickerProps) {
  const datePickerId = useId();
  return (
    <div className={cn("grid w-full items-center gap-1.5", className)}>
      {label ? (
        <label
          className="font-medium text-sm leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70"
          htmlFor={datePickerId}
        >
          {label}
          {isRequired ? <span className="text-destructive">*</span> : null}
        </label>
      ) : null}
      <Popover>
        <PopoverTrigger
          render={
            <Button
              className={cn(
                "h-9 w-full justify-start rounded-md! bg-transparent px-3 text-left font-normal text-sm shadow-xs hover:bg-transparent",
                !value && "text-muted-foreground/70",
                error ? "border-destructive" : null
              )}
              disabled={disabled}
              id={datePickerId}
              variant="outline"
            />
          }
        >
          <HugeiconsIcon
            className="mr-2 size-4 opacity-60"
            icon={Calendar03Icon}
          />
          {value ? (
            value.toLocaleDateString()
          ) : (
            <span className="font-normal">Select a date</span>
          )}
        </PopoverTrigger>
        <PopoverContent className="w-auto p-0">
          <Calendar
            initialFocus
            mode="single"
            onSelect={onChange}
            selected={value}
          />
        </PopoverContent>
      </Popover>
    </div>
  );
}

DatePicker.displayName = "DatePicker";

export { DatePicker, type DatePickerProps };
