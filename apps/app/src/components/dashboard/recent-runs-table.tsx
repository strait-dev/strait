import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { ChartEmptyState } from "@strait/ui/components/chart-empty-state";
import { StatusBadge } from "@strait/ui/components/status-badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@strait/ui/components/table";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { formatDistanceToNow } from "date-fns";
import type { JobRun, PaginatedResponse, RunStatus } from "@/hooks/api/types";
import { runsQueryOptions } from "@/hooks/api/use-runs";
import { LIVE_REFETCH_INTERVAL } from "@/hooks/utils";
import { ActivityIcon } from "@/lib/icons";

function formatDuration(
  startedAt: string | null,
  finishedAt: string | null
): string {
  if (!startedAt) {
    return "-";
  }
  const start = new Date(startedAt).getTime();
  const end = finishedAt ? new Date(finishedAt).getTime() : Date.now();
  const ms = end - start;
  if (ms < 1000) {
    return `${ms}ms`;
  }
  return `${(ms / 1000).toFixed(1)}s`;
}

const RecentRunsTable = ({ hasProject = true }: { hasProject?: boolean }) => {
  const { data } = useQuery({
    ...runsQueryOptions({ limit: 6 }),
    enabled: hasProject,
    refetchInterval: LIVE_REFETCH_INTERVAL,
    refetchIntervalInBackground: false,
  });
  const typed = data as PaginatedResponse<JobRun> | undefined;
  const runs = typed?.data ?? [];

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">Recent runs</CardTitle>
      </CardHeader>
      <CardContent className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="pl-6">Run ID</TableHead>
              <TableHead>Job</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Duration</TableHead>
              <TableHead>Started</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {runs.map((run) => (
              <TableRow key={run.id}>
                <TableCell className="pl-6">
                  <Link
                    className="font-mono text-sm hover:underline"
                    params={{ id: run.id }}
                    to="/app/runs/$id"
                  >
                    {run.id.slice(0, 12)}
                  </Link>
                </TableCell>
                <TableCell className="font-mono text-sm">
                  {run.job_id}
                </TableCell>
                <TableCell>
                  <StatusBadge status={run.status as RunStatus} />
                </TableCell>
                <TableCell className="font-mono text-sm tabular-nums">
                  {formatDuration(
                    run.started_at ?? null,
                    run.finished_at ?? null
                  )}
                </TableCell>
                <TableCell className="text-muted-foreground text-sm">
                  {run.created_at
                    ? formatDistanceToNow(new Date(run.created_at), {
                        addSuffix: true,
                      })
                    : "-"}
                </TableCell>
              </TableRow>
            ))}
            {runs.length === 0 && (
              <TableRow>
                <TableCell colSpan={5}>
                  <div className="py-8">
                    <ChartEmptyState
                      icon={ActivityIcon}
                      message={
                        hasProject
                          ? "No recent runs yet. Trigger a job to see activity here."
                          : "Create a project to start tracking runs."
                      }
                    />
                  </div>
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
};

export default RecentRunsTable;
