import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { BarList } from "@strait/ui/components/bar-list";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import type { ChartConfig } from "@strait/ui/components/chart";
import { ChartEmptyState } from "@strait/ui/components/chart-empty-state";
import { DonutChart } from "@strait/ui/components/charts";
import { ConfigRow } from "@strait/ui/components/config-row";
import {
  DataGrid,
  DataGridContainer,
  DataGridPagination,
  DataGridScrollArea,
  DataGridSelectionBar,
  DataGridTable,
} from "@strait/ui/components/data-grid";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@strait/ui/components/empty";
import { MetricCard } from "@strait/ui/components/metric-card";
import { Shell } from "@strait/ui/components/shell";
import { StatusBadge } from "@strait/ui/components/status-badge";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs";
import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { useEffect, useMemo, useState } from "react";
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import RunDetailSheet from "@/components/dashboard/run-detail-sheet";
import { createRunColumns } from "@/components/tables/runs-columns";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { Job, JobRun, PaginatedResponse } from "@/hooks/api/types";
import {
  jobHealthQueryOptions,
  jobQueryOptions,
  usePauseJob,
  useResumeJob,
  useTriggerJob,
} from "@/hooks/api/use-jobs";
import {
  runsQueryOptions,
  useCancelRun,
  useRetryRun,
} from "@/hooks/api/use-runs";
import {
  type ProjectPermissionFlags,
  useProjectPermissions,
} from "@/hooks/auth/use-project-permissions";
import { useHydratedTableData } from "@/hooks/use-hydrated-table-data";
import {
  ActivityIcon,
  ClockIcon,
  EyeIcon,
  GlobeIcon,
  PauseActionIcon,
  PlayActionIcon,
  RefreshIcon,
  TagIcon,
  XCircleIcon,
} from "@/lib/icons";
import { stopInteractiveRowClick } from "@/lib/table-interactions";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/jobs/$id")({
  head: () => ({ meta: [{ title: "Job · Strait" }] }),
  loader: async ({ context, params }) => {
    const { session } = context as AppRouteContext;
    await Promise.all([
      context.queryClient.ensureQueryData(jobQueryOptions(params.id)),
      context.queryClient.ensureQueryData(
        runsQueryOptions({ job_id: params.id })
      ),
      context.queryClient.ensureQueryData(
        jobHealthQueryOptions(params.id, "7d")
      ),
    ]);
    return { session };
  },
  pendingComponent: DetailPageSkeleton,
  errorComponent: ErrorComponent,
  component: JobDetailPage,
});

type HealthWindow = "1h" | "1d" | "7d" | "30d";

const HEALTH_WINDOWS: { value: HealthWindow; label: string }[] = [
  { value: "1h", label: "1 hour" },
  { value: "1d", label: "24 hours" },
  { value: "7d", label: "7 days" },
  { value: "30d", label: "30 days" },
];

const STATUS_DISTRIBUTION_CONFIG = {
  Completed: { label: "Completed", color: "chart-1" },
  Failed: { label: "Failed", color: "chart-2" },
  "Timed Out": { label: "Timed Out", color: "chart-5" },
  Canceled: { label: "Canceled", color: "chart-5" },
} satisfies ChartConfig;

type JobHeaderActionsProps = {
  isHydrated: boolean;
  isPaused: boolean;
  job: Job;
  permissions: ProjectPermissionFlags;
  pauseJob: ReturnType<typeof usePauseJob>;
  resumeJob: ReturnType<typeof useResumeJob>;
  triggerJob: ReturnType<typeof useTriggerJob>;
};

function JobHeaderActions({
  isHydrated,
  isPaused,
  job,
  permissions,
  pauseJob,
  resumeJob,
  triggerJob,
}: JobHeaderActionsProps) {
  return (
    <div className="flex gap-2">
      {permissions.canTriggerJobs && (
        <Button
          disabled={!isHydrated || triggerJob.isPending}
          onClick={() => triggerJob.mutate({ id: job.id })}
        >
          <HugeiconsIcon className="mr-1.5" icon={PlayActionIcon} size={14} />
          {triggerJob.isPending ? "Triggering..." : "Trigger"}
        </Button>
      )}
      {permissions.canWriteJobs && (
        <Button
          disabled={!isHydrated || pauseJob.isPending || resumeJob.isPending}
          onClick={() =>
            isPaused
              ? resumeJob.mutate({ id: job.id })
              : pauseJob.mutate({ id: job.id })
          }
          variant="outline"
        >
          <HugeiconsIcon
            className="mr-1.5"
            icon={isPaused ? PlayActionIcon : PauseActionIcon}
            size={14}
          />
          {isPaused ? "Resume" : "Pause"}
        </Button>
      )}
    </div>
  );
}

function JobDetailPage() {
  const { id } = Route.useParams();
  const { session } = Route.useLoaderData();
  usePageEvent("job_detail_viewed", { job_id: id });
  const { data: job } = useSuspenseQuery(jobQueryOptions(id)) as {
    data: Job | undefined;
  };
  const { data: runsData } = useSuspenseQuery(
    runsQueryOptions({ job_id: id })
  ) as {
    data: PaginatedResponse<JobRun> | undefined;
  };

  const [activeTab, setActiveTab] = useState("overview");
  const [healthWindow, setHealthWindow] = useState<HealthWindow>("7d");
  const [selectedRun, setSelectedRun] = useState<JobRun | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
  const [isHydrated, setIsHydrated] = useState(false);

  const { data: health } = useQuery(jobHealthQueryOptions(id, healthWindow));

  const triggerJob = useTriggerJob();
  const pauseJob = usePauseJob();
  const resumeJob = useResumeJob();
  const retryRun = useRetryRun();
  const cancelRun = useCancelRun();
  const { permissions } = useProjectPermissions(session.user.activeProjectId);

  const jobRuns = runsData?.data ?? [];
  const tableData = useHydratedTableData(jobRuns);

  useEffect(() => {
    setIsHydrated(true);
  }, []);

  const table = useReactTable({
    data: tableData.data,
    columns: createRunColumns({
      onView: (run) => handleRowClick(run),
      onRetry: permissions.canWriteRuns
        ? (run) => retryRun.mutate({ run_id: run.id })
        : undefined,
      onCancel: permissions.canWriteRuns
        ? (run) => cancelRun.mutate({ run_id: run.id })
        : undefined,
    }),
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    state: { rowSelection },
    getRowId: (row) => row.id,
  });

  const selectedIds = Object.keys(rowSelection).filter((k) => rowSelection[k]);

  const stats = useMemo(() => {
    if (!health) {
      return {
        successRate: "0%",
        totalRuns: "0",
        avgDuration: "0s",
        failedRuns: "0",
      };
    }
    return {
      successRate: `${health.success_rate.toFixed(1)}%`,
      totalRuns: health.total_runs.toLocaleString(),
      avgDuration: `${health.avg_duration_secs.toFixed(1)}s`,
      failedRuns: health.failed_runs.toLocaleString(),
    };
  }, [health]);

  const chartData = useMemo(() => {
    if (!health) {
      return [];
    }
    return [
      {
        name: "Completed",
        value: health.completed_runs,
      },
      {
        name: "Failed",
        value: health.failed_runs,
      },
      {
        name: "Timed Out",
        value: health.timed_out_runs,
      },
      {
        name: "Canceled",
        value: health.canceled_runs,
      },
    ].filter((d) => d.value > 0);
  }, [health]);

  function handleRowClick(run: JobRun) {
    setSelectedRun(run);
    setSheetOpen(true);
  }

  if (!job) {
    return (
      <Shell>
        <EntityNotFound backTo="/app/jobs" entity="Job" />
      </Shell>
    );
  }

  const isPaused = job.paused || !job.enabled;

  return (
    <Shell>
      {/* Header */}
      <div className="flex flex-col gap-3 pt-4 pb-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex flex-col gap-1.5">
          <div className="flex items-center gap-3">
            <h1 className="text-balance font-normal text-xl tracking-tight">
              {job.name}
            </h1>
            <StatusBadge showDot status={isPaused ? "paused" : "completed"} />
          </div>
          {job.description && (
            <p className="text-pretty text-muted-foreground text-sm">
              {job.description}
            </p>
          )}
        </div>
        <JobHeaderActions
          isHydrated={isHydrated}
          isPaused={isPaused}
          job={job}
          pauseJob={pauseJob}
          permissions={permissions}
          resumeJob={resumeJob}
          triggerJob={triggerJob}
        />
      </div>

      {/* Tabs */}
      <Tabs className="w-full" onValueChange={setActiveTab} value={activeTab}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="runs">Recent Runs</TabsTrigger>
        </TabsList>

        <TabsContent className="mt-6 space-y-6" value="overview">
          {/* Time window selector */}
          <div className="flex items-center gap-1">
            {HEALTH_WINDOWS.map((w) => (
              <Button
                key={w.value}
                onClick={() => setHealthWindow(w.value)}
                variant={healthWindow === w.value ? "default" : "outline"}
              >
                {w.label}
              </Button>
            ))}
          </div>

          {/* Stats row */}
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <MetricCard
              size="sm"
              title="Success Rate"
              value={stats.successRate}
            />
            <MetricCard size="sm" title="Total Runs" value={stats.totalRuns} />
            <MetricCard
              size="sm"
              title="Avg Duration"
              value={stats.avgDuration}
            />
            <MetricCard
              size="sm"
              title="Failed Runs"
              value={stats.failedRuns}
            />
          </div>

          {/* Run Status Distribution */}
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="font-medium text-sm">
                Run Status Distribution
              </CardTitle>
            </CardHeader>
            <CardContent>
              {chartData.length > 0 ? (
                <div className="flex items-center gap-6">
                  <div className="h-[220px] flex-1">
                    <DonutChart
                      config={STATUS_DISTRIBUTION_CONFIG}
                      containerHeight={220}
                      data={chartData}
                      dataKey="value"
                      nameKey="name"
                      valueFormatter={(value) => value.toLocaleString()}
                    />
                  </div>
                  <BarList
                    className="w-56"
                    data={chartData}
                    sortOrder="none"
                    valueFormatter={(value) => value.toLocaleString()}
                  />
                </div>
              ) : (
                <ChartEmptyState message="No run data available for this time window." />
              )}
            </CardContent>
          </Card>

          {/* Configuration card */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="font-medium text-sm">
                Configuration
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <ConfigRow
                icon={GlobeIcon}
                label="Endpoint"
                value={job.endpoint_url || "-"}
              />
              <ConfigRow
                icon={ClockIcon}
                label="Schedule"
                value={job.cron || "Manual"}
              />
              <ConfigRow
                icon={RefreshIcon}
                label="Retry"
                value={`${job.max_attempts} attempts (${job.retry_strategy})`}
              />
              <ConfigRow
                icon={ClockIcon}
                label="Timeout"
                value={`${job.timeout_secs}s`}
              />
            </CardContent>
          </Card>

          {/* Tags */}
          {job.tags && Object.keys(job.tags).length > 0 && (
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="flex items-center gap-1.5 font-medium text-sm">
                  <HugeiconsIcon icon={TagIcon} size={14} />
                  Tags
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="flex flex-wrap gap-1.5">
                  {Object.entries(job.tags).map(([key, val]) => (
                    <Badge key={key} variant="secondary">
                      {key}: {String(val)}
                    </Badge>
                  ))}
                </div>
              </CardContent>
            </Card>
          )}
        </TabsContent>

        <TabsContent className="mt-6" value="runs">
          <div onClickCapture={stopInteractiveRowClick}>
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
                      No runs yet. Trigger this job to start an execution.
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
              }
              loading={tableData.isLoading}
              onRowClick={handleRowClick}
              recordCount={tableData.isHydrated ? jobRuns.length : 0}
              table={table}
              tableClassNames={{ base: "min-w-[1200px]" }}
            >
              <DataGridContainer>
                <DataGridScrollArea>
                  <DataGridTable />
                </DataGridScrollArea>
                <DataGridPagination />
              </DataGridContainer>
              <DataGridSelectionBar
                actions={[
                  ...(selectedIds.length === 1
                    ? [
                        {
                          label: "View",
                          icon: EyeIcon,
                          onClick: () => {
                            const run = table
                              .getRowModel()
                              .rows.find(
                                (r) => r.id === selectedIds[0]
                              )?.original;
                            if (run) {
                              handleRowClick(run);
                            }
                          },
                        },
                      ]
                    : []),
                  ...(permissions.canWriteRuns
                    ? [
                        {
                          label: "Retry",
                          icon: RefreshIcon,
                          onClick: () => {
                            for (const id of selectedIds) {
                              retryRun.mutate({ run_id: id });
                            }
                          },
                        },
                        {
                          label: "Cancel",
                          icon: XCircleIcon,
                          onClick: () => {
                            for (const id of selectedIds) {
                              cancelRun.mutate({ run_id: id });
                            }
                          },
                          variant: "destructive" as const,
                        },
                      ]
                    : []),
                ]}
              />
            </DataGrid>
          </div>

          <RunDetailSheet
            onOpenChange={setSheetOpen}
            open={sheetOpen}
            run={selectedRun}
          />
        </TabsContent>
      </Tabs>
    </Shell>
  );
}
