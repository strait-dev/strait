import type { BadgeProps } from "@strait/ui/components/badge.tsx";
import { Badge } from "@strait/ui/components/badge.tsx";
import { Button } from "@strait/ui/components/button.tsx";
import { Separator } from "@strait/ui/components/separator.tsx";
import { Tabs, TabsList, TabsTrigger } from "@strait/ui/components/tabs.tsx";
import { Link } from "@tanstack/react-router";
import type * as React from "react";
import type { TabConfig } from "@/hooks/use-entity-sheet.ts";
import PageHeaderWithBack from "../common/page-header-with-back.tsx";

export type EntityAction = {
  label: string;
  href?: string;
  onClick?: () => void;
  variant?:
    | "default"
    | "destructive"
    | "outline"
    | "secondary"
    | "ghost"
    | "link";
  icon?: React.ReactNode;
  disabled?: boolean;
};

export type EntityViewLayoutProps = {
  title: string;
  subtitle?: string;
  status?: {
    label: string;
    variant?:
      | "primary"
      | "secondary"
      | "destructive"
      | "outline"
      | "success"
      | "warning"
      | "info";
  };
  backHref: string;
  backLabel?: string;
  primaryAction?: EntityAction;
  secondaryActions?: EntityAction[];
  tabs: TabConfig[];
  activeTab: string;
  onTabChange: (tab: string) => void;
  children: React.ReactNode;
  isLoading?: boolean;
  error?: Error | null;
};

const EMPTY_ACTIONS: EntityViewLayoutProps["secondaryActions"] = [];

export const EntityViewLayout = ({
  title,
  subtitle,
  status,
  backHref,
  backLabel = "Back",
  primaryAction,
  secondaryActions = EMPTY_ACTIONS,
  tabs,
  activeTab,
  onTabChange,
  children,
  isLoading = false,
  error = null,
}: EntityViewLayoutProps) => {
  if (error) {
    return <EntityViewError backHref={backHref} error={error} />;
  }

  if (isLoading) {
    return <EntityViewSkeleton />;
  }

  return (
    <div className="w-full space-y-6">
      {/* Page Header with Back Navigation */}
      <PageHeaderWithBack
        backHref={backHref}
        backLabel={backLabel}
        button={
          <div className="flex items-center gap-2">
            {secondaryActions.map((action) =>
              action.href ? (
                <Button
                  disabled={action.disabled}
                  key={action.label}
                  onClick={action.onClick}
                  render={<Link to={action.href} />}
                  variant={action.variant || "outline"}
                >
                  {action.icon}
                  {action.label}
                </Button>
              ) : (
                <Button
                  disabled={action.disabled}
                  key={action.label}
                  onClick={action.onClick}
                  variant={action.variant || "outline"}
                >
                  {action.icon}
                  {action.label}
                </Button>
              )
            )}

            {primaryAction ? (
              <Button
                disabled={primaryAction.disabled}
                onClick={primaryAction.onClick}
                render={
                  primaryAction.href ? (
                    <Link to={primaryAction.href} />
                  ) : undefined
                }
                variant={primaryAction.variant || "default"}
              >
                {primaryAction.icon}
                {primaryAction.label}
              </Button>
            ) : null}
          </div>
        }
        text={subtitle || "View and manage entity details"}
        title={title}
      />

      {/* Status Badge (if provided) */}
      {status ? (
        <div className="flex items-center">
          <Badge
            variant={
              `${status.variant || "primary"}-light` as BadgeProps["variant"]
            }
          >
            {status.label}
          </Badge>
        </div>
      ) : null}

      <Separator />

      {/* Tabbed Content */}
      <Tabs className="w-full" onValueChange={onTabChange} value={activeTab}>
        <TabsList>
          {tabs.map((tab) => (
            <TabsTrigger
              className="flex items-center gap-2"
              disabled={tab.disabled}
              key={tab.id}
              value={tab.id}
            >
              {tab.label}
              {tab.badge ? (
                <Badge className="ml-1 text-xs" variant="secondary-light">
                  {tab.badge}
                </Badge>
              ) : null}
            </TabsTrigger>
          ))}
        </TabsList>

        <div className="mt-6">{children}</div>
      </Tabs>
    </div>
  );
};

/**
 * Loading skeleton for entity view pages
 */
const EntityViewSkeleton = () => {
  return (
    <div className="w-full space-y-6">
      {/* Back button skeleton */}
      <div className="h-9 w-20 animate-pulse rounded bg-muted" />

      {/* Header skeleton */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="space-y-2">
          <div className="h-8 w-48 animate-pulse rounded bg-muted" />
          <div className="h-4 w-32 animate-pulse rounded bg-muted" />
        </div>
        <div className="flex items-center gap-2">
          <div className="h-9 w-20 animate-pulse rounded bg-muted" />
          <div className="h-9 w-16 animate-pulse rounded bg-muted" />
        </div>
      </div>

      {/* Status badge skeleton */}
      <div className="h-6 w-16 animate-pulse rounded bg-muted" />

      <div className="h-px bg-muted" />

      {/* Tabs skeleton */}
      <div className="space-y-6">
        <div className="flex gap-1">
          <div className="h-10 w-24 animate-pulse rounded bg-muted" />
          <div className="h-10 w-20 animate-pulse rounded bg-muted" />
          <div className="h-10 w-28 animate-pulse rounded bg-muted" />
        </div>

        {/* Content skeleton */}
        <div className="space-y-4">
          <div className="h-4 w-full animate-pulse rounded bg-muted" />
          <div className="h-4 w-3/4 animate-pulse rounded bg-muted" />
          <div className="h-4 w-1/2 animate-pulse rounded bg-muted" />
        </div>
      </div>
    </div>
  );
};

/**
 * Error state for entity view pages
 */
export type EntityViewErrorProps = {
  error: Error;
  backHref: string;
};

const EntityViewError = ({ error, backHref }: EntityViewErrorProps) => {
  return (
    <div className="w-full">
      <div className="flex flex-col items-center justify-center py-12">
        <div className="space-y-4 text-center">
          <h2 className="font-heading font-semibold text-lg text-secondary-foreground">
            Error loading data
          </h2>
          <p className="max-w-md text-muted-foreground text-sm">
            {error.message ||
              "An unexpected error occurred while loading the information."}
          </p>
          <div className="flex gap-2">
            <Button onClick={() => window.location.reload()} variant="outline">
              Try again
            </Button>
            <Button render={<Link to={backHref} />}>Back</Button>
          </div>
        </div>
      </div>
    </div>
  );
};
