import { Add01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button.tsx";
import { Link } from "@tanstack/react-router";
import type * as React from "react";

type TableEmptyStateProps = {
  icon: React.ReactNode;
  title: string;
  description: string;
  buttonText?: string;
  href?: string;
  hideButton?: boolean;
  customButton?: React.ReactNode;
};

const TableEmptyState = ({
  icon,
  title,
  description,
  buttonText,
  href,
  hideButton = false,
  customButton,
}: TableEmptyStateProps) => (
  <div className="flex h-[300px] flex-col items-center justify-center gap-4 rounded-xl border border-muted-foreground/10 border-dashed p-8 text-center">
    <div>
      <div className="flex aspect-square h-14 items-center justify-center rounded-xl bg-primary/10">
        {icon}
      </div>
    </div>

    <div className="flex max-w-xs flex-col items-center gap-2 text-center">
      <h2 className="font-semibold text-lg text-secondary-foreground tracking-tight">
        {title}
      </h2>
      <p className="text-muted-foreground text-sm">{description}</p>
    </div>

    {!hideButton &&
      (customButton || (
        <Button
          className="mt-4"
          render={<Link className="items-center" to={href || "#"} />}
        >
          <HugeiconsIcon
            aria-hidden="true"
            className="size-4"
            icon={Add01Icon}
          />
          <span className="leading-none">{buttonText}</span>
        </Button>
      ))}
  </div>
);

export default TableEmptyState;
