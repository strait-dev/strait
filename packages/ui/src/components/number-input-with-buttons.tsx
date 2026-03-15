"use client";

import { Add01Icon, Cancel01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import type React from "react";
import { Button, Group, Input, NumberField } from "react-aria-components";
import { cn } from "../utils/index";

export type NumberInputWithButtonsProps = Omit<
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
  className?: string;
  containerClassName?: string;
};

function NumberInputWithButtons({
  defaultValue,
  name,
  value,
  onChange,
  min,
  max,
  step = 1,
  disabled,
  label,
  placeholder,
  className,
  containerClassName,
  ...props
}: NumberInputWithButtonsProps) {
  return (
    <NumberField
      aria-label={label || `${name} input`}
      data-slot="number-input-with-buttons"
      defaultValue={defaultValue}
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
        <Button
          aria-label="Decrease value"
          className={cn(
            "-ms-px flex aspect-square h-[inherit] items-center justify-center rounded-s-md border border-input bg-transparent text-muted-foreground/80 text-sm ring-offset-background transition-shadow hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50"
          )}
          data-slot="decrement-button"
          slot="decrement"
        >
          <HugeiconsIcon
            aria-hidden="true"
            className="size-4"
            icon={Cancel01Icon}
          />
        </Button>
        <Input
          aria-label={label || `${name} input`}
          className={cn(
            "w-full grow bg-transparent px-3 py-1 text-center text-foreground tabular-nums focus:outline-hidden",
            className
          )}
          data-slot="input"
          placeholder={placeholder}
          {...props}
        />
        <Button
          aria-label="Increase value"
          className={cn(
            "-me-px flex aspect-square h-[inherit] items-center justify-center rounded-e-md border border-input bg-transparent text-muted-foreground/80 text-sm ring-offset-background transition-shadow hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50"
          )}
          data-slot="increment-button"
          slot="increment"
        >
          <HugeiconsIcon
            aria-hidden="true"
            className="size-4"
            icon={Add01Icon}
          />
        </Button>
      </Group>
    </NumberField>
  );
}

NumberInputWithButtons.displayName = "NumberInputWithButtons";

export { NumberInputWithButtons };
