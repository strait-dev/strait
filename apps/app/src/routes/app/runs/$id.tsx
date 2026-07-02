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
import { CodeBlock } from "@strait/ui/components/code-block";
import { CopyButton } from "@strait/ui/components/copy-button";
import {
  DescriptionDetails,
  DescriptionList,
  DescriptionTerm,
} from "@strait/ui/components/description-list";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@strait/ui/components/empty";
import { ExecutionTraceBar } from "@strait/ui/components/execution-trace-bar";
import { Shell } from "@strait/ui/components/shell";
import { StatusBadge } from "@strait/ui/components/status-badge";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs";
import {
  Timeline,
  TimelineDate,
  TimelineHeader,
  TimelineIndicator,
  TimelineItem,
  TimelineSeparator,
  TimelineTitle,
} from "@strait/ui/components/timeline";
import {
  ToggleGroup,
  ToggleGroupItem,
} from "@strait/ui/components/toggle-group";
import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type {
  ExecutionTrace,
  JobRun,
  PaginatedResponse,
  RunEvent,
  RunStatus,
} from "@/hooks/api/types";
import {
  runEventsQueryOptions,
  runQueryOptions,
  useCancelRun,
  useRetryRun,
} from "@/hooks/api/use-runs";
import { useProjectPermissions } from "@/hooks/auth/use-project-permissions";
import { useIsHydrated } from "@/hooks/use-is-hydrated";
import { formatDuration } from "@/lib/format";
import { AlertCircleIcon, RefreshIcon, XCircleIcon } from "@/lib/icons";
import type { AppRouteContext } from "@/routes/app/layout";

export const Route = createFileRoute("/app/runs/$id")({
  loader: async ({ context, params }) => {
    const { session } = context as AppRouteContext;
    await Promise.all([
      context.queryClient.ensureQueryData(runQueryOptions(params.id)),
      context.queryClient
        .prefetchQuery(runEventsQueryOptions(params.id))
        .catch(() => undefined),
    ]);
    return { session };
  },
  head: () => ({ meta: [{ title: "Run details · Strait" }] }),
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

const LEVEL_FILTERS = ["all", "error", "warn", "info", "debug"] as const;
type LevelFilter = (typeof LEVEL_FILTERS)[number];

const EMPTY_ARRAY: never[] = [];

const EXECUTION_TRACE_SEGMENTS: {
  key: keyof ExecutionTrace;
  label: string;
  color: string;
}[] = [
  {
    key: "queue_wait_ms",
    label: "Queue wait",
    color: "var(--muted-foreground)",
  },
  { key: "dequeue_ms", label: "Dequeue", color: "var(--chart-1)" },
  { key: "dispatch_ms", label: "Dispatch", color: "var(--info)" },
  { key: "connect_ms", label: "Connect", color: "var(--primary)" },
  { key: "ttfb_ms", label: "TTFB", color: "var(--warning)" },
  { key: "transfer_ms", label: "Transfer", color: "var(--success)" },
];

function RunDetailPage() {
  const { id } = Route.useParams();
  const { session } = Route.useLoaderData();
  usePageEvent("run_detail_viewed", { run_id: id });
  const { data: run } = useSuspenseQuery(runQueryOptions(id)) as {
    data: JobRun | undefined;
  };
  const {
    data: eventsData,
    isError: eventsError,
    isLoading: eventsLoading,
  } = useQuery({
    ...runEventsQueryOptions(id),
    throwOnError: false,
  }) as {
    data: PaginatedResponse<RunEvent> | undefined;
    isError: boolean;
    isLoading: boolean;
  };
  const events = eventsData?.data ?? EMPTY_ARRAY;
  const [activeTab, setActiveTab] = useState("logs");
  const [cancelDialogOpen, setCancelDialogOpen] = useState(false);
  const isHydrated = useIsHydrated();
  const [levelFilter, setLevelFilter] = useState<LevelFilter>("all");
  const retryRun = useRetryRun();
  const cancelRun = useCancelRun();
  const { permissions } = useProjectPermissions(session.user.activeProjectId);

  const filteredEvents = (() => {
    if (levelFilter === "all") {
      return events;
    }
    return events.filter(
      (e) => (e.level ?? "info").toLowerCase() === levelFilter
    );
  })();

  const logText = filteredEvents
    .map(
      (e) =>
        `[${new Date(e.created_at).toISOString()}] [${(e.level ?? "info").toUpperCase()}] ${e.message}`
    )
    .join("\n");

  if (!run) {
    return (
      <Shell>
        <EntityNotFound backTo="/app/runs" entity="Run" />
      </Shell>
    );
  }

  const isFailed = FAILED_STATUSES.has(run.status as RunStatus);
  const isActive = ACTIVE_STATUSES.has(run.status as RunStatus);

  return (
    <Shell>
      {/* Header */}
      <div className="flex flex-col gap-3 pt-4 pb-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex flex-col gap-2 overflow-hidden">
          <div className="flex items-center gap-3">
            <h1 className="truncate text-balance font-mono font-normal text-xl tracking-tight">
              {run.id}
            </h1>
            <StatusBadge showDot status={run.status as RunStatus} />
          </div>
          <p className="text-pretty text-muted-foreground text-sm">
            Job:{" "}
            <span className="font-mono underline underline-offset-2">
              {run.job_id}
            </span>
            <span className="mx-2 text-muted-foreground">·</span>v
            {run.job_version}
            <span className="mx-2 text-muted-foreground">·</span>
            attempt {run.attempt}
            <span className="mx-2 text-muted-foreground">·</span>
            triggered by {run.triggered_by}
          </p>
        </div>
        <div className="flex shrink-0 gap-2">
          {isFailed && permissions.canWriteRuns && (
            <Button
              disabled={!isHydrated || retryRun.isPending}
              onClick={() => retryRun.mutate({ run_id: run.id })}
              variant="outline"
            >
              <HugeiconsIcon className="mr-1.5" icon={RefreshIcon} size={14} />
              {retryRun.isPending ? "Retrying…" : "Retry"}
            </Button>
          )}
          {isActive && permissions.canWriteRuns && cancelDialogOpen && (
            <>
              <Button
                disabled={!isHydrated || cancelRun.isPending}
                onClick={() => setCancelDialogOpen(false)}
                variant="outline"
              >
                Keep running
              </Button>
              <Button
                disabled={!isHydrated || cancelRun.isPending}
                onClick={() =>
                  cancelRun.mutate(
                    { run_id: run.id },
                    {
                      onSettled: () => setCancelDialogOpen(false),
                    }
                  )
                }
                variant="destructive"
              >
                {cancelRun.isPending ? "Canceling..." : "Cancel run"}
              </Button>
            </>
          )}
          {isActive && permissions.canWriteRuns && !cancelDialogOpen && (
            <Button
              disabled={!isHydrated || cancelRun.isPending}
              onClick={() => setCancelDialogOpen(true)}
              variant="outline"
            >
              <HugeiconsIcon className="mr-1.5" icon={XCircleIcon} size={14} />
              {cancelRun.isPending ? "Canceling..." : "Cancel"}
            </Button>
          )}
        </div>
      </div>

      {/* Dominant error banner */}
      {isFailed && run.error && (
        <Alert className="mb-6" variant="destructive">
          <HugeiconsIcon icon={AlertCircleIcon} size={20} />
          <AlertTitle className="text-base">
            {run.error_class ?? "Run failed"}
          </AlertTitle>
          <AlertDescription className="mt-1 font-mono text-sm">
            {run.error}
          </AlertDescription>
        </Alert>
      )}

      {/* Execution trace — above the fold */}
      {run.execution_trace && (
        <Card className="mb-6">
          <CardHeader>
            <CardTitle>Execution trace</CardTitle>
          </CardHeader>
          <CardContent>
            <ExecutionTraceBar
              formatValue={(value) => `${value}ms`}
              segments={EXECUTION_TRACE_SEGMENTS.map((segment) => ({
                label: segment.label,
                value: run.execution_trace?.[segment.key] ?? 0,
                color: segment.color,
              }))}
              total={run.execution_trace?.total_ms || 1}
            />
          </CardContent>
        </Card>
      )}

      {/* What happened — horizontal timeline */}
      <Card className="mb-6">
        <CardHeader>
          <CardTitle>What happened</CardTitle>
        </CardHeader>
        <CardContent>
          <RunTimeline run={run} />
        </CardContent>
      </Card>

      {/* Tabs */}
      <Tabs className="w-full" onValueChange={setActiveTab} value={activeTab}>
        <div className="flex items-center justify-between gap-3">
          <TabsList>
            <TabsTrigger value="logs">Logs ({events.length})</TabsTrigger>
            <TabsTrigger value="payload">Payload</TabsTrigger>
            <TabsTrigger value="response">Response</TabsTrigger>
          </TabsList>

          {activeTab === "logs" && events.length > 0 && (
            <div className="flex items-center gap-1">
              <ToggleGroup
                aria-label="Filter logs by level"
                emphasis="outline"
                onValueChange={(value) => {
                  const next = value[0];
                  if (next) {
                    setLevelFilter(next as LevelFilter);
                  }
                }}
                size="xs"
                value={[levelFilter]}
              >
                {LEVEL_FILTERS.map((lvl) => (
                  <ToggleGroupItem
                    aria-label={`Show ${lvl} logs`}
                    key={lvl}
                    value={lvl}
                  >
                    {lvl === "all" ? "All" : lvl}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
              <CopyButton className="ml-2" text={logText} variant="outline">
                Copy
              </CopyButton>
            </div>
          )}
        </div>

        <TabsContent className="mt-6" value="logs">
          <LogViewer
            events={filteredEvents}
            isError={eventsError}
            isLoading={eventsLoading}
            logText={logText}
          />
        </TabsContent>

        <TabsContent className="mt-6" value="payload">
          <CodeBlock
            code={
              run.payload ? JSON.stringify(run.payload, null, 2) : "No payload"
            }
            language="json"
            maxHeight={500}
            wrap
          />
        </TabsContent>

        <TabsContent className="mt-6" value="response">
          <CodeBlock
            code={
              run.result
                ? JSON.stringify(run.result, null, 2)
                : "No response data"
            }
            language="json"
            maxHeight={500}
            wrap
          />
        </TabsContent>
      </Tabs>
    </Shell>
  );
}

type TimelineStep = {
  label: string;
  at: string | null | undefined;
};

function formatTimelineTimestamp(value: string) {
  return `${new Date(value).toISOString().replace("T", " ").slice(0, 19)} UTC`;
}

function RunTimeline({ run }: { run: JobRun }) {
  const isHydrated = useIsHydrated();
  const steps: TimelineStep[] = [
    { label: "Created", at: run.created_at },
    { label: "Scheduled", at: run.scheduled_at },
    { label: "Started", at: run.started_at },
    { label: "Finished", at: run.finished_at },
  ];
  const activeStep = steps.filter((step) => !!step.at).length;
  const duration =
    run.finished_at || isHydrated
      ? formatDuration(run.started_at ?? null, run.finished_at ?? null)
      : "In progress";

  return (
    <div className="flex flex-col gap-4">
      <Timeline
        orientation="horizontal"
        size="sm"
        value={activeStep}
        variant="primary"
      >
        {steps.map((step, index) => (
          <TimelineItem key={step.label} step={index + 1}>
            <TimelineHeader>
              <TimelineSeparator />
              <TimelineIndicator />
              <TimelineDate dateTime={step.at ?? undefined}>
                {step.at ? formatTimelineTimestamp(step.at) : "—"}
              </TimelineDate>
            </TimelineHeader>
            <TimelineTitle>{step.label}</TimelineTitle>
          </TimelineItem>
        ))}
      </Timeline>

      <DescriptionList orientation="horizontal" size="sm">
        <DescriptionTerm>Duration</DescriptionTerm>
        <DescriptionDetails className="font-mono">
          {duration}
        </DescriptionDetails>
        <DescriptionTerm>Priority</DescriptionTerm>
        <DescriptionDetails className="font-mono">
          {run.priority}
        </DescriptionDetails>
        {run.execution_mode && (
          <>
            <DescriptionTerm>Mode</DescriptionTerm>
            <DescriptionDetails className="font-mono">
              {run.execution_mode}
            </DescriptionDetails>
          </>
        )}
        {run.queue_name && (
          <>
            <DescriptionTerm>Queue</DescriptionTerm>
            <DescriptionDetails className="font-mono">
              {run.queue_name}
            </DescriptionDetails>
          </>
        )}
      </DescriptionList>
    </div>
  );
}

function LogViewer({
  events,
  isError,
  isLoading,
  logText,
}: {
  events: RunEvent[];
  isError: boolean;
  isLoading: boolean;
  logText: string;
}) {
  if (isError) {
    return (
      <Empty border={false}>
        <EmptyHeader>
          <EmptyTitle>Log events unavailable</EmptyTitle>
          <EmptyDescription>
            Log events are unavailable right now.
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    );
  }

  if (events.length === 0) {
    return (
      <Empty border={false}>
        <EmptyHeader>
          <EmptyTitle>
            {isLoading ? "Loading log events" : "No log events"}
          </EmptyTitle>
          <EmptyDescription>
            {isLoading
              ? "Loading log events for this run."
              : "No log events for this run."}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    );
  }

  return <CodeBlock code={logText} maxHeight={500} wrap />;
}
