import { LinkSquare01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { ScrollArea } from "@strait/ui/components/scroll-area";
import { Separator } from "@strait/ui/components/separator";
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@strait/ui/components/sheet";
import { Link } from "@tanstack/react-router";
import { memo, type ReactNode, useMemo } from "react";

export type EntitySheetProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  viewAllHref: string;
  viewAllLabel?: string;
  children: ReactNode;
  isLoading?: boolean;
  error?: Error | null;
};

/**
 * Base EntitySheet component for quick preview of entities.
 *
 * This component provides a consistent structure for entity preview sheets
 * across the application. It includes:
 * - Header with title and description
 * - Scrollable content area
 * - Footer with "View full details" action
 * - Loading and error states
 *
 * @example
 * <EntitySheet
 *   open={isOpen}
 *   onOpenChange={setIsOpen}
 *   title="Cliente"
 *   description="Quick customer information"
 *   viewAllHref="/app/customers/123"
 *   viewAllLabel="View full details"
 * >
 *   <CustomerSheetContent customer={customer} />
 * </EntitySheet>
 */
export const EntitySheet = memo<EntitySheetProps>(
  ({
    open,
    onOpenChange,
    title,
    description,
    viewAllHref,
    viewAllLabel = "View full details",
    children,
    isLoading = false,
    error = null,
  }) => {
    // Memoize error content to prevent recreation on every render
    const errorContent = useMemo(() => {
      if (!error) {
        return null;
      }
      return (
        <div className="flex items-center justify-center py-8">
          <div className="text-center">
            <p className="text-muted-foreground">Error loading information</p>
            <p className="mt-1 text-muted-foreground">{error.message}</p>
          </div>
        </div>
      );
    }, [error]);

    // Memoize loading content
    const loadingContent = useMemo(
      () => (
        <div className="space-y-4">
          <EntitySheetSkeleton />
        </div>
      ),
      []
    );

    return (
      <Sheet onOpenChange={onOpenChange} open={open}>
        <SheetContent className="flex h-full w-[400px] flex-col gap-0 p-0 sm:w-[540px] sm:max-w-[540px]">
          <SheetHeader className="px-6 pt-6 pb-4">
            <SheetTitle className="text-left">{title}</SheetTitle>
            {description ? (
              <SheetDescription className="text-left">
                {description}
              </SheetDescription>
            ) : null}
          </SheetHeader>

          <Separator />

          <ScrollArea className="flex-1 overflow-hidden">
            <div className="px-6 py-6">
              {(() => {
                if (error) {
                  return errorContent;
                }
                if (isLoading) {
                  return loadingContent;
                }
                return children;
              })()}
            </div>
          </ScrollArea>

          <Separator />

          <div className="flex w-full gap-3 px-6 py-4">
            <SheetClose
              render={<Button className="flex-1" variant="secondary" />}
            >
              Close
            </SheetClose>
            <Button className="flex-1" render={<Link to={viewAllHref} />}>
              <HugeiconsIcon className="size-4" icon={LinkSquare01Icon} />
              {viewAllLabel}
            </Button>
          </div>
        </SheetContent>
      </Sheet>
    );
  }
);

EntitySheet.displayName = "EntitySheet";

/**
 * Loading skeleton for entity sheets
 */
const EntitySheetSkeleton = memo(() => {
  return (
    <div className="space-y-6">
      {/* Header info skeleton */}
      <div className="space-y-3">
        <div className="h-4 w-3/4 animate-pulse rounded bg-muted" />
        <div className="h-3 w-1/2 animate-pulse rounded bg-muted" />
      </div>

      {/* Content blocks skeleton */}
      <div className="space-y-4">
        <div className="space-y-2">
          <div className="h-3 w-1/4 animate-pulse rounded bg-muted" />
          <div className="h-3 w-full animate-pulse rounded bg-muted" />
          <div className="h-3 w-2/3 animate-pulse rounded bg-muted" />
        </div>

        <div className="space-y-2">
          <div className="h-3 w-1/3 animate-pulse rounded bg-muted" />
          <div className="h-3 w-full animate-pulse rounded bg-muted" />
        </div>

        <div className="space-y-2">
          <div className="h-3 w-1/5 animate-pulse rounded bg-muted" />
          <div className="h-3 w-4/5 animate-pulse rounded bg-muted" />
        </div>
      </div>

      {/* Metrics skeleton */}
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <div className="h-3 w-3/4 animate-pulse rounded bg-muted" />
          <div className="h-6 w-full animate-pulse rounded bg-muted" />
        </div>
        <div className="space-y-2">
          <div className="h-3 w-2/3 animate-pulse rounded bg-muted" />
          <div className="h-6 w-full animate-pulse rounded bg-muted" />
        </div>
      </div>
    </div>
  );
});

EntitySheetSkeleton.displayName = "EntitySheetSkeleton";

/**
 * Common section component for entity sheets
 */
export type EntitySheetSectionProps = {
  title: string;
  children: ReactNode;
  className?: string;
};

export const EntitySheetSection = memo<EntitySheetSectionProps>(
  ({ title, children, className = "" }) => (
    <div className={`space-y-3 ${className}`}>
      <h4 className="font-medium text-secondary-foreground">{title}</h4>
      <div className="space-y-2">{children}</div>
    </div>
  )
);

EntitySheetSection.displayName = "EntitySheetSection";

/**
 * Common field component for entity sheets
 */
export type EntitySheetFieldProps = {
  label: string;
  value: string | number | null | undefined;
  placeholder?: string;
  className?: string;
};

export const EntitySheetField = memo<EntitySheetFieldProps>(
  ({ label, value, placeholder = "—", className = "" }) => {
    const displayValue = value || placeholder;

    return (
      <div className={`flex items-center justify-between ${className}`}>
        <span className="text-muted-foreground">{label}:</span>
        <span className="text-muted-foreground">{displayValue}</span>
      </div>
    );
  }
);

EntitySheetField.displayName = "EntitySheetField";

/**
 * Metrics display component for entity sheets
 */
export type EntitySheetMetricProps = {
  label: string;
  value: string | number;
  className?: string;
};

export const EntitySheetMetric = memo<EntitySheetMetricProps>(
  ({ label, value, className = "" }) => (
    <div className={`text-center ${className}`}>
      <div className="font-normal text-lg text-secondary-foreground">
        {value}
      </div>
      <div className="text-muted-foreground text-sm">{label}</div>
    </div>
  )
);

EntitySheetMetric.displayName = "EntitySheetMetric";
