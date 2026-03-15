"use client";

import { ArrowDown01Icon, ArrowUp01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import type React from "react";
import { Button, Group, Input, NumberField } from "react-aria-components";
import { cn } from "../utils/index";

export type NumberInputWithChevronsProps = Omit<
  React.InputHTMLAttributes<HTMLInputElement>,
  "value" | "onChange" | "defaultValue"
> & {
  defaultValue?: number;
  name: string;
  value?: number;
  onChange?: (value: number) => void;
  min?: number;
  max?: number;
  step?: number;
  disabled?: boolean;
  label?: string;
  formatOptions?: Intl.NumberFormatOptions;
  containerClassName?: string;
};

function NumberInputWithChevrons({
  defaultValue,
  name,
  value,
  onChange,
  min,
  max,
  step = 1,
  disabled,
  label,
  formatOptions,
  className,
  containerClassName,
  ...props
}: NumberInputWithChevronsProps) {
  return (
    <NumberField
      aria-label={label || `${name} input`}
      data-slot="number-input-with-chevrons"
      defaultValue={defaultValue}
      formatOptions={formatOptions}
      isDisabled={disabled}
      maxValue={max}
      minValue={min}
      onChange={onChange}
      step={step}
      value={value}
    >
      <Group
        className={cn(
          "relative inline-flex h-9 w-full items-center overflow-hidden whitespace-nowrap rounded-md border border-input bg-input/20 text-sm shadow-black/5 shadow-xs ring-offset-background transition-shadow data-focus-within:border-ring data-disabled:opacity-50 data-focus-within:outline-hidden data-focus-within:ring-[3px] data-focus-within:ring-ring/50 data-focus-within:ring-offset-2 dark:bg-input/30",
          containerClassName
        )}
        data-slot="input-group"
      >
        <Input
          aria-label={label || `${name} input`}
          className={cn(
            "min-w-0 flex-1 bg-transparent px-3 py-1 text-foreground tabular-nums focus:outline-hidden",
            className
          )}
          data-slot="input"
          {...props}
        />
        <div
          className="flex h-full w-8 shrink-0 flex-col border-input border-l"
          data-slot="chevrons-container"
        >
          <Button
            aria-label="Increase value"
            className="flex h-1/2 w-8 items-center justify-center bg-transparent text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50"
            data-slot="increment-button"
            slot="increment"
          >
            <HugeiconsIcon
              aria-hidden="true"
              className="size-4"
              icon={ArrowUp01Icon}
            />
          </Button>
          <Button
            aria-label="Decrease value"
            className="flex h-1/2 w-8 items-center justify-center border-input border-t bg-transparent text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50"
            data-slot="decrement-button"
            slot="decrement"
          >
            <HugeiconsIcon
              aria-hidden="true"
              className="size-4"
              icon={ArrowDown01Icon}
            />
          </Button>
        </div>
      </Group>
    </NumberField>
  );
}

NumberInputWithChevrons.displayName = "NumberInputWithChevrons";

export { NumberInputWithChevrons };
