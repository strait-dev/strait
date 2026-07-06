import { Shell } from "@strait/ui/components/shell";
import { createFileRoute, redirect } from "@tanstack/react-router";
import MigrationCalculator from "@/components/billing/migration-calculator";
import DefaultCatchBoundary from "@/components/common/default-catch-boundary";
import NotFound from "@/components/common/not-found";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import { isCommunityEdition } from "@/lib/edition";
import { seo } from "@/lib/seo";

export const Route = createFileRoute("/app/pricing/compare")({
  // Cloud-only: pricing comparison against competitors is not
  // relevant for the community edition.
  beforeLoad: () => {
    if (isCommunityEdition) {
      throw redirect({ to: "/app" });
    }
  },
  head: () => ({ meta: seo({ title: "Compare plans" }) }),
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
          <h1 className="text-balance font-normal text-xl tracking-tight">
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
