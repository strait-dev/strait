import { ArrowLeft01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import type { BadgeProps } from "@strait/ui/components/badge";
import { Badge } from "@strait/ui/components/badge";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@strait/ui/components/breadcrumb";
import { Button } from "@strait/ui/components/button";
import { Separator } from "@strait/ui/components/separator";
import { Tabs, TabsList, TabsTrigger } from "@strait/ui/components/tabs";
import { Link } from "@tanstack/react-router";
import type React from "react";
import { Fragment } from "react";
import type { TabConfig } from "@/hooks/use-entity-sheet";

export type BreadcrumbConfig = {
  label: string;
  href?: string;
};

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

export type EntityDetailLayoutProps = {
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
  breadcrumbs: BreadcrumbConfig[];
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

const EMPTY_ACTIONS: EntityDetailLayoutProps["secondaryActions"] = [];

/**
 * Base EntityDetailLayout component for comprehensive entity management pages.
 *
 * This component provides a consistent structure for entity detail pages with:
 * - Breadcrumb navigation
 * - Entity header with title, subtitle, and status
 * - Action buttons (primary and secondary)
 * - Tabbed content area
 * - Loading and error states
 *
 * @example
 * <EntityDetailLayout
 *   title="John Doe"
 *   subtitle="Customer since 15/03/2024"
 *   status={{ label: "Active", variant: "success" }}
 *   breadcrumbs={[
 *     { label: "Customers", href: "/app/customers" },
 *     { label: "John Doe" }
 *   ]}
 *   backHref="/app/customers"
 *   primaryAction={{
 *     label: "Edit",
 *     href: "/app/customers/edit/123",
 *     icon: <HugeiconsIcon className="size-4" icon={PencilEdit02Icon} />
 *   }}
 *   tabs={[
 *     { id: "overview", label: "Overview" },
 *     { id: "orders", label: "Orders", badge: "12" },
 *     { id: "analytics", label: "Analytics" }
 *   ]}
 *   activeTab={activeTab}
 *   onTabChange={setActiveTab}
 * >
 *   <TabsContent value="overview">
 *     <CustomerOverviewTab customer={customer} />
 *   </TabsContent>
 *   <TabsContent value="orders">
 *     <CustomerOrdersTab customerId={customer.id} />
 *   </TabsContent>
 *   <TabsContent value="analytics">
 *     <CustomerAnalyticsTab customerId={customer.id} />
 *   </TabsContent>
 * </EntityDetailLayout>
 */
export const EntityDetailLayout = ({
  title,
  subtitle,
  status,
  breadcrumbs,
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
}: EntityDetailLayoutProps) => {
  if (error) {
    return <EntityDetailError backHref={backHref} error={error} />;
  }

  if (isLoading) {
    return <EntityDetailSkeleton />;
  }

  return (
    <div className="w-full space-y-6">
      {/* Breadcrumbs */}
      <Breadcrumb>
        <BreadcrumbList>
          {breadcrumbs.map((breadcrumb, index) => (
            <Fragment key={breadcrumb.label}>
              <BreadcrumbItem>
                {breadcrumb.href ? (
                  <BreadcrumbLink render={<Link to={breadcrumb.href} />}>
                    {breadcrumb.label}
                  </BreadcrumbLink>
                ) : (
                  <BreadcrumbPage>{breadcrumb.label}</BreadcrumbPage>
                )}
              </BreadcrumbItem>
              {index < breadcrumbs.length - 1 && <BreadcrumbSeparator />}
            </Fragment>
          ))}
        </BreadcrumbList>
      </Breadcrumb>

      {/* Back Button */}
      <div>
        <Button render={<Link to={backHref} />} size="sm" variant="ghost">
          <HugeiconsIcon className="size-4" icon={ArrowLeft01Icon} />
          {backLabel}
        </Button>
      </div>

      {/* Entity Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="space-y-2">
          <div className="flex items-center gap-3">
            <h1 className="font-bold font-heading text-2xl text-secondary-foreground tracking-tight">
              {title}
            </h1>
            {status ? (
              <Badge
                variant={
                  `${status.variant || "primary"}-light` as BadgeProps["variant"]
                }
              >
                {status.label}
              </Badge>
            ) : null}
          </div>
          {subtitle ? (
            <p className="text-muted-foreground text-sm">{subtitle}</p>
          ) : null}
        </div>

        {/* Actions */}
        <div className="flex items-center gap-2">
          {secondaryActions.map((action) =>
            action.href ? (
              <Button
                disabled={action.disabled}
                key={action.label}
                onClick={action.onClick}
                render={<Link to={action.href} />}
                size="sm"
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
                size="sm"
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
              size="sm"
              variant={primaryAction.variant || "default"}
            >
              {primaryAction.icon}
              {primaryAction.label}
            </Button>
          ) : null}
        </div>
      </div>

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
 * Loading skeleton for entity detail pages
 */
const EntityDetailSkeleton = () => {
  return (
    <div className="w-full space-y-6">
      {/* Breadcrumbs skeleton */}
      <div className="flex items-center gap-2">
        <div className="h-4 w-20 animate-pulse rounded bg-muted" />
        <div className="h-4 w-1 animate-pulse rounded bg-muted" />
        <div className="h-4 w-24 animate-pulse rounded bg-muted" />
      </div>

      {/* Back button skeleton */}
      <div className="h-9 w-20 animate-pulse rounded bg-muted" />

      {/* Header skeleton */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="space-y-2">
          <div className="flex items-center gap-3">
            <div className="h-8 w-48 animate-pulse rounded bg-muted" />
            <div className="h-6 w-16 animate-pulse rounded bg-muted" />
          </div>
          <div className="h-4 w-32 animate-pulse rounded bg-muted" />
        </div>
        <div className="flex items-center gap-2">
          <div className="h-9 w-20 animate-pulse rounded bg-muted" />
          <div className="h-9 w-16 animate-pulse rounded bg-muted" />
        </div>
      </div>

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
 * Error state for entity detail pages
 */
export type EntityDetailErrorProps = {
  error: Error;
  backHref: string;
};

const EntityDetailError = ({ error, backHref }: EntityDetailErrorProps) => {
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
            <Button render={<Link to={backHref} />}>
              <HugeiconsIcon className="size-4" icon={ArrowLeft01Icon} />
              Back
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
};
