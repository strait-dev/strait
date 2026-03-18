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
import { DetailPageSkeleton } from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import TableEmptyState from "@/components/common/table-empty-state";
import { ChartTooltip } from "@/components/dashboard/chart-tooltip";
import { RunDetailSheet } from "@/components/dashboard/run-detail-sheet";
import { StatusBadge } from "@/components/dashboard/status-badge";
import { runColumns } from "@/components/tables/runs-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import { DataTableFloatingBar } from "@/components/ui/data-table/data-table-floating-bar";
import type { JobRun } from "@/hooks/api/types";
import { jobQueryOptions } from "@/hooks/api/use-jobs";
import { runsQueryOptions } from "@/hooks/api/use-runs";
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
      context.queryClient.ensureQueryData(runsQueryOptions()),
    ]);
  },
  pendingComponent: DetailPageSkeleton,
  errorComponent: ErrorComponent,
  component: JobDetailPage,
});

// --- Mock chart data generator ---

type DateRange = "7d" | "14d" | "30d" | "90d";

const DATE_RANGES: { value: DateRange; label: string; days: number }[] = [
  { value: "7d", label: "7 days", days: 7 },
  { value: "14d", label: "14 days", days: 14 },
  { value: "30d", label: "30 days", days: 30 },
  { value: "90d", label: "90 days", days: 90 },
];

function generateChartData(days: number) {
  const data: { date: string; completed: number; failed: number }[] = [];
  const now = new Date();
  const fmt =
    days <= 14
      ? { month: "short" as const, day: "numeric" as const }
      : { month: "short" as const, day: "numeric" as const };
  for (let i = days - 1; i >= 0; i--) {
    const d = new Date(now);
    d.setDate(d.getDate() - i);
    data.push({
      date: d.toLocaleDateString("en-US", fmt),
      completed: Math.floor(Math.random() * 40) + 60,
      failed: Math.floor(Math.random() * 6),
    });
  }
  return data;
}

function computeStats(chartData: { completed: number; failed: number }[]) {
  const totalSuccess = chartData.reduce((s, d) => s + d.completed, 0);
  const totalFailed = chartData.reduce((s, d) => s + d.failed, 0);
  const totalRuns = totalSuccess + totalFailed;
  const successRate =
    totalRuns > 0 ? ((totalSuccess / totalRuns) * 100).toFixed(1) : "0";
  const avgDuration = (2 + Math.random() * 5).toFixed(1);
  return {
    successRate: `${successRate}%`,
    totalRuns: totalRuns.toLocaleString(),
    avgDuration: `${avgDuration}s`,
    failedRuns: totalFailed.toLocaleString(),
  };
}

const CHART_LABEL_MAP = {
  completed: { label: "Completed", color: CHART_COLORS.success },
  failed: { label: "Failed", color: CHART_COLORS.error },
};

const CHART_LEGEND = [
  { label: "Completed", color: CHART_COLORS.success },
  { label: "Failed", color: CHART_COLORS.error },
];

// --- Page ---

function JobDetailPage() {
  const { id } = Route.useParams();
  const { data: job } = useSuspenseQuery(jobQueryOptions(id));
  const { data: runsData } = useSuspenseQuery(runsQueryOptions());

  const [activeTab, setActiveTab] = useState("overview");
  const [dateRange, setDateRange] = useState<DateRange>("7d");
  const [selectedRun, setSelectedRun] = useState<JobRun | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});

  // Filter runs for this job
  const jobRuns = useMemo(
    () => (runsData?.data ?? []).filter((r) => r.job_id === job?.id),
    [runsData, job?.id]
  );

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

  const rangeDays = DATE_RANGES.find((r) => r.value === dateRange)?.days ?? 7;
  const chartData = useMemo(() => generateChartData(rangeDays), [rangeDays]);
  const stats = useMemo(() => computeStats(chartData), [chartData]);

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
      <div className="flex items-start justify-between pt-4 pb-4">
        <div className="flex flex-col gap-1.5">
          <div className="flex items-center gap-3">
            <h1 className="text-balance font-normal text-2xl tracking-tight">
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
          <Button size="sm">
            <HugeiconsIcon className="mr-1.5" icon={PlayActionIcon} size={14} />
            Trigger
          </Button>
          <Button size="sm" variant="outline">
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
          {/* Date range selector */}
          <div className="flex items-center gap-1">
            {DATE_RANGES.map((range) => (
              <Button
                key={range.value}
                onClick={() => setDateRange(range.value)}
                size="sm"
                variant={dateRange === range.value ? "default" : "outline"}
              >
                {range.label}
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

          {/* Run History Bar Chart */}
          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="font-medium text-sm">
                Run History (Last {rangeDays} Days)
              </CardTitle>
              <div className="flex items-center gap-1">
                {CHART_LEGEND.map((item) => (
                  <div
                    className="flex items-center gap-1.5 rounded-md px-2 py-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                    key={item.label}
                  >
                    <span
                      className="size-2 shrink-0 rounded-full"
                      style={{ backgroundColor: item.color }}
                    />
                    <span>{item.label}</span>
                  </div>
                ))}
              </div>
            </CardHeader>
            <CardContent>
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
                      dataKey="date"
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
                    <Bar
                      dataKey="completed"
                      fill={CHART_COLORS.success}
                      radius={[2, 2, 0, 0]}
                      stackId="runs"
                    />
                    <Bar
                      dataKey="failed"
                      fill={CHART_COLORS.error}
                      radius={[2, 2, 0, 0]}
                      stackId="runs"
                    />
                  </BarChart>
                </ResponsiveContainer>
              </div>
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
                          // TODO
                        },
                      },
                      {
                        label: "Cancel",
                        icon: XCircleIcon,
                        onClick: () => {
                          // TODO
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

function ConfigRow({
  icon,
  label,
  value,
}: {
  icon: any;
  label: string;
  value: string;
}) {
  return (
    <div className="flex items-center justify-between gap-2 text-sm">
      <div className="flex shrink-0 items-center gap-2">
        <HugeiconsIcon
          className="text-muted-foreground"
          icon={icon}
          size={14}
        />
        <span className="text-muted-foreground">{label}</span>
      </div>
      <span className="truncate font-mono text-xs">{value}</span>
    </div>
  );
}
