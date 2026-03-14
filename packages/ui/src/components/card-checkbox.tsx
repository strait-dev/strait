import { cva, type VariantProps } from "class-variance-authority";
import type React from "react";

import { cn } from "../utils/index";
import { Checkbox } from "./checkbox";

const cardCheckboxVariants = cva(
  "relative flex w-full items-start gap-2 rounded-md border border-input p-4 shadow-black/5 shadow-sm has-[[data-state=checked]]:border-ring",
  {
    variants: {
      variant: {
        default: "w-full",
        compact: "w-auto",
      },
      layout: {
        default: "items-center",
        start: "items-start",
      },
      disabled: {
        true: "cursor-not-allowed",
        false: "",
      },
    },
    defaultVariants: {
      variant: "default",
      layout: "default",
      disabled: false,
    },
  }
);

const CardCheckboxGroup: React.FC<React.HTMLAttributes<HTMLDivElement>> = ({
  className,
  ...props
}) => <div className={cn("grid gap-2", className)} {...props} />;
CardCheckboxGroup.displayName = "CardCheckboxGroup";

type CardCheckboxItemProps = React.ComponentPropsWithoutRef<typeof Checkbox> &
  VariantProps<typeof cardCheckboxVariants> & {
    icon?: React.ReactNode;
    label: React.ReactNode;
    description?: React.ReactNode;
    disabled?: boolean;
  };

const CardCheckboxItem: React.FC<CardCheckboxItemProps> = ({
  className,
  variant,
  layout,
  icon,
  label,
  description,
  id,
  disabled,
  ...props
}) => (
  <div
    className={cn(
      cardCheckboxVariants({ variant, layout, disabled, className })
    )}
  >
    <Checkbox
      aria-describedby={description ? `${id}-description` : undefined}
      className="after:absolute after:inset-0"
      disabled={disabled}
      id={id}
      {...props}
    />
    <div className="flex grow items-start gap-3">
      {icon ? (
        <div
          className={cn("shrink-0", disabled ? "text-muted-foreground" : null)}
        >
          {icon}
        </div>
      ) : null}
      <div className="grid grow gap-2">
        <span
          className={cn(
            "font-medium text-sm leading-none",
            disabled ? "text-muted-foreground" : null
          )}
        >
          {label}
        </span>
        {description ? (
          <p
            className={cn(
              "text-sm",
              disabled ? "text-muted-foreground/75" : "text-muted-foreground"
            )}
            id={id ? `${id}-description` : undefined}
          >
            {description}
          </p>
        ) : null}
      </div>
    </div>
  </div>
);
CardCheckboxItem.displayName = "CardCheckboxItem";

export { CardCheckboxGroup, CardCheckboxItem, type CardCheckboxItemProps };
