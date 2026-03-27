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
import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { useMemo, useState } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import CostEstimateCard from "@/components/billing/cost-estimate-card";
import ConfigRow from "@/components/common/config-row";
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import TableEmptyState from "@/components/common/table-empty-state";
import ChartTooltip from "@/components/dashboard/chart-tooltip";
import RunDetailSheet from "@/components/dashboard/run-detail-sheet";
import StatusBadge from "@/components/dashboard/status-badge";
import { runColumns } from "@/components/tables/runs-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import { DataTableFloatingBar } from "@/components/ui/data-table/data-table-floating-bar";
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
import { CHART_COLORS } from "@/lib/status-colors";

export const Route = createFileRoute("/app/jobs/$id")({
  loader: async ({ context, params }) => {
    await Promise.all([
      context.queryClient.ensureQueryData(jobQueryOptions(params.id)),
      context.queryClient.ensureQueryData(
        runsQueryOptions({ job_id: params.id })
      ),
      context.queryClient.ensureQueryData(
        jobHealthQueryOptions(params.id, "7d")
      ),
    ]);
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

const CHART_LABEL_MAP = {
  value: { label: "Runs", color: CHART_COLORS.success },
};

// --- Page ---

function JobDetailPage() {
  const { id } = Route.useParams();
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

  const { data: health } = useQuery(jobHealthQueryOptions(id, healthWindow));

  const triggerJob = useTriggerJob();
  const pauseJob = usePauseJob();
  const resumeJob = useResumeJob();
  const retryRun = useRetryRun();
  const cancelRun = useCancelRun();

  const jobRuns = runsData?.data ?? [];

  const table = useReactTable({
    data: jobRuns,
    columns: runColumns,
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
      { label: "Completed", value: health.completed_runs, fill: CHART_COLORS.success },
      { label: "Failed", value: health.failed_runs, fill: CHART_COLORS.error },
      { label: "Timed Out", value: health.timed_out_runs, fill: CHART_COLORS.neutral },
      { label: "Canceled", value: health.canceled_runs, fill: CHART_COLORS.neutral },
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

  return (
    <Shell>
      {/* Header */}
      <div className="flex flex-col gap-3 pt-4 pb-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex flex-col gap-1.5">
          <div className="flex items-center gap-3">
            <h1 className="text-balance font-normal text-xl tracking-tight sm:text-2xl">
              {job.name}
            </h1>
            <StatusBadge
              showDot
              status={job.enabled ? "completed" : "paused"}
            />
          </div>
          {job.description && (
            <p className="text-pretty text-muted-foreground text-sm">
              {job.description}
            </p>
          )}
        </div>
        <div className="flex gap-2">
          <Button
            disabled={triggerJob.isPending}
            onClick={() => triggerJob.mutate({ id: job.id })}
            size="sm"
          >
            <HugeiconsIcon className="mr-1.5" icon={PlayActionIcon} size={14} />
            {triggerJob.isPending ? "Triggering..." : "Trigger"}
          </Button>
          <Button
            disabled={pauseJob.isPending || resumeJob.isPending}
            onClick={() =>
              job.enabled
                ? pauseJob.mutate({ id: job.id })
                : resumeJob.mutate({ id: job.id })
            }
            size="sm"
            variant="outline"
          >
            <HugeiconsIcon
              className="mr-1.5"
              icon={job.enabled ? PauseActionIcon : PlayActionIcon}
              size={14}
            />
            {job.enabled ? "Pause" : "Resume"}
          </Button>
        </div>
      </div>

      {/* Tabs */}
      <Tabs className="w-full" onValueChange={setActiveTab} value={activeTab}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="runs">Recent Runs</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent className="mt-6 space-y-6" value="overview">
          {/* Time window selector */}
          <div className="flex items-center gap-1">
            {HEALTH_WINDOWS.map((w) => (
              <Button
                key={w.value}
                onClick={() => setHealthWindow(w.value)}
                size="sm"
                variant={healthWindow === w.value ? "default" : "outline"}
              >
                {w.label}
              </Button>
            ))}
          </div>

          {/* Stats row */}
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <StatCard label="Success Rate" value={stats.successRate} />
            <StatCard label="Total Runs" value={stats.totalRuns} />
            <StatCard label="Avg Duration" value={stats.avgDuration} />
            <StatCard label="Failed Runs" value={stats.failedRuns} />
          </div>

          {/* Run Status Distribution */}
          <Card>
            <CardHeader className="flex flex-col gap-2 pb-2 sm:flex-row sm:items-center sm:justify-between">
              <CardTitle className="font-medium text-sm">
                Run Status Distribution
              </CardTitle>
            </CardHeader>
            <CardContent>
              {chartData.length > 0 ? (
                <div className="h-[240px]">
                  <ResponsiveContainer
                    height="100%"
                    minHeight={1}
                    minWidth={1}
                    width="100%"
                  >
                    <BarChart data={chartData}>
                      <CartesianGrid
                        className="stroke-border"
                        strokeDasharray="3 3"
                      />
                      <XAxis
                        className="text-muted-foreground"
                        dataKey="label"
                        tick={{ fontSize: 12 }}
                      />
                      <YAxis
                        className="text-muted-foreground"
                        tick={{ fontSize: 12 }}
                      />
                      <Tooltip
                        content={<ChartTooltip labelMap={CHART_LABEL_MAP} />}
                        cursor={{ fill: "var(--muted)" }}
                      />
                      <Bar dataKey="value" radius={[2, 2, 0, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              ) : (
                <p className="py-8 text-center text-muted-foreground text-sm">
                  No run data available for this time window.
                </p>
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

          {/* Cost Estimate */}
          <CostEstimateCard timeoutSecs={job.timeout_secs} />

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
          {/* biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noNoninteractiveElementInteractions lint/a11y/noStaticElementInteractions: event delegation on table container */}
          <div
            className="[&_tbody_tr]:cursor-pointer"
            onClick={(e) => {
              const target = e.target as HTMLElement;
              if (target.closest("a, button")) {
                return;
              }
              const row = target.closest("tr[data-row-index]");
              if (!row) {
                return;
              }
              const idx = Number(row.getAttribute("data-row-index"));
              const run = table.getRowModel().rows[idx]?.original;
              if (run) {
                handleRowClick(run);
              }
            }}
          >
            <DataTable
              emptyState={
                <TableEmptyState
                  description="No runs found for this job."
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
              floatingBar={
                selectedIds.length > 0 ? (
                  <DataTableFloatingBar
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
                      {
                        label: "Retry",
                        icon: RefreshIcon,
                        onClick: () => {
                          for (const runId of selectedIds) {
                            retryRun.mutate({ run_id: runId });
                          }
                          setRowSelection({});
                        },
                      },
                      {
                        label: "Cancel",
                        icon: XCircleIcon,
                        onClick: () => {
                          for (const runId of selectedIds) {
                            cancelRun.mutate({ run_id: runId });
                          }
                          setRowSelection({});
                        },
                        variant: "destructive" as const,
                      },
                    ]}
                    onClearSelection={() => setRowSelection({})}
                    selectedCount={selectedIds.length}
                  />
                ) : null
              }
              table={table}
            />
          </div>

          <RunDetailSheet
            onOpenChange={setSheetOpen}
            open={sheetOpen}
            run={selectedRun}
          />
        </TabsContent>

        <TabsContent className="mt-6" value="settings">
          <Card>
            <CardHeader>
              <CardTitle className="font-medium text-sm">
                Job Settings
              </CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-muted-foreground text-sm">
                Job configuration management coming soon.
              </p>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </Shell>
  );
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <Card>
      <CardContent className="p-4 text-center">
        <p className="font-normal text-2xl tabular-nums">{value}</p>
        <p className="mt-1 text-muted-foreground text-xs">{label}</p>
      </CardContent>
    </Card>
  );
}
