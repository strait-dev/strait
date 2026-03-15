import { Shell } from "@strait/ui/components/shell";
import { createFileRoute } from "@tanstack/react-router";
import PageHeader from "@/components/common/page-header";
import { LiveActivityFeed } from "@/components/dashboard/live-activity-feed";
import { RecentRunsTable } from "@/components/dashboard/recent-runs-table";
import { RunsChart } from "@/components/dashboard/runs-chart";
import { StatusDistributionChart } from "@/components/dashboard/status-distribution-chart";

export const Route = createFileRoute("/app/dashboard")({
  component: RouteComponent,
});

function RouteComponent() {
  return (
    <Shell>
      <PageHeader
        text="Detailed view of your orchestration activity, performance, and status."
        title="Dashboard"
      />

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <RunsChart />
        </div>
        <StatusDistributionChart />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <RecentRunsTable />
        </div>
        <LiveActivityFeed />
      </div>
    </Shell>
  );
}
