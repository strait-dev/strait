import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Shell } from "@strait/ui/components/shell";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs";
import { useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  type ColumnDef,
  getCoreRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import { useState } from "react";
import ConfigRow from "@/components/common/config-row";
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import TableEmptyState from "@/components/common/table-empty-state";
import StatusBadge from "@/components/dashboard/status-badge";
import WorkflowDAGFlow from "@/components/dashboard/workflow-dag-flow";
import { DataTable } from "@/components/ui/data-table/data-table";
import type { WorkflowRun, WorkflowStep } from "@/hooks/api/types";
import {
  workflowQueryOptions,
  workflowRunsQueryOptions,
  workflowStepsQueryOptions,
} from "@/hooks/api/use-workflows";
import {
  ActivityIcon,
  CheckCircleIcon,
  ClockIcon,
  PauseActionIcon,
  PlayActionIcon,
  RefreshIcon,
  TagIcon,
} from "@/lib/icons";

export const Route = createFileRoute("/app/workflows/$id")({
  loader: async ({ context, params }) => {
    await Promise.all([
      context.queryClient.ensureQueryData(workflowQueryOptions(params.id)),
      context.queryClient.ensureQueryData(workflowStepsQueryOptions(params.id)),
      context.queryClient.ensureQueryData(workflowRunsQueryOptions(params.id)),
    ]);
  },
  pendingComponent: DetailPageSkeleton,
  errorComponent: ErrorComponent,
  component: WorkflowDetailPage,
});

const workflowRunColumns: ColumnDef<WorkflowRun>[] = [
  {
    accessorKey: "id",
    header: "Run ID",
    cell: ({ row }) => (
      <span className="font-mono text-xs">{row.original.id.slice(0, 8)}</span>
    ),
  },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }) => <StatusBadge status={row.original.status} />,
  },
  {
    accessorKey: "triggered_by",
    header: "Trigger",
    cell: ({ row }) => (
      <Badge className="capitalize" variant="outline">
        {row.original.triggered_by}
      </Badge>
    ),
  },
  {
    accessorKey: "workflow_version",
    header: "Version",
    cell: ({ row }) => (
      <code className="text-xs">v{row.original.workflow_version}</code>
    ),
  },
  {
    accessorKey: "created_at",
    header: "Started",
    cell: ({ row }) =>
      formatDistanceToNow(new Date(row.original.created_at), {
        addSuffix: true,
      }),
  },
];

function WorkflowDetailPage() {
  const { id } = Route.useParams();
  const { data: workflow } = useSuspenseQuery(workflowQueryOptions(id));
  const { data: apiSteps } = useSuspenseQuery(workflowStepsQueryOptions(id));
  const { data: runsData } = useSuspenseQuery(workflowRunsQueryOptions(id));
  const runs = runsData?.data ?? [];
  const [activeTab, setActiveTab] = useState("overview");

  // Map API steps to the shape WorkflowDAGFlow expects
  const dagSteps = (apiSteps ?? []).map((s: WorkflowStep) => ({
    id: s.id,
    name: s.step_ref,
    type: s.step_type,
    status: "pending" as const,
    dependencies: s.depends_on,
  }));

  const runsTable = useReactTable({
    data: runs ?? [],
    columns: workflowRunColumns,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
  });

  // Compute overview stats from runs
  const totalRuns = runs?.length ?? 0;
  const successfulRuns =
    runs?.filter((r) => r.status === "completed").length ?? 0;
  const successRate =
    totalRuns > 0 ? Math.round((successfulRuns / totalRuns) * 100) : 0;
  const recentRuns = (runs ?? []).slice(0, 5);

  if (!workflow) {
    return (
      <Shell>
        <EntityNotFound backTo="/app/workflows" entity="Workflow" />
      </Shell>
    );
  }

  return (
    <Shell>
      {/* Header */}
      <div className="flex items-start justify-between pt-4 pb-6">
        <div className="flex flex-col gap-2">
          <div className="flex items-center gap-3">
            <h1 className="text-balance font-normal text-2xl tracking-tight">
              {workflow.name}
            </h1>
            <StatusBadge
              showDot
              status={workflow.enabled ? "completed" : "paused"}
            />
          </div>
          {workflow.description && (
            <p className="text-pretty text-muted-foreground text-sm">
              {workflow.description}
            </p>
          )}
        </div>
        <div className="flex gap-2">
          <Button size="sm">
            <HugeiconsIcon className="mr-1.5" icon={PlayActionIcon} size={14} />
            Trigger
          </Button>
          <Button size="sm" variant="outline">
            <HugeiconsIcon
              className="mr-1.5"
              icon={workflow.enabled ? PauseActionIcon : PlayActionIcon}
              size={14}
            />
            {workflow.enabled ? "Pause" : "Resume"}
          </Button>
        </div>
      </div>

      {/* Tabs */}
      <Tabs className="w-full" onValueChange={setActiveTab} value={activeTab}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="dag">DAG</TabsTrigger>
          <TabsTrigger value="runs">Recent Runs</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        {/* Overview Tab */}
        <TabsContent className="mt-6 space-y-6" value="overview">
          {/* Stats row */}
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Card size="sm">
              <CardHeader>
                <CardTitle className="flex items-center gap-2 font-normal text-muted-foreground text-sm">
                  <HugeiconsIcon icon={CheckCircleIcon} size={14} />
                  Success Rate
                </CardTitle>
              </CardHeader>
              <CardContent>
                <p className="font-semibold text-2xl tabular-nums">
                  {successRate}%
                </p>
              </CardContent>
            </Card>

            <Card size="sm">
              <CardHeader>
                <CardTitle className="flex items-center gap-2 font-normal text-muted-foreground text-sm">
                  <HugeiconsIcon icon={ActivityIcon} size={14} />
                  Total Runs
                </CardTitle>
              </CardHeader>
              <CardContent>
                <p className="font-semibold text-2xl tabular-nums">
                  {totalRuns}
                </p>
              </CardContent>
            </Card>

            <Card size="sm">
              <CardHeader>
                <CardTitle className="flex items-center gap-2 font-normal text-muted-foreground text-sm">
                  <HugeiconsIcon icon={ClockIcon} size={14} />
                  Avg Duration
                </CardTitle>
              </CardHeader>
              <CardContent>
                <p className="font-semibold text-2xl tabular-nums">--</p>
              </CardContent>
            </Card>
          </div>

          {/* Recent activity timeline */}
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">Recent Activity</CardTitle>
            </CardHeader>
            <CardContent>
              {recentRuns.length === 0 ? (
                <p className="text-muted-foreground text-sm">
                  No recent activity.
                </p>
              ) : (
                <div className="space-y-3">
                  {recentRuns.map((run) => (
                    <div
                      className="flex items-center gap-3 text-sm"
                      key={run.id}
                    >
                      <StatusBadge showDot size="xs" status={run.status} />
                      <span className="font-mono text-muted-foreground text-xs">
                        {run.id.slice(0, 8)}
                      </span>
                      <Badge className="capitalize" size="xs" variant="outline">
                        {run.triggered_by}
                      </Badge>
                      <span className="ml-auto text-muted-foreground text-xs">
                        {formatDistanceToNow(new Date(run.created_at), {
                          addSuffix: true,
                        })}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* DAG Tab */}
        <TabsContent className="mt-6" value="dag">
          <Card>
            <CardContent className="p-0">
              <WorkflowDAGFlow steps={dagSteps} />
            </CardContent>
          </Card>
        </TabsContent>

        {/* Recent Runs Tab */}
        <TabsContent className="mt-6" value="runs">
          <DataTable
            emptyState={
              <TableEmptyState
                description="No runs found for this workflow."
                hideButton
                icon={
                  <HugeiconsIcon
                    className="size-6 text-foreground"
                    icon={ActivityIcon}
                  />
                }
                title="No runs found"
              />
            }
            table={runsTable}
          />
        </TabsContent>

        {/* Settings Tab */}
        <TabsContent className="mt-6 space-y-6" value="settings">
          {/* Configuration */}
          <div className="space-y-3 rounded-md border p-4">
            <h3 className="font-medium text-muted-foreground text-xs uppercase">
              Configuration
            </h3>
            <div className="space-y-2.5">
              <ConfigRow
                icon={ClockIcon}
                label="Timeout"
                value={`${workflow.timeout_secs}s`}
              />
              <ConfigRow
                icon={RefreshIcon}
                label="Max Concurrent Runs"
                value={String(workflow.max_concurrent_runs)}
              />
              <ConfigRow
                icon={RefreshIcon}
                label="Max Parallel Steps"
                value={String(workflow.max_parallel_steps)}
              />
              <ConfigRow
                icon={ClockIcon}
                label="Schedule"
                value={workflow.cron || "Manual"}
              />
              {workflow.cron_timezone && (
                <ConfigRow
                  icon={ClockIcon}
                  label="Timezone"
                  value={workflow.cron_timezone}
                />
              )}
              <ConfigRow
                icon={RefreshIcon}
                label="Skip If Running"
                value={workflow.skip_if_running ? "Yes" : "No"}
              />
              <ConfigRow
                icon={RefreshIcon}
                label="Version Policy"
                value={workflow.version_policy}
              />
            </div>
          </div>

          {/* Tags */}
          {workflow.tags && Object.keys(workflow.tags).length > 0 && (
            <div className="rounded-md border p-4">
              <h3 className="mb-3 flex items-center gap-1.5 font-medium text-muted-foreground text-xs uppercase">
                <HugeiconsIcon icon={TagIcon} size={12} />
                Tags
              </h3>
              <div className="flex flex-wrap gap-1.5">
                {Object.entries(workflow.tags).map(([key, val]) => (
                  <Badge key={key} variant="secondary">
                    {key}: {val}
                  </Badge>
                ))}
              </div>
            </div>
          )}
        </TabsContent>
      </Tabs>
    </Shell>
  );
}
