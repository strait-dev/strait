import { ArrowDown01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import type React from "react";
import { cn } from "../utils/index";

const SelectNative = ({
  className,
  children,
  ...props
}: React.ComponentProps<"select">) => (
  <div className="relative flex">
    <select
      className={cn(
        "peer inline-flex w-full appearance-none items-center rounded-md border border-input text-foreground text-sm shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 has-[option[disabled]:checked]:text-muted-foreground aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40",
        props.multiple
          ? "py-1 *:px-3 *:py-1 [&_option:checked]:bg-accent"
          : "h-9 ps-3 pe-8",
        className
      )}
      data-slot="select-native"
      {...props}
    >
      {children}
    </select>
    {!props.multiple && (
      <span className="pointer-events-none absolute inset-y-0 end-0 flex h-full w-9 items-center justify-center text-muted-foreground/80 peer-disabled:opacity-50 peer-aria-invalid:text-destructive/80">
        <HugeiconsIcon
          aria-hidden="true"
          className="size-4"
          icon={ArrowDown01Icon}
        />
      </span>
    )}
  </div>
);

export { SelectNative };
