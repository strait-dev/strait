"use client";

import { Input } from "../components/input";
import { cn } from "../utils/index";

export type InputWithStartIconProps =
  React.InputHTMLAttributes<HTMLInputElement> & {
    icon?: React.ReactNode;
    label?: string;
    containerClassName?: string;
  };

function InputWithStartIcon({
  className,
  icon,
  label,
  containerClassName,
  ...props
}: InputWithStartIconProps) {
  return (
    <div className="flex flex-col gap-2" data-slot="input-with-start-icon">
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
          className={cn("peer ps-9", className)}
          data-slot="input"
          {...props}
        />
        <div
          className="pointer-events-none absolute inset-y-0 start-0 flex items-center justify-center ps-3 text-muted-foreground/80 peer-disabled:opacity-50"
          data-slot="icon"
        >
          {icon}
        </div>
      </div>
    </div>
  );
}

export { InputWithStartIcon };
