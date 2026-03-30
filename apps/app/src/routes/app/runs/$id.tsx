import { HugeiconsIcon } from "@hugeicons/react";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@strait/ui/components/alert";
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
import { useEffect, useRef, useState } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import {
  buildRunTimeline,
  type RunTimelineItem,
  summarizeRunDebugBundle,
} from "@/components/agents/run-debug-utils";
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import ChartTooltip from "@/components/dashboard/chart-tooltip";
import StatusBadge from "@/components/dashboard/status-badge";
import DetailCell from "@/components/runs/detail-cell";
import ExecutionTraceBar from "@/components/runs/execution-trace-bar";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { DebugBundle, RunStatus } from "@/hooks/api/types";
import { runDebugBundleQueryOptions } from "@/hooks/api/use-runs";
import { useAgentStream } from "@/hooks/use-agent-stream";
import { formatDuration, formatMicroUsd } from "@/lib/format";
import { AlertCircleIcon, RefreshIcon, XCircleIcon } from "@/lib/icons";
import { CHART_COLORS } from "@/lib/status-colors";

export const Route = createFileRoute("/app/runs/$id")({
  loader: async ({ context, params }) => {
    await context.queryClient.ensureQueryData(
      runDebugBundleQueryOptions(params.id)
    );
  },
  pendingComponent: DetailPageSkeleton,
  errorComponent: ErrorComponent,
  component: RunDetailPage,
});

const FAILED_STATUSES: ReadonlySet<RunStatus> = new Set([
  "failed",
  "crashed",
  "system_failed",
  "timed_out",
]);

const ACTIVE_STATUSES: ReadonlySet<RunStatus> = new Set([
  "executing",
  "queued",
  "waiting",
]);

const USAGE_LABEL_MAP = {
  cost_microusd: {
    color: CHART_COLORS.active,
    format: formatMicroUsd,
    label: "Cost",
  },
};

type UsageSeriesEntry = {
  cost_microusd: number;
  label: string;
};

function getTimelineItemKey(item: RunTimelineItem): string {
  if (item.kind === "checkpoint") {
    return `${item.kind}-${item.sequence}`;
  }
  if (item.kind === "event") {
    return `${item.kind}-${item.created_at}-${item.type}-${item.message}`;
  }
  if (item.kind === "tool_call") {
    return `${item.kind}-${item.created_at}-${item.tool_name}-${item.status}`;
  }
  return `${item.kind}-${item.created_at}-${item.provider}-${item.model}-${item.cost_microusd}`;
}

function TimelineRow({ item }: { item: RunTimelineItem }) {
  if (item.kind === "event") {
    return (
      <div className="rounded-md border p-3">
        <div className="flex items-center justify-between gap-3">
          <div>
            <p className="font-medium text-sm">{item.label}</p>
            <p className="text-muted-foreground text-xs">{item.message}</p>
          </div>
          <span className="font-mono text-muted-foreground text-xs">
            {new Date(item.created_at).toLocaleTimeString()}
          </span>
        </div>
      </div>
    );
  }

  if (item.kind === "checkpoint") {
    return (
      <div className="rounded-md border p-3">
        <div className="flex items-center justify-between gap-3">
          <p className="font-medium text-sm">{item.label}</p>
          <span className="font-mono text-muted-foreground text-xs">
            {new Date(item.created_at).toLocaleTimeString()}
          </span>
        </div>
        <pre className="mt-2 whitespace-pre-wrap break-all rounded bg-muted p-2 font-mono text-xs">
          {JSON.stringify(item.state, null, 2)}
        </pre>
      </div>
    );
  }

  if (item.kind === "tool_call") {
    return (
      <div className="rounded-md border p-3">
        <div className="flex items-center justify-between gap-3">
          <div>
            <p className="font-medium text-sm">{item.tool_name}</p>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-xs">
              <span className="text-muted-foreground">{item.status}</span>
              {item.sandbox_executor ? (
                <span className="rounded-full border px-2 py-0.5 font-mono text-[11px]">
                  {item.sandbox_executor}
                </span>
              ) : null}
              {item.outbound_reason ? (
                <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 font-mono text-[11px] text-amber-700 dark:text-amber-300">
                  {item.outbound_reason}
                </span>
              ) : null}
            </div>
          </div>
          <span className="font-mono text-muted-foreground text-xs">
            {item.duration_ms ? `${item.duration_ms}ms` : "-"}
          </span>
        </div>
      </div>
    );
  }

  return (
    <div className="rounded-md border p-3">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="font-medium text-sm">{item.label}</p>
          <p className="text-muted-foreground text-xs">
            {item.total_tokens.toLocaleString()} tokens
          </p>
        </div>
        <span className="font-mono text-muted-foreground text-xs">
          {formatMicroUsd(item.cost_microusd)}
        </span>
      </div>
    </div>
  );
}

function RunSummaryCards({
  blockedToolCallCount,
  checkpointCount,
  toolCallCount,
  totalCostMicrousd,
  totalTokens,
}: {
  blockedToolCallCount: number;
  checkpointCount: number;
  toolCallCount: number;
  totalCostMicrousd: number;
  totalTokens: number;
}) {
  return (
    <div className="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-5">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="font-medium text-muted-foreground text-sm">
            Total Cost
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-2xl">{formatMicroUsd(totalCostMicrousd)}</p>
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="font-medium text-muted-foreground text-sm">
            Total Tokens
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-2xl">{totalTokens.toLocaleString()}</p>
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="font-medium text-muted-foreground text-sm">
            Checkpoints
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-2xl">{checkpointCount}</p>
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="font-medium text-muted-foreground text-sm">
            Tool Calls
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-2xl">{toolCallCount.toLocaleString()}</p>
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="font-medium text-muted-foreground text-sm">
            Blocked Calls
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-2xl">{blockedToolCallCount.toLocaleString()}</p>
        </CardContent>
      </Card>
    </div>
  );
}

function RunTelemetryTab({
  blockedReasonBreakdown,
  blockedToolCallCount,
  isActive,
  latestCheckpoint,
  modelBreakdown,
  runId,
  toolDetails,
  toolBreakdown,
  toolExecutorBreakdown,
  usageSeries,
}: {
  blockedReasonBreakdown: ReturnType<
    typeof summarizeRunDebugBundle
  >["blocked_reason_breakdown"];
  blockedToolCallCount: number;
  isActive: boolean;
  latestCheckpoint: unknown;
  modelBreakdown: ReturnType<typeof summarizeRunDebugBundle>["model_breakdown"];
  runId: string;
  toolDetails: ReturnType<typeof summarizeRunDebugBundle>["tool_details"];
  toolBreakdown: ReturnType<typeof summarizeRunDebugBundle>["tool_breakdown"];
  toolExecutorBreakdown: ReturnType<
    typeof summarizeRunDebugBundle
  >["tool_executor_breakdown"];
  usageSeries: UsageSeriesEntry[];
}) {
  return (
    <TabsContent className="mt-6 space-y-6" value="telemetry">
      <div className="grid gap-6 lg:grid-cols-[1.2fr_1fr]">
        <Card>
          <CardHeader>
            <CardTitle>LLM Call Cost</CardTitle>
          </CardHeader>
          <CardContent>
            {usageSeries.length > 0 ? (
              <div className="h-[260px]">
                <ResponsiveContainer height="100%" width="100%">
                  <BarChart data={usageSeries}>
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
                      content={<ChartTooltip labelMap={USAGE_LABEL_MAP} />}
                    />
                    <Bar
                      dataKey="cost_microusd"
                      fill={CHART_COLORS.active}
                      radius={[4, 4, 0, 0]}
                    />
                  </BarChart>
                </ResponsiveContainer>
              </div>
            ) : (
              <p className="text-muted-foreground text-sm">
                No usage records were emitted for this run.
              </p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Model Breakdown</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {modelBreakdown.length > 0 ? (
              modelBreakdown.map((entry) => (
                <div
                  className="flex items-center justify-between text-sm"
                  key={entry.label}
                >
                  <span className="text-muted-foreground">{entry.label}</span>
                  <div className="flex items-center gap-3">
                    <span className="font-mono text-muted-foreground text-xs">
                      {entry.total_tokens.toLocaleString()} tokens
                    </span>
                    <span className="font-medium">
                      {formatMicroUsd(entry.cost_microusd)}
                    </span>
                  </div>
                </div>
              ))
            ) : (
              <p className="text-muted-foreground text-sm">
                No model usage recorded.
              </p>
            )}
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-6 lg:grid-cols-[1.1fr_1fr]">
        <Card>
          <CardHeader>
            <CardTitle>Tool Calls</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {toolBreakdown.length > 0 ? (
              toolBreakdown.map((tool) => (
                <div className="rounded-md border p-3" key={tool.tool_name}>
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <span className="font-medium text-sm">
                        {tool.tool_name}
                      </span>
                      <div className="mt-1 flex flex-wrap gap-2">
                        {tool.executors.map((executor) => (
                          <span
                            className="rounded-full border px-2 py-0.5 font-mono text-[11px]"
                            key={`${tool.tool_name}-${executor}`}
                          >
                            {executor}
                          </span>
                        ))}
                      </div>
                    </div>
                    <div className="text-right">
                      <span className="font-mono text-muted-foreground text-xs">
                        {tool.count} calls
                      </span>
                    </div>
                  </div>
                  <p className="mt-1 text-muted-foreground text-xs">
                    {tool.failed_count} non-completed, {tool.blocked_count}{" "}
                    blocked
                  </p>
                </div>
              ))
            ) : (
              <p className="text-muted-foreground text-sm">
                No tool calls recorded.
              </p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Checkpoint State</CardTitle>
          </CardHeader>
          <CardContent>
            {latestCheckpoint ? (
              <pre className="max-h-[260px] overflow-auto whitespace-pre-wrap break-all rounded-lg bg-muted p-3 font-mono text-xs leading-relaxed">
                {JSON.stringify(latestCheckpoint, null, 2)}
              </pre>
            ) : (
              <p className="text-muted-foreground text-sm">
                No checkpoints were recorded.
              </p>
            )}
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-6 lg:grid-cols-[0.9fr_1.1fr]">
        <Card>
          <CardHeader>
            <CardTitle>Sandbox Execution</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="rounded-md border p-3">
              <p className="font-medium text-sm">Blocked Tool Calls</p>
              <p className="mt-1 text-2xl">{blockedToolCallCount}</p>
            </div>

            <div className="space-y-3">
              <p className="font-medium text-sm">Executor Breakdown</p>
              {toolExecutorBreakdown.length > 0 ? (
                toolExecutorBreakdown.map((entry) => (
                  <div
                    className="flex items-center justify-between text-sm"
                    key={entry.executor}
                  >
                    <span className="font-mono text-muted-foreground">
                      {entry.executor}
                    </span>
                    <span className="text-muted-foreground">
                      {entry.count} calls, {entry.blocked_count} blocked
                    </span>
                  </div>
                ))
              ) : (
                <p className="text-muted-foreground text-sm">
                  No sandbox execution data recorded.
                </p>
              )}
            </div>

            <div className="space-y-3">
              <p className="font-medium text-sm">Blocked Reasons</p>
              {blockedReasonBreakdown.length > 0 ? (
                blockedReasonBreakdown.map((entry) => (
                  <div
                    className="flex items-center justify-between text-sm"
                    key={entry.reason}
                  >
                    <span className="font-mono text-muted-foreground">
                      {entry.reason}
                    </span>
                    <span>{entry.count}</span>
                  </div>
                ))
              ) : (
                <p className="text-muted-foreground text-sm">
                  No blocked sandbox calls recorded.
                </p>
              )}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Recent Tool Activity</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {toolDetails.length > 0 ? (
              toolDetails.slice(0, 8).map((tool) => (
                <div
                  className="rounded-md border p-3"
                  key={`${tool.created_at}-${tool.tool_name}`}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <p className="font-medium text-sm">{tool.tool_name}</p>
                      <div className="mt-1 flex flex-wrap gap-2 text-xs">
                        <span className="text-muted-foreground">
                          {tool.status}
                        </span>
                        {tool.sandbox_executor ? (
                          <span className="rounded-full border px-2 py-0.5 font-mono text-[11px]">
                            {tool.sandbox_executor}
                          </span>
                        ) : null}
                        {tool.outbound_reason ? (
                          <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 font-mono text-[11px] text-amber-700 dark:text-amber-300">
                            {tool.outbound_reason}
                          </span>
                        ) : null}
                      </div>
                    </div>
                    <div className="text-right text-muted-foreground text-xs">
                      <div>
                        {tool.duration_ms ? `${tool.duration_ms}ms` : "-"}
                      </div>
                      <div>
                        {new Date(tool.created_at).toLocaleTimeString()}
                      </div>
                    </div>
                  </div>
                </div>
              ))
            ) : (
              <p className="text-muted-foreground text-sm">
                No recent tool activity recorded.
              </p>
            )}
          </CardContent>
        </Card>
      </div>

      <AgentStreamCard isActive={isActive} runId={runId} />
    </TabsContent>
  );
}

function AgentStreamCard({
  runId,
  isActive,
}: {
  runId: string;
  isActive: boolean;
}) {
  const stream = useAgentStream(runId, isActive);
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          Streaming
          {stream.connected && (
            <span className="size-2 rounded-full bg-green-500" />
          )}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {!isActive && stream.chunks.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            Live streaming is available while a run is executing.
          </p>
        ) : (
          <div
            className="max-h-64 overflow-y-auto whitespace-pre-wrap rounded bg-muted p-3 font-mono text-sm"
            ref={scrollRef}
          >
            {stream.chunks.length > 0 ? (
              stream.chunks.map((chunk, i) => (
                <span
                  key={`${runId}-chunk-${
                    // biome-ignore lint/suspicious/noArrayIndexKey: stream chunks are append-only
                    i
                  }`}
                >
                  {chunk}
                </span>
              ))
            ) : (
              <span className="text-muted-foreground">
                Waiting for stream data...
              </span>
            )}
          </div>
        )}
        {stream.error && (
          <p className="mt-2 text-destructive text-sm">{stream.error}</p>
        )}
      </CardContent>
    </Card>
  );
}

function RunDetailPage() {
  const { id } = Route.useParams();
  usePageEvent("run_detail_viewed", { run_id: id });
  const { data: bundle } = useSuspenseQuery({
    ...runDebugBundleQueryOptions(id),
    refetchInterval: (query) => {
      const nextBundle = query.state.data as DebugBundle | undefined;
      const nextRun = nextBundle?.run;
      return nextRun && ACTIVE_STATUSES.has(nextRun.status as RunStatus)
        ? 3000
        : false;
    },
    refetchIntervalInBackground: true,
  }) as {
    data: DebugBundle | undefined;
  };
  const [activeTab, setActiveTab] = useState("timeline");

  if (!bundle) {
    return (
      <Shell>
        <EntityNotFound backTo="/app/runs" entity="Run" />
      </Shell>
    );
  }

  const run = bundle.run;
  const isFailed = FAILED_STATUSES.has(run.status as RunStatus);
  const isActive = ACTIVE_STATUSES.has(run.status as RunStatus);
  const summary = summarizeRunDebugBundle(bundle);
  const timeline = buildRunTimeline(bundle);
  const usageSeries = (bundle.usage ?? []).map((usage, index) => ({
    cost_microusd: usage.cost_microusd,
    label: `${index + 1}`,
  }));

  return (
    <Shell>
      <div className="flex flex-col gap-3 pt-4 pb-6 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex flex-col gap-2 overflow-hidden">
          <div className="flex items-center gap-3">
            <h1 className="truncate text-balance font-mono font-normal text-lg tracking-tight sm:text-xl">
              {run.id}
            </h1>
            <StatusBadge showDot status={run.status as RunStatus} />
            {isActive ? (
              <span className="text-muted-foreground text-xs">
                Live refresh: 3s
              </span>
            ) : null}
          </div>
          <p className="text-pretty text-muted-foreground text-sm">
            Job:{" "}
            <span className="font-mono underline underline-offset-2">
              {run.job_id}
            </span>
          </p>
        </div>
        <div className="flex shrink-0 gap-2">
          {isFailed && (
            <Button size="sm" variant="outline">
              <HugeiconsIcon className="mr-1.5" icon={RefreshIcon} size={14} />
              Retry
            </Button>
          )}
          {isActive && (
            <Button size="sm" variant="outline">
              <HugeiconsIcon className="mr-1.5" icon={XCircleIcon} size={14} />
              Cancel
            </Button>
          )}
        </div>
      </div>

      {isFailed && run.error && (
        <Alert className="mb-6" variant="destructive">
          <HugeiconsIcon icon={AlertCircleIcon} size={16} />
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{run.error}</AlertDescription>
        </Alert>
      )}

      <RunSummaryCards
        blockedToolCallCount={summary.blocked_tool_call_count}
        checkpointCount={summary.checkpoint_count}
        toolCallCount={bundle.tool_calls?.length ?? 0}
        totalCostMicrousd={summary.total_cost_microusd}
        totalTokens={summary.total_tokens}
      />

      <Card className="mb-6">
        <CardHeader>
          <CardTitle>Execution Overview</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
            <DetailCell label="Status" value={run.status} />
            <DetailCell
              label="Duration"
              value={formatDuration(
                run.started_at ?? null,
                run.finished_at ?? null
              )}
            />
            <DetailCell
              label="Started"
              value={
                run.started_at ? new Date(run.started_at).toLocaleString() : "-"
              }
            />
            <DetailCell
              label="Completed"
              value={
                run.finished_at
                  ? new Date(run.finished_at).toLocaleString()
                  : "-"
              }
            />
            <DetailCell label="Triggered By" value={run.triggered_by} />
            <DetailCell label="Attempt" value={String(run.attempt)} />
            <DetailCell label="Job Version" value={String(run.job_version)} />
            <DetailCell label="Priority" value={String(run.priority)} />
          </div>
        </CardContent>
      </Card>

      {run.execution_trace && (
        <Card className="mb-6">
          <CardHeader>
            <CardTitle>Execution Trace</CardTitle>
          </CardHeader>
          <CardContent>
            <ExecutionTraceBar trace={run.execution_trace} />
          </CardContent>
        </Card>
      )}

      <Tabs className="w-full" onValueChange={setActiveTab} value={activeTab}>
        <TabsList>
          <TabsTrigger value="timeline">Timeline</TabsTrigger>
          <TabsTrigger value="telemetry">Telemetry</TabsTrigger>
          <TabsTrigger value="payload">Payload</TabsTrigger>
          <TabsTrigger value="response">Response</TabsTrigger>
        </TabsList>

        <TabsContent className="mt-6 space-y-6" value="timeline">
          <Card>
            <CardHeader>
              <CardTitle>Run Timeline</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {timeline.length > 0 ? (
                timeline.map((item) => (
                  <TimelineRow item={item} key={getTimelineItemKey(item)} />
                ))
              ) : (
                <p className="text-muted-foreground text-sm">
                  No timeline records were captured for this run.
                </p>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Event Log</CardTitle>
            </CardHeader>
            <CardContent>
              <pre className="max-h-[400px] overflow-auto whitespace-pre-wrap break-all rounded-lg bg-muted p-3 font-mono text-xs leading-relaxed sm:p-4">
                {bundle.events && bundle.events.length > 0
                  ? bundle.events
                      .map(
                        (evt) =>
                          `[${new Date(evt.created_at).toISOString()}] [${(evt.level ?? "info").toUpperCase()}] ${evt.message}`
                      )
                      .join("\n")
                  : "No log events available for this run."}
              </pre>
            </CardContent>
          </Card>
        </TabsContent>

        <RunTelemetryTab
          blockedReasonBreakdown={summary.blocked_reason_breakdown}
          blockedToolCallCount={summary.blocked_tool_call_count}
          isActive={isActive}
          latestCheckpoint={summary.latest_checkpoint}
          runId={run.id}
          modelBreakdown={summary.model_breakdown}
          toolBreakdown={summary.tool_breakdown}
          toolDetails={summary.tool_details}
          toolExecutorBreakdown={summary.tool_executor_breakdown}
          usageSeries={usageSeries}
        />

        <TabsContent className="mt-6" value="payload">
          <pre className="max-h-[500px] overflow-auto whitespace-pre-wrap break-all rounded-lg bg-muted p-3 font-mono text-xs leading-relaxed sm:p-4">
            {run.payload ? JSON.stringify(run.payload, null, 2) : "No payload"}
          </pre>
        </TabsContent>

        <TabsContent className="mt-6" value="response">
          <pre className="max-h-[500px] overflow-auto whitespace-pre-wrap break-all rounded-lg bg-muted p-3 font-mono text-xs leading-relaxed sm:p-4">
            {run.result
              ? JSON.stringify(run.result, null, 2)
              : "No response data"}
          </pre>
        </TabsContent>
      </Tabs>
    </Shell>
  );
}
