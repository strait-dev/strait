import { Shell } from "@strait/ui/components/shell";
import { createFileRoute } from "@tanstack/react-router";
import { Suspense } from "react";
import { DefaultCatchBoundary } from "@/components/common/default-catch-boundary";
import NotFound from "@/components/common/not-found";
import { ProjectSettings } from "@/components/project/project-settings";
import {
  projectSettingsQueryOptions,
  regionsQueryOptions,
} from "@/hooks/api/use-regions";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/projects/$projectId/settings")({
  loader: ({ context, params }) => {
    const ctx = context as AppRouteContext;
    ctx.queryClient.ensureQueryData(regionsQueryOptions());
    ctx.queryClient.ensureQueryData(
      projectSettingsQueryOptions(params.projectId)
    );
  },
  errorComponent: DefaultCatchBoundary,
  notFoundComponent: () => <NotFound />,
  component: RouteComponent,
});

function RouteComponent() {
  const { projectId } = Route.useParams();

  return (
    <Shell>
      <Suspense
        fallback={<div className="h-64 animate-pulse rounded-lg bg-muted" />}
      >
        <ProjectSettings projectId={projectId} />
      </Suspense>
    </Shell>
  );
}
