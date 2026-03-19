import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { ScrollArea } from "@strait/ui/components/scroll-area";
import { cn } from "@strait/ui/utils/index";
import { useQuery } from "@tanstack/react-query";
import { formatDistanceToNow } from "date-fns";
import type { JobRun } from "@/hooks/api/types";
import { runsQueryOptions } from "@/hooks/api/use-runs";
import { ActivityIcon } from "@/lib/icons";
import { ChartEmptyState } from "./chart-empty-state";

const STATUS_DOT: Record<string, string> = {
  executing: "bg-info",
  completed: "bg-success",
  failed: "bg-destructive",
  timed_out: "bg-destructive",
  crashed: "bg-destructive",
  canceled: "bg-muted-foreground",
  queued: "bg-info",
  delayed: "bg-warning",
  waiting: "bg-warning",
  dead_letter: "bg-destructive",
};

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

export function LiveActivityFeed({
  hasProject = true,
}: {
  hasProject?: boolean;
}) {
  const { data } = useQuery({
    ...runsQueryOptions({ limit: 20 }),
    enabled: hasProject,
  });
  const runs = data?.data ?? [];

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">Live Activity</CardTitle>
      </CardHeader>
      <CardContent className="p-0">
        <ScrollArea className="h-[320px] px-6 pb-6">
          <div className="space-y-3">
            {runs.map((run) => (
              <div className="flex items-start gap-2.5" key={run.id}>
                <span
                  className={cn(
                    "mt-1.5 size-2 shrink-0 rounded-full",
                    STATUS_DOT[run.status] ?? "bg-muted-foreground"
                  )}
                />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm leading-tight">
                    {runToMessage(run)}
                  </p>
                  <p className="text-muted-foreground text-xs">
                    {formatDistanceToNow(new Date(run.created_at), {
                      addSuffix: true,
                    })}
                  </p>
                </div>
              </div>
            ))}
            {runs.length === 0 && (
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
            )}
          </div>
        </ScrollArea>
      </CardContent>
    </Card>
  );
}
