import { cn } from "@strait/ui/utils";
import type { ElementType, ReactNode } from "react";

type ShellProps = {
  children: ReactNode;
  className?: string;
  as?: ElementType;
  variant?: "default" | "narrow" | "wide";
};

const Shell = ({
  children,
  className,
  as: Component = "div",
  variant = "default",
}: ShellProps) => {
  const maxWidthClasses = {
    narrow: "max-w-4xl",
    default: "max-w-5xl",
    wide: "max-w-[1600px]",
  };

  return (
    <Component
      className={cn(
        "mx-auto px-4 sm:px-6 lg:px-8",
        maxWidthClasses[variant],
        className
      )}
    >
      {children}
    </Component>
  );
};

export default Shell;
