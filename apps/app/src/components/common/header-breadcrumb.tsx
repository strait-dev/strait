import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@strait/ui/components/breadcrumb";
import { Link, useRouterState } from "@tanstack/react-router";
import { Fragment, useMemo } from "react";

const ROUTE_LABELS: Record<string, string> = {
  app: "Home",
  dashboard: "Dashboard",
  jobs: "Jobs",
  workflows: "Workflows",
  runs: "Runs",
  schedules: "Schedules",
  dlq: "Dead letter queue",
  logs: "Logs",
  events: "Events",
  webhooks: "Webhooks",
  settings: "Settings",
  org: "Organization",
  upgrade: "Upgrade",
  billing: "Billing",
  analytics: "Analytics",
  pricing: "Plans",
  compare: "Compare plans",
  new: "New",
  "enterprise-contact": "Contact sales",
};

const HeaderBreadcrumb = () => {
  const pathname = useRouterState({
    select: (s) => s.location.pathname,
  });

  const crumbs = useMemo(() => {
    const segments = pathname.split("/").filter(Boolean);

    // Remove "app" prefix — it's always there
    if (segments[0] === "app") {
      segments.shift();
    }

    if (segments.length === 0) {
      return [{ label: "Overview", href: "/app", isPage: true }];
    }

    const result: { label: string; href: string; isPage: boolean }[] = [];

    for (let i = 0; i < segments.length; i++) {
      const segment = segments[i];
      const href = `/app/${segments.slice(0, i + 1).join("/")}`;
      const isPage = i === segments.length - 1;

      // Skip "org" segment — use the next segment (org ID) contextually
      if (segment === "org") {
        continue;
      }

      // If previous segment was "org", this is the org ID — label it
      if (i > 0 && segments[i - 1] === "org") {
        result.push({
          label: "Organization settings",
          href,
          isPage,
        });
        continue;
      }

      const label =
        ROUTE_LABELS[segment] ??
        // If it looks like an ID, show a truncated version
        (segment.length > 12 ? `${segment.slice(0, 8)}...` : segment);

      result.push({ label, href, isPage });
    }

    return result;
  }, [pathname]);

  return (
    <Breadcrumb className="min-w-0">
      <BreadcrumbList className="flex-nowrap overflow-hidden">
        {crumbs.map((crumb, i) => (
          <Fragment key={crumb.href}>
            {i > 0 && <BreadcrumbSeparator />}
            <BreadcrumbItem>
              {crumb.isPage ? (
                <BreadcrumbPage className="font-medium text-sm">
                  {crumb.label}
                </BreadcrumbPage>
              ) : (
                <BreadcrumbLink render={<Link to={crumb.href} />}>
                  {crumb.label}
                </BreadcrumbLink>
              )}
            </BreadcrumbItem>
          </Fragment>
        ))}
      </BreadcrumbList>
    </Breadcrumb>
  );
};

export default HeaderBreadcrumb;
