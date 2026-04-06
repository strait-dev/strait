import { createFileRoute } from "@tanstack/react-router";
import { lazy, Suspense } from "react";

import ErrorComponent from "@/components/common/error-component";

const DashboardContent = lazy(
  () => import("@/components/dashboard/dashboard-content")
);

import { usePageEvent } from "@/hooks/analytics/use-page-event";
import {
  analyticsQueryOptions as analyticsQueryOptionsFn,
  statsQueryOptions as statsQueryOptionsFn,
} from "@/hooks/api/use-dashboard";
import { runsQueryOptions } from "@/hooks/api/use-runs";
import { projectCostsQueryOptions } from "@/hooks/billing/use-project-costs";
import type { AppRouteContext } from "@/routes/app/layout";

const statsQueryOptions = statsQueryOptionsFn();
const analyticsQueryOptions = analyticsQueryOptionsFn(24);

export const Route = createFileRoute("/app/dashboard")({
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    const activeProjectId = session.user.activeProjectId ?? null;
    if (hasProject) {
      await Promise.allSettled([
        context.queryClient.ensureQueryData(statsQueryOptions),
        context.queryClient.ensureQueryData(analyticsQueryOptions),
        context.queryClient.ensureQueryData(runsQueryOptions({ limit: 20 })),
        context.queryClient.ensureQueryData(projectCostsQueryOptions()),
      ]);
    }
    return { hasProject, activeProjectId };
  },
  errorComponent: ErrorComponent,
  component: RouteComponent,
});

function RouteComponent() {
  usePageEvent("dashboard_viewed");
  const { hasProject, activeProjectId } = Route.useLoaderData();

  return (
    <Suspense
      fallback={<div className="h-96 animate-pulse rounded-lg bg-muted" />}
    >
      <DashboardContent
        activeProjectId={activeProjectId}
        hasProject={hasProject}
      />
    </Suspense>
  );
}
