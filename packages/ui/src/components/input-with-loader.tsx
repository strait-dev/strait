"use client";

import { Loading01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { cn } from "../utils/index";
import { Input } from "./input";

export type InputWithLoaderProps =
  React.InputHTMLAttributes<HTMLInputElement> & {
    isLoading?: boolean;
    icon?: React.ReactNode;
    endIcon?: React.ReactNode;
    onEndIconClick?: () => void;
    endIconAriaLabel?: string;
  };

function InputWithLoader({
  className,
  isLoading = false,
  icon,
  endIcon,
  onEndIconClick,
  endIconAriaLabel,
  ...props
}: InputWithLoaderProps) {
  return (
    <div className="relative" data-slot="input-with-loader">
      <Input
        className={cn(endIcon ? "pe-9" : "", icon ? "ps-9" : "", className)}
        data-slot="input"
        {...props}
      />
      {icon || isLoading ? (
        <div
          className="pointer-events-none absolute inset-y-0 start-0 flex items-center justify-center ps-3 text-muted-foreground/80 peer-disabled:opacity-50"
          data-slot="start-icon"
        >
          {isLoading ? (
            <HugeiconsIcon
              aria-hidden="true"
              className="size-4 animate-spin"
              icon={Loading01Icon}
            />
          ) : (
            icon
          )}
        </div>
      ) : null}
      {endIcon ? (
        <button
          aria-label={endIconAriaLabel}
          className="absolute inset-y-0 end-0 flex h-full w-9 items-center justify-center rounded-e-md text-muted-foreground/80 outline-offset-2 transition-colors hover:text-foreground focus:z-10 focus-visible:outline focus-visible:outline-ring/70 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50"
          data-slot="end-icon"
          onClick={onEndIconClick}
          type="button"
        >
          {endIcon}
        </button>
      ) : null}
    </div>
  );
}

export { InputWithLoader };
