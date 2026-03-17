"use client";

import { ViewIcon, ViewOffIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useState } from "react";

import { cn } from "../utils/index";
import { Input } from "./input";

type InputWithShowHidePasswordProps = Omit<
  React.ComponentProps<"input">,
  "type"
>;

function InputWithShowHidePassword({
  className,
  ...props
}: InputWithShowHidePasswordProps) {
  const [isVisible, setIsVisible] = useState<boolean>(false);

  const toggleVisibility = () => setIsVisible((prevState) => !prevState);

  return (
    <div className="relative" data-slot="input-with-show-hide-password">
      <Input
        className={cn("pe-9", className)}
        type={isVisible ? "text" : "password"}
        {...props}
      />
      <button
        aria-label={isVisible ? "Hide password" : "Show password"}
        aria-pressed={isVisible}
        className="absolute inset-y-0 end-0 flex h-full w-9 items-center justify-center rounded-e-lg text-muted-foreground/80 outline-offset-2 transition-colors hover:text-foreground focus:z-10 focus-visible:outline focus-visible:outline-ring/70 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50"
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
  );
}

export { InputWithShowHidePassword };
