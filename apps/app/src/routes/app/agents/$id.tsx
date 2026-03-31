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
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { lazy, Suspense, useMemo, useState } from "react";
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import type { AgentCostSummary } from "@/components/agents/agent-cost-utils";
import { summarizeAgentRuns } from "@/components/agents/agent-detail-utils";
import ConfigRow from "@/components/common/config-row";
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import TableEmptyState from "@/components/common/table-empty-state";
import ChartTooltip from "@/components/dashboard/chart-tooltip";
import RunDetailSheet from "@/components/dashboard/run-detail-sheet";
import StatusBadge from "@/components/dashboard/status-badge";
import { createRunColumns } from "@/components/tables/runs-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import type { Agent, DisplayStatus, JobRun } from "@/hooks/api/types";
import { agentCostSummaryQueryOptions } from "@/hooks/api/use-agent-costs";
import {
  agentQueryOptions,
  agentRunsQueryOptions,
  agentVersionsQueryOptions,
  useDeployAgent,
  useRunAgent,
} from "@/hooks/api/use-agents";
import { formatMicroUsd } from "@/lib/format";
import {
  ActivityIcon,
  BriefcaseIcon,
  ClockIcon,
  PlayActionIcon,
  RefreshIcon,
  SparklesIcon,
  TagIcon,
} from "@/lib/icons";
import { CHART_COLORS } from "@/lib/status-colors";

const AgentVersionTimeline = lazy(
  () => import("@/components/agents/agent-version-timeline")
);

export const Route = createFileRoute("/app/agents/$id")({
  loader: async ({ context, params }) => {
    await Promise.all([
      context.queryClient.ensureQueryData(agentQueryOptions(params.id)),
      context.queryClient.ensureQueryData(
        agentRunsQueryOptions(params.id, { limit: 50 })
      ),
      context.queryClient.ensureQueryData(
        agentCostSummaryQueryOptions(params.id)
      ),
      context.queryClient.ensureQueryData(agentVersionsQueryOptions(params.id)),
    ]);
  },
  pendingComponent: DetailPageSkeleton,
  errorComponent: ErrorComponent,
  component: AgentDetailPage,
});

const formatDateTime = (value: string | undefined) =>
  value ? new Date(value).toLocaleString() : "-";

const COST_LABEL_MAP = {
  cost_microusd: {
    color: CHART_COLORS.active,
    format: formatMicroUsd,
    label: "Cost",
  },
};

const TOKEN_LABEL_MAP = {
  total_tokens: {
    color: CHART_COLORS.warning,
    label: "Tokens",
  },
};

const TOOL_LABEL_MAP = {
  count: {
    color: CHART_COLORS.warning,
    label: "Tool Calls",
  },
};

const formatDurationMs = (value: number) => {
  if (value <= 0) {
    return "-";
  }
  if (value < 1000) {
    return `${value}ms`;
  }
  if (value < 60_000) {
    return `${(value / 1000).toFixed(1)}s`;
  }
  return `${(value / 60_000).toFixed(1)}m`;
};

const StatCard = ({
  label,
  value,
}: {
  label: string;
  value: string | number;
}) => {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-muted-foreground text-sm">
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <p className="font-normal text-2xl tracking-tight">{value}</p>
      </CardContent>
    </Card>
  );
};

function AgentDetailPage() {
  const { id } = Route.useParams();
  const navigate = Route.useNavigate();
  const { data: agent } = useSuspenseQuery(agentQueryOptions(id)) as {
    data: Agent | undefined;
  };
  const { data: agentRuns } = useSuspenseQuery({
    ...agentRunsQueryOptions(id, { limit: 50 }),
    refetchInterval: 5000,
    refetchIntervalInBackground: true,
  }) as {
    data: JobRun[] | undefined;
  };
  const { data: costSummary } = useSuspenseQuery({
    ...agentCostSummaryQueryOptions(id),
    refetchInterval: 5000,
    refetchIntervalInBackground: true,
  }) as {
    data: AgentCostSummary;
  };
  const { data: versions } = useSuspenseQuery(
    agentVersionsQueryOptions(id)
  ) as { data: import("@/hooks/api/use-agents").AgentVersion[] | undefined };
  const deployAgent = useDeployAgent();
  const runAgent = useRunAgent();
  const [activeTab, setActiveTab] = useState("overview");
  const [selectedRun, setSelectedRun] = useState<JobRun | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});

  const runs = agentRuns ?? [];
  const summary = useMemo(() => summarizeAgentRuns(runs), [runs]);
  const liveRefresh = summary.activeRuns > 0;
  const runColumns = useMemo(
    () =>
      createRunColumns({
        onView: (run) => {
          setSelectedRun(run);
          setSheetOpen(true);
        },
      }),
    []
  );

  const table = useReactTable({
    data: runs,
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

  if (!agent) {
    return (
      <Shell>
        <EntityNotFound backTo="/app/agents" entity="Agent" />
      </Shell>
    );
  }

  const latestRun = summary.latestRun;

  return (
    <Shell>
      <div className="flex flex-col gap-3 pt-4 pb-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 flex-col gap-1.5">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="truncate font-normal text-xl tracking-tight sm:text-2xl">
              {agent.name}
            </h1>
            <Badge variant="outline">{agent.model}</Badge>
            {liveRefresh && <Badge variant="secondary">Live refresh: 5s</Badge>}
            <code className="rounded bg-muted px-2 py-0.5 font-mono text-xs">
              {agent.slug}
            </code>
          </div>
          {agent.description && (
            <p className="text-pretty text-muted-foreground text-sm">
              {agent.description}
            </p>
          )}
        </div>
        <div className="flex shrink-0 gap-2">
          <Button
            disabled={deployAgent.isPending}
            onClick={() => deployAgent.mutate({ agentId: agent.id })}
            size="sm"
            type="button"
            variant="outline"
          >
            <HugeiconsIcon className="mr-1.5" icon={RefreshIcon} size={14} />
            {deployAgent.isPending ? "Deploying..." : "Deploy"}
          </Button>
          <Button
            disabled={runAgent.isPending}
            onClick={() =>
              runAgent.mutate(
                { agentId: agent.id },
                {
                  onSuccess: (run) =>
                    navigate({
                      params: { id: (run as JobRun).id },
                      to: "/app/runs/$id",
                    }),
                }
              )
            }
            size="sm"
            type="button"
          >
            <HugeiconsIcon className="mr-1.5" icon={PlayActionIcon} size={14} />
            {runAgent.isPending ? "Starting..." : "Run agent"}
          </Button>
        </div>
      </div>

      <Tabs className="w-full" onValueChange={setActiveTab} value={activeTab}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="runs">Recent Runs</TabsTrigger>
          <TabsTrigger value="costs">Costs</TabsTrigger>
          <TabsTrigger value="versions">Versions</TabsTrigger>
          <TabsTrigger value="config">Config</TabsTrigger>
        </TabsList>

        <TabsContent className="mt-6 space-y-6" value="overview">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <StatCard label="Total Runs" value={summary.totalRuns} />
            <StatCard label="Successful" value={summary.successfulRuns} />
            <StatCard label="Failed" value={summary.failedRuns} />
            <StatCard label="Active" value={summary.activeRuns} />
          </div>

          <div className="grid gap-6 lg:grid-cols-[1.4fr_1fr]">
            <Card>
              <CardHeader>
                <CardTitle>Latest Activity</CardTitle>
              </CardHeader>
              <CardContent>
                {latestRun ? (
                  <div className="space-y-4">
                    <div className="flex items-center gap-2">
                      <StatusBadge
                        showDot
                        status={latestRun.status as DisplayStatus}
                      />
                      <span className="font-mono text-sm">{latestRun.id}</span>
                    </div>
                    <div className="grid gap-3 sm:grid-cols-2">
                      <ConfigRow
                        icon={ClockIcon}
                        label="Started"
                        value={formatDateTime(
                          latestRun.started_at ?? undefined
                        )}
                      />
                      <ConfigRow
                        icon={ClockIcon}
                        label="Created"
                        value={formatDateTime(latestRun.created_at)}
                      />
                      <ConfigRow
                        icon={ActivityIcon}
                        label="Attempt"
                        value={String(latestRun.attempt)}
                      />
                      <ConfigRow
                        icon={SparklesIcon}
                        label="Trigger"
                        value={latestRun.triggered_by}
                      />
                    </div>
                    <Button
                      render={
                        <Link
                          params={{ id: latestRun.id }}
                          to="/app/runs/$id"
                        />
                      }
                      size="sm"
                      variant="outline"
                    >
                      View run details
                    </Button>
                  </div>
                ) : (
                  <TableEmptyState
                    description="This agent has not been run yet. Deploy it and trigger the first local run."
                    hideButton
                    icon={
                      <HugeiconsIcon
                        className="size-6 text-foreground"
                        icon={SparklesIcon}
                      />
                    }
                    title="No runs yet"
                  />
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Definition</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <ConfigRow icon={TagIcon} label="Slug" value={agent.slug} />
                <ConfigRow
                  icon={SparklesIcon}
                  label="Model"
                  value={agent.model}
                />
                <ConfigRow
                  icon={BriefcaseIcon}
                  label="Backing job"
                  value={agent.job_id}
                />
                <ConfigRow
                  icon={ClockIcon}
                  label="Created"
                  value={formatDateTime(agent.created_at)}
                />
                <ConfigRow
                  icon={ClockIcon}
                  label="Updated"
                  value={formatDateTime(agent.updated_at)}
                />
                <ConfigRow
                  icon={ActivityIcon}
                  label="Average Duration"
                  value={formatDurationMs(costSummary.average_duration_ms)}
                />
                <ConfigRow
                  icon={SparklesIcon}
                  label="Average Tokens / Run"
                  value={costSummary.average_tokens_per_run.toLocaleString()}
                />
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent className="mt-6" value="runs">
          {/* biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noNoninteractiveElementInteractions lint/a11y/noStaticElementInteractions: event delegation on table container */}
          <div
            className="[&_tbody_tr]:cursor-pointer"
            onClick={(event) => {
              const target = event.target as HTMLElement;
              if (target.closest("a, button")) {
                return;
              }
              const row = target.closest("tr[data-row-index]");
              if (!row) {
                return;
              }
              const index = Number(row.getAttribute("data-row-index"));
              const run = runs[index];
              if (!run) {
                return;
              }
              setSelectedRun(run);
              setSheetOpen(true);
            }}
          >
            <DataTable<JobRun>
              emptyState={
                <TableEmptyState
                  description="Once this agent starts running, execution history will appear here."
                  hideButton
                  icon={
                    <HugeiconsIcon
                      className="size-6 text-foreground"
                      icon={ActivityIcon}
                    />
                  }
                  title="No recent runs"
                />
              }
              table={table}
            />
          </div>
        </TabsContent>

        <TabsContent className="mt-6 space-y-6" value="costs">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <StatCard
              label="Total AI Cost"
              value={formatMicroUsd(costSummary.total_cost_microusd)}
            />
            <StatCard
              label="Average / Run"
              value={formatMicroUsd(costSummary.average_cost_microusd)}
            />
            <StatCard
              label="Total Tokens"
              value={costSummary.total_tokens.toLocaleString()}
            />
            <StatCard
              label="Latest Run Cost"
              value={formatMicroUsd(costSummary.latest_run_cost_microusd)}
            />
            <StatCard
              label="Projected Monthly"
              value={formatMicroUsd(
                costSummary.forecast.projected_monthly_cost_microusd
              )}
            />
          </div>

          {costSummary.budget_limit_microusd ? (
            <Card>
              <CardHeader>
                <CardTitle>Budget Utilization</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <div className="flex items-center justify-between text-sm">
                  <span className="text-muted-foreground">Used</span>
                  <span className="font-medium">
                    {formatMicroUsd(costSummary.total_cost_microusd)} /{" "}
                    {formatMicroUsd(costSummary.budget_limit_microusd)}
                  </span>
                </div>
                <div className="h-2 overflow-hidden rounded-full bg-muted">
                  <div
                    className="h-full rounded-full bg-primary transition-[width]"
                    style={{
                      width: `${Math.min(
                        (costSummary.budget_utilization_ratio ?? 0) * 100,
                        100
                      )}%`,
                    }}
                  />
                </div>
              </CardContent>
            </Card>
          ) : null}

          <div className="grid gap-6 lg:grid-cols-[1.4fr_1fr]">
            <Card>
              <CardHeader>
                <CardTitle>Daily Cost Trend</CardTitle>
              </CardHeader>
              <CardContent>
                {costSummary.daily.length > 0 ? (
                  <div className="h-[260px]">
                    <ResponsiveContainer height="100%" width="100%">
                      <AreaChart data={costSummary.daily}>
                        <CartesianGrid
                          className="stroke-border"
                          strokeDasharray="3 3"
                        />
                        <XAxis dataKey="date" tickLine={false} />
                        <YAxis
                          tickFormatter={(value) => formatMicroUsd(value)}
                          tickLine={false}
                          width={90}
                        />
                        <Tooltip
                          content={<ChartTooltip labelMap={COST_LABEL_MAP} />}
                        />
                        <Area
                          dataKey="cost_microusd"
                          fill={CHART_COLORS.active}
                          fillOpacity={0.18}
                          stroke={CHART_COLORS.active}
                          strokeWidth={2}
                          type="monotone"
                        />
                      </AreaChart>
                    </ResponsiveContainer>
                  </div>
                ) : (
                  <TableEmptyState
                    description="Cost trend data will appear once runs report usage."
                    hideButton
                    icon={
                      <HugeiconsIcon
                        className="size-6 text-foreground"
                        icon={ActivityIcon}
                      />
                    }
                    title="No usage data yet"
                  />
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Provider Breakdown</CardTitle>
              </CardHeader>
              <CardContent>
                {costSummary.providers.length > 0 ? (
                  <div className="space-y-4">
                    <div className="h-[220px]">
                      <ResponsiveContainer height="100%" width="100%">
                        <BarChart data={costSummary.providers}>
                          <CartesianGrid
                            className="stroke-border"
                            strokeDasharray="3 3"
                          />
                          <XAxis dataKey="label" tickLine={false} />
                          <YAxis
                            tickFormatter={(value) => formatMicroUsd(value)}
                            tickLine={false}
                            width={90}
                          />
                          <Tooltip
                            content={<ChartTooltip labelMap={COST_LABEL_MAP} />}
                          />
                          <Bar
                            dataKey="cost_microusd"
                            fill={CHART_COLORS.active}
                            radius={[4, 4, 0, 0]}
                          />
                        </BarChart>
                      </ResponsiveContainer>
                    </div>
                    <div className="space-y-2">
                      {costSummary.providers.map((provider) => (
                        <div
                          className="flex items-center justify-between text-sm"
                          key={provider.label}
                        >
                          <span className="text-muted-foreground capitalize">
                            {provider.label}
                          </span>
                          <div className="flex items-center gap-3">
                            <span className="font-mono text-muted-foreground text-xs">
                              {provider.total_tokens.toLocaleString()} tokens
                            </span>
                            <span className="font-medium">
                              {formatMicroUsd(provider.cost_microusd)}
                            </span>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                ) : (
                  <TableEmptyState
                    description="Provider and token breakdowns will populate as usage records arrive."
                    hideButton
                    icon={
                      <HugeiconsIcon
                        className="size-6 text-foreground"
                        icon={SparklesIcon}
                      />
                    }
                    title="No provider data yet"
                  />
                )}
              </CardContent>
            </Card>
          </div>

          <div className="grid gap-6 lg:grid-cols-[1.2fr_1fr]">
            {costSummary.daily.length > 0 && (
              <Card>
                <CardHeader>
                  <CardTitle>Daily Token Volume</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="h-[220px]">
                    <ResponsiveContainer height="100%" width="100%">
                      <BarChart data={costSummary.daily}>
                        <CartesianGrid
                          className="stroke-border"
                          strokeDasharray="3 3"
                        />
                        <XAxis dataKey="date" tickLine={false} />
                        <YAxis tickLine={false} width={90} />
                        <Tooltip
                          content={<ChartTooltip labelMap={TOKEN_LABEL_MAP} />}
                        />
                        <Bar
                          dataKey="total_tokens"
                          fill={CHART_COLORS.warning}
                          radius={[4, 4, 0, 0]}
                        />
                      </BarChart>
                    </ResponsiveContainer>
                  </div>
                </CardContent>
              </Card>
            )}

            <Card>
              <CardHeader>
                <CardTitle>Tool Call Frequency</CardTitle>
              </CardHeader>
              <CardContent>
                {costSummary.tools.length > 0 ? (
                  <div className="space-y-4">
                    <div className="h-[220px]">
                      <ResponsiveContainer height="100%" width="100%">
                        <BarChart data={costSummary.tools}>
                          <CartesianGrid
                            className="stroke-border"
                            strokeDasharray="3 3"
                          />
                          <XAxis dataKey="tool_name" tickLine={false} />
                          <YAxis tickLine={false} width={70} />
                          <Tooltip
                            content={<ChartTooltip labelMap={TOOL_LABEL_MAP} />}
                          />
                          <Bar
                            dataKey="count"
                            fill={CHART_COLORS.warning}
                            radius={[4, 4, 0, 0]}
                          />
                        </BarChart>
                      </ResponsiveContainer>
                    </div>
                    <div className="space-y-2">
                      {costSummary.tools.slice(0, 5).map((tool) => (
                        <div
                          className="flex items-center justify-between text-sm"
                          key={tool.tool_name}
                        >
                          <span className="text-muted-foreground">
                            {tool.tool_name}
                          </span>
                          <div className="flex items-center gap-3">
                            <span className="font-mono text-muted-foreground text-xs">
                              avg {formatDurationMs(tool.average_duration_ms)}
                            </span>
                            <span className="font-medium">
                              {tool.count} calls
                            </span>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                ) : (
                  <TableEmptyState
                    description="Tool usage metrics will populate as runs emit tool-call telemetry."
                    hideButton
                    icon={
                      <HugeiconsIcon
                        className="size-6 text-foreground"
                        icon={SparklesIcon}
                      />
                    }
                    title="No tool data yet"
                  />
                )}
              </CardContent>
            </Card>
          </div>

          <div className="grid gap-6 lg:grid-cols-[1.2fr_1fr]">
            <Card>
              <CardHeader>
                <CardTitle>Model Spend</CardTitle>
              </CardHeader>
              <CardContent>
                {costSummary.models.length > 0 ? (
                  <div className="space-y-3">
                    {costSummary.models.map((model) => (
                      <div
                        className="flex items-center justify-between text-sm"
                        key={model.label}
                      >
                        <span className="text-muted-foreground">
                          {model.label}
                        </span>
                        <div className="flex items-center gap-3">
                          <span className="font-mono text-muted-foreground text-xs">
                            {model.total_tokens.toLocaleString()} tokens
                          </span>
                          <span className="font-medium">
                            {formatMicroUsd(model.cost_microusd)}
                          </span>
                        </div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <TableEmptyState
                    description="Per-model spend will appear as runs report usage."
                    hideButton
                    icon={
                      <HugeiconsIcon
                        className="size-6 text-foreground"
                        icon={SparklesIcon}
                      />
                    }
                    title="No model data yet"
                  />
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Run Cost Breakdown</CardTitle>
              </CardHeader>
              <CardContent>
                {costSummary.run_breakdown.length > 0 ? (
                  <div className="space-y-3">
                    {costSummary.run_breakdown.slice(0, 5).map((run) => (
                      <div className="rounded-md border p-3" key={run.run_id}>
                        <div className="flex items-center justify-between gap-3">
                          <Link
                            className="font-mono text-sm hover:underline"
                            params={{ id: run.run_id }}
                            to="/app/runs/$id"
                          >
                            {run.run_id}
                          </Link>
                          <StatusBadge status={run.status as DisplayStatus} />
                        </div>
                        <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-muted-foreground text-xs">
                          <span>{formatMicroUsd(run.cost_microusd)}</span>
                          <span>
                            {run.total_tokens.toLocaleString()} tokens
                          </span>
                          <span>{run.tool_calls} tools</span>
                          <span>{formatDurationMs(run.duration_ms)}</span>
                        </div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <TableEmptyState
                    description="Per-run cost breakdown will appear once the agent has usage records."
                    hideButton
                    icon={
                      <HugeiconsIcon
                        className="size-6 text-foreground"
                        icon={ActivityIcon}
                      />
                    }
                    title="No run cost data yet"
                  />
                )}
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent className="mt-6" value="versions">
          <Suspense
            fallback={
              <p className="text-muted-foreground text-sm">
                Loading versions...
              </p>
            }
          >
            <AgentVersionTimeline versions={versions ?? []} />
          </Suspense>
        </TabsContent>

        <TabsContent className="mt-6" value="config">
          <Card>
            <CardHeader>
              <CardTitle>Agent Config</CardTitle>
            </CardHeader>
            <CardContent>
              <pre className="max-h-[500px] overflow-auto whitespace-pre-wrap break-all rounded-lg bg-muted p-3 font-mono text-xs leading-relaxed sm:p-4">
                {agent.config
                  ? JSON.stringify(agent.config, null, 2)
                  : "No config provided"}
              </pre>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <RunDetailSheet
        onOpenChange={setSheetOpen}
        open={sheetOpen}
        run={selectedRun}
      />
    </Shell>
  );
}
