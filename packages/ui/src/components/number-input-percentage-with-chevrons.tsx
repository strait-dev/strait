"use client";

import { ArrowDown01Icon, ArrowUp01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import type React from "react";
import { Button, Group, Input, NumberField } from "react-aria-components";
import { cn } from "../utils/index";

const MINIMUM_VALUE = 0;
const MAX_VALUE = 100;

export type NumberInputPercentageWithChevronsProps = Omit<
  React.InputHTMLAttributes<HTMLInputElement>,
  "value" | "onChange" | "defaultValue"
> & {
  defaultValue?: number;
  name: string;
  value?: number;
  onChange?: (value: number) => void;
  disabled?: boolean;
  label?: string;
  containerClassName?: string;
};

function NumberInputPercentageWithChevrons({
  defaultValue = 0,
  name,
  value,
  onChange,
  disabled,
  label,
  className,
  containerClassName,
  ...props
}: NumberInputPercentageWithChevronsProps) {
  const displayValue = value ? value / MAX_VALUE : 0;

  const handleChange = (newValue: number) => {
    if (onChange) {
      onChange(newValue * MAX_VALUE);
    }
  };

  return (
    <NumberField
      aria-label={label || `${name} input`}
      data-slot="number-input-percentage-with-chevrons"
      defaultValue={defaultValue}
      formatOptions={{
        style: "percent",
        minimumFractionDigits: 0,
        maximumFractionDigits: 2,
      }}
      isDisabled={disabled}
      maxValue={MAX_VALUE}
      minValue={MINIMUM_VALUE}
      onChange={handleChange}
      step={0.01}
      value={displayValue}
    >
      <Group
        className={cn(
          "relative inline-flex h-9 w-full items-center overflow-hidden whitespace-nowrap rounded-md border border-input bg-input/20 text-sm shadow-black/5 shadow-xs ring-offset-background transition-shadow data-focus-within:border-ring data-disabled:opacity-50 data-focus-within:outline-hidden data-focus-within:ring-[3px] data-focus-within:ring-ring/50 data-focus-within:ring-offset-2 dark:bg-input/30",
          containerClassName
        )}
        data-slot="input-group"
      >
        <Input
          aria-label={label || `${name} percentage input`}
          className={cn(
            "flex-1 bg-transparent px-3 py-1 text-foreground tabular-nums focus:outline-hidden",
            className
          )}
          data-slot="input"
          {...props}
        />
        <div
          className="flex h-[calc(100%+2px)] flex-col"
          data-slot="chevrons-container"
        >
          <Button
            aria-label="Increase percentage"
            className="-me-px flex h-1/2 w-8 flex-1 items-center justify-center border border-input bg-transparent text-muted-foreground/80 text-sm ring-offset-background transition-shadow hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50"
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
            aria-label="Decrease percentage"
            className="-me-px -mt-px flex h-1/2 w-8 flex-1 items-center justify-center border border-input bg-transparent text-muted-foreground/80 text-sm ring-offset-background transition-shadow hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50"
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

NumberInputPercentageWithChevrons.displayName =
  "NumberInputPercentageWithChevrons";

export { NumberInputPercentageWithChevrons };
