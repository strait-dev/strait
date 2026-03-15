"use client";

import { HugeiconsIcon, type IconSvgElement } from "@hugeicons/react";
import type React from "react";
import { cn } from "../utils/index";
import { Button } from "./button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "./tooltip";

export type NavigationRailProps = React.HTMLAttributes<HTMLDivElement> & {
  orientation?: "left" | "right";
};

export const NavigationRail = ({
  className,
  orientation = "left",
  children,
  ref,
  ...props
}: NavigationRailProps & { ref?: React.Ref<HTMLDivElement> }) => (
  <div
    className={cn(
      "flex h-full w-20 flex-col border-r bg-background",
      orientation === "right" && "order-last border-r-0 border-l",
      className
    )}
    ref={ref}
    {...props}
  >
    <TooltipProvider delay={0}>{children}</TooltipProvider>
  </div>
);

export type NavigationRailItemProps = {
  icon: IconSvgElement;
  label: string;
  href?: string;
  onClick?: () => void;
  isActive?: boolean;
  disabled?: boolean;
  badge?: React.ReactNode;
};

export const NavigationRailItem = ({
  icon,
  label,
  onClick,
  isActive,
  disabled,
  badge,
  ref,
  ...props
}: NavigationRailItemProps & { ref?: React.Ref<HTMLButtonElement> }) => (
  <Tooltip>
    <TooltipTrigger
      render={
        <Button
          className={cn(
            "relative h-14 w-14 rounded-lg",
            isActive ? "bg-accent" : null,
            disabled ? "pointer-events-none opacity-50" : null
          )}
          disabled={disabled}
          onClick={onClick}
          ref={ref}
          size="icon"
          variant={isActive ? "secondary" : "ghost"}
          {...props}
        />
      }
    >
      <HugeiconsIcon className="size-5" icon={icon} />
      {badge ? (
        <span className="absolute top-2 right-2 flex h-2 w-2">{badge}</span>
      ) : null}
      <span className="sr-only">{label}</span>
    </TooltipTrigger>
    <TooltipContent className="flex items-center gap-4" side="right">
      {label}
    </TooltipContent>
  </Tooltip>
);

export type NavigationRailSectionProps = {
  children: React.ReactNode;
  className?: string;
};

export const NavigationRailSection = ({
  children,
  className,
  ref,
  ...props
}: NavigationRailSectionProps & { ref?: React.Ref<HTMLDivElement> }) => (
  <div
    className={cn("flex flex-col items-center gap-2 py-2", className)}
    ref={ref}
    {...props}
  >
    {children}
  </div>
);

export type NavigationRailHeaderProps = {
  children: React.ReactNode;
  className?: string;
};

export const NavigationRailHeader = ({
  children,
  className,
  ref,
  ...props
}: NavigationRailHeaderProps & { ref?: React.Ref<HTMLDivElement> }) => (
  <div
    className={cn(
      "flex h-20 items-center justify-center border-b px-2",
      className
    )}
    ref={ref}
    {...props}
  >
    {children}
  </div>
);

export type NavigationRailFooterProps = {
  children: React.ReactNode;
  className?: string;
};

export const NavigationRailFooter = ({
  children,
  className,
  ref,
  ...props
}: NavigationRailFooterProps & { ref?: React.Ref<HTMLDivElement> }) => (
  <div
    className={cn(
      "mt-auto flex flex-col items-center gap-2 border-t py-2",
      className
    )}
    ref={ref}
    {...props}
  >
    {children}
  </div>
);
