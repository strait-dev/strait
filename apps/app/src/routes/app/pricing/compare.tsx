import { Shell } from "@strait/ui/components/shell";
import { createFileRoute } from "@tanstack/react-router";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import MigrationCalculator from "@/components/billing/migration-calculator";
import DefaultCatchBoundary from "@/components/common/default-catch-boundary";
import NotFound from "@/components/common/not-found";

export const Route = createFileRoute("/app/pricing/compare")({
  errorComponent: DefaultCatchBoundary,
  notFoundComponent: () => <NotFound />,
  component: RouteComponent,
});

function RouteComponent() {
  usePageEvent("pricing_compared");

  return (
    <Shell>
      <div className="flex w-full flex-col gap-6">
        <div>
          <h1 className="font-semibold text-2xl tracking-tight">
            Compare with competitors
          </h1>
          <p className="mt-1 text-muted-foreground">
            See how Strait pricing compares with alternative providers.
          </p>
        </div>
        <MigrationCalculator />
      </div>
    </Shell>
  );
}
