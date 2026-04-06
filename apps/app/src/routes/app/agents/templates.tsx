import { Shell } from "@strait/ui/components/shell";
import { Skeleton } from "@strait/ui/components/skeleton";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import TemplateCard from "@/components/agents/template-card";
import DefaultCatchBoundary from "@/components/common/default-catch-boundary";
import NotFound from "@/components/common/not-found";
import { agentTemplatesQueryOptions } from "@/hooks/api/use-agent-templates";
import type { AppRouteContext } from "@/routes/app/layout";

// @ts-expect-error -- route will be in routeTree after codegen
export const Route = createFileRoute("/app/agents/templates")({
  loader: async ({ context }) => {
    const ctx = context as AppRouteContext;
    await ctx.queryClient.ensureQueryData(agentTemplatesQueryOptions());
  },
  pendingComponent: () => (
    <Shell>
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }, (_, i) => (
          <Skeleton className="h-48 rounded-lg" key={`skel-${i.toString()}`} />
        ))}
      </div>
    </Shell>
  ),
  errorComponent: DefaultCatchBoundary,
  notFoundComponent: () => <NotFound />,
  component: TemplatesPage,
});

function TemplatesPage() {
  const { data: templates } = useQuery(agentTemplatesQueryOptions());

  return (
    <Shell>
      <div className="space-y-6">
        <div>
          <h1 className="text-balance font-normal text-foreground text-lg tracking-tight">
            Agent Templates
          </h1>
          <p className="text-muted-foreground text-sm">
            Start with a pre-configured agent template and customize for your
            use case.
          </p>
        </div>

        {templates && templates.length > 0 ? (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
            {templates.map((template) => (
              <TemplateCard key={template.slug} template={template} />
            ))}
          </div>
        ) : (
          <p className="text-muted-foreground text-sm">
            No templates available.
          </p>
        )}
      </div>
    </Shell>
  );
}
