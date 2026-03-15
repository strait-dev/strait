"use client";

import { ViewIcon, ViewOffIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useState } from "react";
import { Input } from "../components/input";
import { cn } from "../utils/index";

export type PasswordInputProps = Omit<
  React.InputHTMLAttributes<HTMLInputElement>,
  "type"
> & {
  label?: string;
  showPasswordLabel?: string;
  hidePasswordLabel?: string;
  containerClassName?: string;
};

function PasswordInput({
  className,
  label,
  showPasswordLabel = "Show password",
  hidePasswordLabel = "Hide password",
  containerClassName,
  ...props
}: PasswordInputProps) {
  const [isVisible, setIsVisible] = useState<boolean>(false);

  const toggleVisibility = () => setIsVisible((prevState) => !prevState);

  return (
    <div className="flex flex-col gap-2" data-slot="password-input">
      {label ? (
        <label
          className="font-medium text-foreground text-sm"
          data-slot="label"
          htmlFor={props.id}
        >
          {label}
        </label>
      ) : null}
      <div
        className={cn("relative", containerClassName)}
        data-slot="input-container"
      >
        <Input
          className={cn("pe-9", className)}
          data-slot="input"
          type={isVisible ? "text" : "password"}
          {...props}
        />
        <button
          aria-label={
            isVisible ? (hidePasswordLabel ?? "") : (showPasswordLabel ?? "")
          }
          aria-pressed={isVisible}
          className="absolute inset-y-0 end-0 flex h-full w-9 items-center justify-center rounded-e-md text-muted-foreground/80 ring-offset-background transition-shadow hover:text-foreground focus-visible:border focus-visible:border-ring focus-visible:text-foreground focus-visible:outline-hidden focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:ring-offset-2 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50"
          data-slot="visibility-toggle"
          onClick={toggleVisibility}
          type="button"
        >
          {isVisible ? (
            <HugeiconsIcon
              aria-hidden="true"
              className="size-4"
              icon={ViewOffIcon}
            />
          ) : (
            <HugeiconsIcon
              aria-hidden="true"
              className="size-4"
              icon={ViewIcon}
            />
          )}
        </button>
      </div>
    </div>
  );
}

export { PasswordInput };
