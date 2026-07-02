import {
  ActivityFeed,
  type ActivityItem,
} from "@strait/ui/components/activity-feed";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { ChartEmptyState } from "@strait/ui/components/chart-empty-state";
import { useQuery } from "@tanstack/react-query";
import type { JobRun, PaginatedResponse } from "@/hooks/api/types";
import { runsQueryOptions } from "@/hooks/api/use-runs";
import { LIVE_REFETCH_INTERVAL } from "@/hooks/utils";
import { ActivityIcon } from "@/lib/icons";

function runToMessage(run: JobRun): string {
  const jobId = run.job_id;
  switch (run.status) {
    case "completed":
      return `${jobId} completed`;
    case "executing":
      return `${jobId} started (${run.id.slice(0, 10)})`;
    case "failed":
      return `${jobId} failed: ${run.error || "unknown error"}`;
    case "timed_out":
      return `${jobId} timed out`;
    case "canceled":
      return `${jobId} canceled`;
    case "queued":
      return `${jobId} queued`;
    case "dead_letter":
      return `${jobId} moved to DLQ`;
    default:
      return `${jobId} ${run.status}`;
  }
}

const LiveActivityFeed = ({ hasProject = true }: { hasProject?: boolean }) => {
  const { data } = useQuery({
    ...runsQueryOptions({ limit: 20 }),
    enabled: hasProject,
    refetchInterval: LIVE_REFETCH_INTERVAL,
    refetchIntervalInBackground: false,
  });
  const typed = data as PaginatedResponse<JobRun> | undefined;
  const runs = typed?.data ?? [];
  const items: ActivityItem[] = runs.map((run) => ({
    id: run.id,
    status: run.status,
    title: runToMessage(run),
    timestamp: run.created_at,
  }));

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">Live activity</CardTitle>
      </CardHeader>
      <CardContent className="p-0">
        <ActivityFeed
          className="px-6 pb-6"
          emptyState={
            <div className="py-8">
              <ChartEmptyState
                icon={ActivityIcon}
                message={
                  hasProject
                    ? "No recent activity yet."
                    : "Create a project to see live activity."
                }
              />
            </div>
          }
          items={items}
        />
      </CardContent>
    </Card>
  );
};

export default LiveActivityFeed;
