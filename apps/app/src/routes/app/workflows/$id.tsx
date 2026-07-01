import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { ConfigRow } from "@strait/ui/components/config-row";
import {
  DataGrid,
  DataGridContainer,
  DataGridPagination,
  DataGridScrollArea,
  DataGridTable,
} from "@strait/ui/components/data-grid";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@strait/ui/components/empty";
import { FeatureLock } from "@strait/ui/components/feature-lock";
import { IdCell } from "@strait/ui/components/id-cell";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemGroup,
  ItemTitle,
} from "@strait/ui/components/item";
import { MetricCard } from "@strait/ui/components/metric-card";
import { Shell } from "@strait/ui/components/shell";
import { StatusBadge } from "@strait/ui/components/status-badge";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs";
import { useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import {
  type ColumnDef,
  getCoreRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import { useEffect, useState } from "react";
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import WorkflowDAGFlow from "@/components/dashboard/workflow-dag-flow";
import { RESOURCE_TABLE_CLASS_NAMES } from "@/components/tables/resource-table";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type {
  PaginatedResponse,
  Workflow,
  WorkflowRun,
  WorkflowRunStatus,
  WorkflowStep,
} from "@/hooks/api/types";
import {
  usePauseWorkflow,
  useResumeWorkflow,
  useTriggerWorkflow,
  workflowQueryOptions,
  workflowRunsQueryOptions,
  workflowStepsQueryOptions,
} from "@/hooks/api/use-workflows";
import {
  type ProjectPermissionFlags,
  useProjectPermissions,
} from "@/hooks/auth/use-project-permissions";
import { useCurrentPlan } from "@/hooks/billing/use-current-plan";
import { useHydratedTableData } from "@/hooks/use-hydrated-table-data";
import {
  ActivityIcon,
  CheckCircleIcon,
  ClockIcon,
  PauseActionIcon,
  PlayActionIcon,
  RefreshIcon,
  TagIcon,
  WorkflowIcon,
} from "@/lib/icons";
import { canUseFeature } from "@/lib/plan-tiers";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/workflows/$id")({
  head: () => ({ meta: [{ title: "Workflow · Strait" }] }),
  loader: async ({ context, params }) => {
    const { session } = context as AppRouteContext;
    await Promise.all([
      context.queryClient.ensureQueryData(workflowQueryOptions(params.id)),
      context.queryClient.ensureQueryData(workflowStepsQueryOptions(params.id)),
      context.queryClient.ensureQueryData(workflowRunsQueryOptions(params.id)),
    ]);
    return { session };
  },
  pendingComponent: DetailPageSkeleton,
  errorComponent: ErrorComponent,
  component: WorkflowDetailPage,
});

function formatDurationMs(ms: number) {
  if (!Number.isFinite(ms) || ms < 0) {
    return "--";
  }
  if (ms < 1000) {
    return `${ms}ms`;
  }
  if (ms < 60_000) {
    return `${(ms / 1000).toFixed(1)}s`;
  }

  const minutes = Math.floor(ms / 60_000);
  const seconds = Math.round((ms % 60_000) / 1000);
  return `${minutes}m ${seconds}s`;
}

function formatWorkflowRunDuration(run: WorkflowRun) {
  if (!(run.started_at && run.finished_at)) {
    return "--";
  }

  return formatDurationMs(
    new Date(run.finished_at).getTime() - new Date(run.started_at).getTime()
  );
}

function averageWorkflowRunDuration(runs: WorkflowRun[]) {
  const durations = runs
    .map((run) => {
      if (!(run.started_at && run.finished_at)) {
        return null;
      }
      const ms =
        new Date(run.finished_at).getTime() -
        new Date(run.started_at).getTime();
      return Number.isFinite(ms) && ms >= 0 ? ms : null;
    })
    .filter((duration): duration is number => duration !== null);

  if (durations.length === 0) {
    return "--";
  }

  const avg =
    durations.reduce((sum, duration) => sum + duration, 0) / durations.length;
  return formatDurationMs(avg);
}

const workflowRunColumns: ColumnDef<WorkflowRun>[] = [
  {
    accessorKey: "id",
    header: "Run ID",
    cell: ({ row }) => <IdCell id={row.original.id} length={8} />,
  },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }) => (
      <StatusBadge status={row.original.status as WorkflowRunStatus} />
    ),
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
    id: "duration",
    header: "Duration",
    cell: ({ row }) => formatWorkflowRunDuration(row.original),
  },
  {
    accessorKey: "error",
    header: "Error",
    cell: ({ row }) => (
      <span className="line-clamp-1 text-muted-foreground text-xs">
        {row.original.error || "--"}
      </span>
    ),
  },
  {
    accessorKey: "workflow_version",
    header: "Version",
    cell: ({ row }) => (
      <Badge mono size="xs" variant="secondary-light">
        v{row.original.workflow_version}
      </Badge>
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

type WorkflowDagStep = {
  dependencies: string[];
  id: string;
  name: string;
  status: "pending";
  type: string;
};

function WorkflowStepsCard({ steps }: { steps: WorkflowDagStep[] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">Workflow Steps</CardTitle>
      </CardHeader>
      <CardContent>
        {steps.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            No workflow steps are available for the current version.
          </p>
        ) : (
          <ItemGroup>
            {steps.map((step, index) => {
              const dependencies = step.dependencies.filter(Boolean);
              return (
                <Item key={step.id} size="xs" variant="ghost">
                  <Badge mono size="xs" variant="secondary-light">
                    {index + 1}
                  </Badge>
                  <ItemContent>
                    <ItemTitle>{step.name}</ItemTitle>
                    <p className="text-muted-foreground text-xs">
                      {step.type}
                      {dependencies.length > 0
                        ? ` after ${dependencies.join(", ")}`
                        : " with no dependencies"}
                    </p>
                  </ItemContent>
                  <ItemActions>
                    <Badge size="xs" variant="outline">
                      {dependencies.length > 0
                        ? `${dependencies.length} dependency${
                            dependencies.length === 1 ? "" : "ies"
                          }`
                        : "root step"}
                    </Badge>
                  </ItemActions>
                </Item>
              );
            })}
          </ItemGroup>
        )}
      </CardContent>
    </Card>
  );
}

function FailureSummaryCard({ failedRuns }: { failedRuns: number }) {
  if (failedRuns === 0) {
    return null;
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">Failure Summary</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-muted-foreground text-sm">
          {failedRuns} recent workflow run
          {failedRuns === 1 ? " has" : "s have"} failed or timed out. Open the
          Recent Runs tab to inspect the failed run rows and errors.
        </p>
      </CardContent>
    </Card>
  );
}

type LockedDAGFeature = {
  title: string;
  description: string;
};

function getLockedDAGFeature(
  currentPlan: string,
  hasApprovalGate: boolean,
  hasSubWorkflow: boolean
): LockedDAGFeature | null {
  if (hasApprovalGate && !canUseFeature(currentPlan, "approval_gates")) {
    return {
      title: "Approval Gates",
      description: "Requires the Pro plan or higher",
    };
  }

  if (hasSubWorkflow && !canUseFeature(currentPlan, "sub_workflows")) {
    return {
      title: "Sub-Workflows",
      description: "Requires the Pro plan or higher",
    };
  }

  return null;
}

function useIsHydrated() {
  const [isHydrated, setIsHydrated] = useState(false);

  useEffect(() => {
    setIsHydrated(true);
  }, []);

  return isHydrated;
}

function WorkflowActions({
  permissions,
  workflow,
}: {
  permissions: ProjectPermissionFlags;
  workflow: Workflow;
}) {
  const isHydrated = useIsHydrated();
  const triggerWorkflow = useTriggerWorkflow();
  const pauseWorkflow = usePauseWorkflow();
  const resumeWorkflow = useResumeWorkflow();

  return (
    <div className="flex gap-2">
      {permissions.canTriggerWorkflows && (
        <Button
          disabled={!isHydrated || triggerWorkflow.isPending}
          onClick={() => triggerWorkflow.mutate({ workflowId: workflow.id })}
        >
          <HugeiconsIcon className="mr-1.5 size-3.5" icon={PlayActionIcon} />
          Trigger
        </Button>
      )}
      {permissions.canWriteWorkflows && (
        <Button
          disabled={
            !isHydrated || pauseWorkflow.isPending || resumeWorkflow.isPending
          }
          onClick={() =>
            workflow.enabled
              ? pauseWorkflow.mutate({ workflowId: workflow.id })
              : resumeWorkflow.mutate({ workflowId: workflow.id })
          }
          variant="outline"
        >
          <HugeiconsIcon
            className="mr-1.5 size-3.5"
            icon={workflow.enabled ? PauseActionIcon : PlayActionIcon}
          />
          {workflow.enabled ? "Pause" : "Resume"}
        </Button>
      )}
    </div>
  );
}

function WorkflowDetailPage() {
  const { id } = Route.useParams();
  const { session } = Route.useLoaderData();
  const navigate = useNavigate();
  const currentPlan = useCurrentPlan();
  usePageEvent("workflow_detail_viewed", { workflow_id: id });
  const { data: workflow } = useSuspenseQuery(workflowQueryOptions(id)) as {
    data: Workflow | undefined;
  };
  const { data: apiSteps } = useSuspenseQuery(
    workflowStepsQueryOptions(id)
  ) as { data: WorkflowStep[] | undefined };
  const { data: runsData } = useSuspenseQuery(workflowRunsQueryOptions(id)) as {
    data: PaginatedResponse<WorkflowRun> | undefined;
  };
  const runs = runsData?.data ?? [];
  const tableData = useHydratedTableData(runs);
  const { permissions } = useProjectPermissions(session.user.activeProjectId);
  const [activeTab, setActiveTab] = useState("overview");

  // Map API steps to the shape WorkflowDAGFlow expects
  const dagSteps = (apiSteps ?? []).map((s: WorkflowStep) => ({
    id: s.step_ref || s.id,
    name: s.step_ref,
    type: s.step_type ?? "job",
    status: "pending" as const,
    dependencies: s.depends_on ?? [],
  }));

  const runsTable = useReactTable({
    data: tableData.data,
    columns: workflowRunColumns,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
  });

  // Compute overview stats from runs
  const totalRuns = runs?.length ?? 0;
  const successfulRuns =
    runs?.filter((r) => r.status === "completed").length ?? 0;
  const failedRuns =
    runs?.filter((r) => r.status === "failed" || r.status === "timed_out")
      .length ?? 0;
  const successRate =
    totalRuns > 0 ? Math.round((successfulRuns / totalRuns) * 100) : 0;
  const avgDuration = averageWorkflowRunDuration(runs);
  const recentRuns = (runs ?? []).slice(0, 5);
  const hasApprovalGate = dagSteps.some((step) => step.type === "approval");
  const hasSubWorkflow = dagSteps.some((step) => step.type === "sub_workflow");
  const lockedDAGFeature = getLockedDAGFeature(
    currentPlan,
    hasApprovalGate,
    hasSubWorkflow
  );

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
            <h1 className="text-balance font-normal text-xl tracking-tight">
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
        <WorkflowActions permissions={permissions} workflow={workflow} />
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
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-4">
            <MetricCard
              icon={CheckCircleIcon}
              size="sm"
              title="Success Rate"
              value={`${successRate}%`}
            />
            <MetricCard
              icon={ActivityIcon}
              size="sm"
              title="Total Runs"
              value={totalRuns}
            />
            <MetricCard
              icon={WorkflowIcon}
              size="sm"
              title="Steps"
              value={dagSteps.length}
            />
            <MetricCard
              icon={ClockIcon}
              size="sm"
              title="Avg Duration"
              value={avgDuration}
            />
          </div>

          <WorkflowStepsCard steps={dagSteps} />

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
                <ItemGroup>
                  {recentRuns.map((run) => (
                    <Item key={run.id} size="xs" variant="ghost">
                      <StatusBadge
                        showDot
                        size="xs"
                        status={run.status as WorkflowRunStatus}
                      />
                      <ItemContent>
                        <ItemTitle className="font-mono">
                          {run.id.slice(0, 8)}
                        </ItemTitle>
                      </ItemContent>
                      <Badge size="xs" variant="outline">
                        {run.triggered_by}
                      </Badge>
                      {run.error ? (
                        <Badge size="xs" variant="destructive">
                          {run.error}
                        </Badge>
                      ) : null}
                      <ItemActions>
                        <span className="text-muted-foreground">
                          {formatWorkflowRunDuration(run)}
                        </span>
                        <span>
                          {formatDistanceToNow(new Date(run.created_at), {
                            addSuffix: true,
                          })}
                        </span>
                      </ItemActions>
                    </Item>
                  ))}
                </ItemGroup>
              )}
            </CardContent>
          </Card>
          <FailureSummaryCard failedRuns={failedRuns} />
        </TabsContent>

        {/* DAG Tab */}
        <TabsContent className="mt-6 space-y-4" value="dag">
          <FeatureLock
            action={
              lockedDAGFeature ? (
                <Button
                  onClick={() => navigate({ to: "/app/upgrade" })}
                  variant="default"
                >
                  Upgrade
                </Button>
              ) : null
            }
            description={lockedDAGFeature?.description}
            locked={Boolean(lockedDAGFeature)}
            planLabel={lockedDAGFeature ? "Pro" : undefined}
            title={lockedDAGFeature?.title}
          >
            <Card>
              <CardContent className="p-0">
                <WorkflowDAGFlow steps={dagSteps} />
              </CardContent>
            </Card>
          </FeatureLock>
        </TabsContent>

        {/* Recent Runs Tab */}
        <TabsContent className="mt-6" value="runs">
          <DataGrid
            emptyMessage={
              <Empty className="h-[300px]">
                <EmptyHeader>
                  <EmptyMedia media="icon" size="lg">
                    <HugeiconsIcon
                      className="size-6 text-foreground"
                      icon={ActivityIcon}
                    />
                  </EmptyMedia>
                  <EmptyTitle>No runs found</EmptyTitle>
                  <EmptyDescription>
                    No runs yet. Trigger this workflow to start an execution.
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            }
            loading={tableData.isLoading}
            recordCount={tableData.isHydrated ? runs.length : 0}
            table={runsTable}
            tableClassNames={RESOURCE_TABLE_CLASS_NAMES}
          >
            <DataGridContainer>
              <DataGridScrollArea>
                <DataGridTable />
              </DataGridScrollArea>
              <DataGridPagination />
            </DataGridContainer>
          </DataGrid>
        </TabsContent>

        {/* Settings Tab */}
        <TabsContent className="mt-6 space-y-6" value="settings">
          {/* Configuration */}
          <Card>
            <CardHeader>
              <CardTitle className="font-medium text-muted-foreground text-xs uppercase tracking-wider">
                Configuration
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2.5">
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
            </CardContent>
          </Card>

          {/* Tags */}
          {workflow.tags && Object.keys(workflow.tags).length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-1.5 font-medium text-muted-foreground text-xs uppercase tracking-wider">
                  <HugeiconsIcon icon={TagIcon} size={12} />
                  Tags
                </CardTitle>
              </CardHeader>
              <CardContent className="flex flex-wrap gap-1.5">
                {Object.entries(workflow.tags).map(([key, val]) => (
                  <Badge key={key} variant="secondary">
                    {key}: {val}
                  </Badge>
                ))}
              </CardContent>
            </Card>
          )}
        </TabsContent>
      </Tabs>
    </Shell>
  );
}
