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
import { useMemo, useState } from "react";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import TableEmptyState from "@/components/common/table-empty-state";
import { ChartTooltip } from "@/components/dashboard/chart-tooltip";
import { StatusBadge } from "@/components/dashboard/status-badge";
import { DataTable } from "@/components/ui/data-table/data-table";
import type { Job } from "@/hooks/api/types";
import { jobQueryOptions } from "@/hooks/api/use-jobs";
import {
  ActivityIcon,
  ClockIcon,
  GlobeIcon,
  PauseActionIcon,
  PlayActionIcon,
  RefreshIcon,
  TagIcon,
} from "@/lib/icons";

export const Route = createFileRoute("/app/jobs/$id")({
  loader: async ({ context, params }) => {
    await context.queryClient.ensureQueryData(jobQueryOptions(params.id));
  },
  component: JobDetailPage,
});

// --- Mock data for the run history chart (last 7 days) ---

const CHART_DATA = (() => {
  const days: { date: string; success: number; failed: number }[] = [];
  const now = new Date();
  for (let i = 6; i >= 0; i--) {
    const d = new Date(now);
    d.setDate(d.getDate() - i);
    days.push({
      date: d.toLocaleDateString("en-US", { month: "short", day: "numeric" }),
      success: Math.floor(Math.random() * 40) + 60,
      failed: Math.floor(Math.random() * 6),
    });
  }
  return days;
})();

const CHART_LABEL_MAP = {
  success: { label: "Success", color: "var(--color-chart-1)" },
  failed: { label: "Failed", color: "var(--color-chart-4)" },
};

const CHART_LEGEND = [
  { label: "Success", color: "var(--color-chart-1)" },
  { label: "Failed", color: "var(--color-chart-4)" },
];

// --- Mock data for recent runs table ---

type JobRun = {
  id: string;
  status: "completed" | "failed" | "executing" | "queued";
  triggered_by: string;
  duration_ms: number;
  created_at: string;
};

const MOCK_RUNS: JobRun[] = [
  {
    id: "run_a1b2c3d4",
    status: "completed",
    triggered_by: "cron",
    duration_ms: 3200,
    created_at: new Date(Date.now() - 120_000).toISOString(),
  },
  {
    id: "run_e5f6g7h8",
    status: "completed",
    triggered_by: "cron",
    duration_ms: 4100,
    created_at: new Date(Date.now() - 3_600_000).toISOString(),
  },
  {
    id: "run_i9j0k1l2",
    status: "failed",
    triggered_by: "manual",
    duration_ms: 1800,
    created_at: new Date(Date.now() - 7_200_000).toISOString(),
  },
  {
    id: "run_m3n4o5p6",
    status: "completed",
    triggered_by: "api",
    duration_ms: 5400,
    created_at: new Date(Date.now() - 14_400_000).toISOString(),
  },
  {
    id: "run_q7r8s9t0",
    status: "completed",
    triggered_by: "cron",
    duration_ms: 2900,
    created_at: new Date(Date.now() - 28_800_000).toISOString(),
  },
  {
    id: "run_u1v2w3x4",
    status: "failed",
    triggered_by: "cron",
    duration_ms: 950,
    created_at: new Date(Date.now() - 43_200_000).toISOString(),
  },
  {
    id: "run_y5z6a7b8",
    status: "completed",
    triggered_by: "manual",
    duration_ms: 6200,
    created_at: new Date(Date.now() - 57_600_000).toISOString(),
  },
  {
    id: "run_c9d0e1f2",
    status: "completed",
    triggered_by: "cron",
    duration_ms: 3800,
    created_at: new Date(Date.now() - 72_000_000).toISOString(),
  },
];

function formatDuration(ms: number): string {
  if (ms < 1000) {
    return `${ms}ms`;
  }
  return `${(ms / 1000).toFixed(1)}s`;
}

const jobRunColumns: ColumnDef<JobRun>[] = [
  {
    accessorKey: "id",
    header: "Run ID",
    cell: ({ row }) => (
      <span className="font-mono text-xs">{row.original.id}</span>
    ),
  },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }) => <StatusBadge status={row.original.status} />,
  },
  {
    accessorKey: "triggered_by",
    header: "Triggered By",
    cell: ({ row }) => (
      <Badge className="capitalize" variant="outline">
        {row.original.triggered_by}
      </Badge>
    ),
  },
  {
    accessorKey: "duration_ms",
    header: "Duration",
    cell: ({ row }) => (
      <span className="font-mono text-xs">
        {formatDuration(row.original.duration_ms)}
      </span>
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

// --- Page component ---

function JobDetailPage() {
  const { id } = Route.useParams();
  const { data: job } = useSuspenseQuery(jobQueryOptions(id)) as {
    data: Job | null;
  };
  const [activeTab, setActiveTab] = useState("overview");

  const runsTable = useReactTable({
    data: MOCK_RUNS,
    columns: jobRunColumns,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
  });

  // Derive stats from mock chart data
  const stats = useMemo(() => {
    const totalSuccess = CHART_DATA.reduce((s, d) => s + d.success, 0);
    const totalFailed = CHART_DATA.reduce((s, d) => s + d.failed, 0);
    const totalRuns = totalSuccess + totalFailed;
    const successRate =
      totalRuns > 0 ? ((totalSuccess / totalRuns) * 100).toFixed(1) : "0";
    return {
      successRate: `${successRate}%`,
      totalRuns: totalRuns.toLocaleString(),
      avgDuration: "4.2s",
      failedRuns: totalFailed.toLocaleString(),
    };
  }, []);

  if (!job) {
    return (
      <Shell>
        <div className="flex items-center justify-center py-20">
          <p className="text-muted-foreground">Job not found.</p>
        </div>
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
          {/* Stats row */}
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <StatCard label="Success Rate" value={stats.successRate} />
            <StatCard label="Total Runs" value={stats.totalRuns} />
            <StatCard label="Avg Duration" value={stats.avgDuration} />
            <StatCard label="Failed Runs" value={stats.failedRuns} />
          </div>

          {/* Run History Chart */}
          <Card>
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="font-medium text-sm">
                Run History (Last 7 Days)
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
                  <AreaChart data={CHART_DATA}>
                    <defs>
                      <linearGradient
                        id="fillSuccess"
                        x1="0"
                        x2="0"
                        y1="0"
                        y2="1"
                      >
                        <stop
                          offset="5%"
                          stopColor="var(--color-chart-1)"
                          stopOpacity={0.3}
                        />
                        <stop
                          offset="95%"
                          stopColor="var(--color-chart-1)"
                          stopOpacity={0}
                        />
                      </linearGradient>
                      <linearGradient
                        id="fillFailed"
                        x1="0"
                        x2="0"
                        y1="0"
                        y2="1"
                      >
                        <stop
                          offset="5%"
                          stopColor="var(--color-chart-4)"
                          stopOpacity={0.3}
                        />
                        <stop
                          offset="95%"
                          stopColor="var(--color-chart-4)"
                          stopOpacity={0}
                        />
                      </linearGradient>
                    </defs>
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
                      cursor={{
                        stroke: "var(--muted-foreground)",
                        strokeDasharray: "3 3",
                      }}
                    />
                    <Area
                      dataKey="success"
                      fill="url(#fillSuccess)"
                      fillOpacity={1}
                      stroke="var(--color-chart-1)"
                      strokeWidth={2}
                      type="monotone"
                    />
                    <Area
                      dataKey="failed"
                      fill="url(#fillFailed)"
                      fillOpacity={1}
                      stroke="var(--color-chart-4)"
                      strokeWidth={2}
                      type="monotone"
                    />
                  </AreaChart>
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
          <DataTable
            emptyState={
              <TableEmptyState
                description="No runs found for this job."
                hideButton
                icon={
                  <HugeiconsIcon
                    className="size-6 text-primary"
                    icon={ActivityIcon}
                  />
                }
                title="No runs found"
              />
            }
            table={runsTable}
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
    <div className="flex items-center justify-between text-sm">
      <div className="flex items-center gap-2">
        <HugeiconsIcon
          className="text-muted-foreground"
          icon={icon}
          size={14}
        />
        <span className="text-muted-foreground">{label}</span>
      </div>
      <span className="font-mono text-xs">{value}</span>
    </div>
  );
}
