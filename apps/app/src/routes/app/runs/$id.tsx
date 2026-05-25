import { HugeiconsIcon } from "@hugeicons/react";

import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@strait/ui/components/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@strait/ui/components/alert-dialog";
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
import { cn } from "@strait/ui/utils/index";
import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import StatusBadge from "@/components/dashboard/status-badge";
import ExecutionTraceBar from "@/components/runs/execution-trace-bar";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type {
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
import { formatDuration } from "@/lib/format";
import {
  AlertCircleIcon,
  CopyIcon,
  RefreshIcon,
  XCircleIcon,
} from "@/lib/icons";

export const Route = createFileRoute("/app/runs/$id")({
  head: () => ({ meta: [{ title: "Run details · Strait" }] }),
  loader: async ({ context, params }) => {
    await context.queryClient.ensureQueryData(runQueryOptions(params.id));
    await context.queryClient
      .prefetchQuery(runEventsQueryOptions(params.id))
      .catch(() => undefined);
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

const LEVEL_FILTERS = ["all", "error", "warn", "info", "debug"] as const;
type LevelFilter = (typeof LEVEL_FILTERS)[number];

const LEVEL_COLOR: Record<string, string> = {
  error: "text-destructive",
  warn: "text-warning",
  warning: "text-warning",
  info: "text-info",
  debug: "text-muted-foreground/70",
};

function RunDetailPage() {
  const { id } = Route.useParams();
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
  const events = eventsData?.data ?? [];
  const [activeTab, setActiveTab] = useState("logs");
  const [levelFilter, setLevelFilter] = useState<LevelFilter>("all");
  const [copied, setCopied] = useState(false);
  const retryRun = useRetryRun();
  const cancelRun = useCancelRun();

  const filteredEvents = useMemo(() => {
    if (levelFilter === "all") {
      return events;
    }
    return events.filter(
      (e) => (e.level ?? "info").toLowerCase() === levelFilter
    );
  }, [events, levelFilter]);

  const logText = useMemo(
    () =>
      filteredEvents
        .map(
          (e) =>
            `[${new Date(e.created_at).toISOString()}] [${(e.level ?? "info").toUpperCase()}] ${e.message}`
        )
        .join("\n"),
    [filteredEvents]
  );

  const handleCopyLogs = async () => {
    try {
      await navigator.clipboard.writeText(logText);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard API unavailable; silently no-op.
    }
  };

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
            <span className="mx-2 text-muted-foreground/40">·</span>v
            {run.job_version}
            <span className="mx-2 text-muted-foreground/40">·</span>
            attempt {run.attempt}
            <span className="mx-2 text-muted-foreground/40">·</span>
            triggered by {run.triggered_by}
          </p>
        </div>
        <div className="flex shrink-0 gap-2">
          {isFailed && (
            <Button
              disabled={retryRun.isPending}
              onClick={() => retryRun.mutate({ run_id: run.id })}
              variant="outline"
            >
              <HugeiconsIcon className="mr-1.5" icon={RefreshIcon} size={14} />
              {retryRun.isPending ? "Retrying…" : "Retry"}
            </Button>
          )}
          {isActive && (
            <AlertDialog>
              <AlertDialogTrigger
                render={
                  <Button disabled={cancelRun.isPending} variant="outline">
                    <HugeiconsIcon
                      className="mr-1.5"
                      icon={XCircleIcon}
                      size={14}
                    />
                    {cancelRun.isPending ? "Cancelling…" : "Cancel"}
                  </Button>
                }
              />
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Cancel this run?</AlertDialogTitle>
                  <AlertDialogDescription>
                    The run is currently {run.status}. Cancelling stops
                    execution and marks the run as canceled. This cannot be
                    undone.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Keep running</AlertDialogCancel>
                  <AlertDialogAction
                    onClick={() => cancelRun.mutate({ run_id: run.id })}
                  >
                    Cancel run
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          )}
        </div>
      </div>

      {/* Dominant error banner */}
      {isFailed && run.error && (
        <Alert
          className="mb-6 border-l-4 border-l-destructive"
          variant="destructive"
        >
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
            <ExecutionTraceBar trace={run.execution_trace} />
          </CardContent>
        </Card>
      )}

      {/* What happened — horizontal timeline */}
      <Card className="mb-6">
        <CardHeader>
          <CardTitle>What happened</CardTitle>
        </CardHeader>
        <CardContent>
          <Timeline run={run} />
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
              <div className="flex items-center rounded-md border bg-card p-0.5">
                {LEVEL_FILTERS.map((lvl) => (
                  <button
                    className={cn(
                      "rounded-sm px-2 py-1 font-medium text-xs uppercase transition-colors",
                      levelFilter === lvl
                        ? "bg-accent text-foreground"
                        : "text-muted-foreground hover:text-foreground"
                    )}
                    key={lvl}
                    onClick={() => setLevelFilter(lvl)}
                    type="button"
                  >
                    {lvl === "all" ? "All" : lvl}
                  </button>
                ))}
              </div>
              <Button
                className="ml-2"
                onClick={handleCopyLogs}
                size="sm"
                variant="outline"
              >
                <HugeiconsIcon className="mr-1.5" icon={CopyIcon} size={14} />
                {copied ? "Copied" : "Copy"}
              </Button>
            </div>
          )}
        </div>

        <TabsContent className="mt-6" value="logs">
          <LogViewer
            events={filteredEvents}
            isError={eventsError}
            isLoading={eventsLoading}
          />
        </TabsContent>

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

type TimelineStep = {
  label: string;
  at: string | null | undefined;
};

function formatTimelineTimestamp(value: string) {
  return `${new Date(value).toISOString().replace("T", " ").slice(0, 19)} UTC`;
}

function Timeline({ run }: { run: JobRun }) {
  const steps: TimelineStep[] = [
    { label: "Created", at: run.created_at },
    { label: "Scheduled", at: run.scheduled_at },
    { label: "Started", at: run.started_at },
    { label: "Finished", at: run.finished_at },
  ];
  const duration = formatDuration(
    run.started_at ?? null,
    run.finished_at ?? null
  );

  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        {steps.map((s, i) => {
          const done = !!s.at;
          const isCurrent =
            done && (i === steps.length - 1 || !steps[i + 1]?.at);
          return (
            <div className="relative flex flex-col gap-1" key={s.label}>
              <div className="flex items-center gap-2">
                <span
                  aria-hidden
                  className={cn(
                    "size-2 rounded-full",
                    done ? "bg-primary" : "bg-muted-foreground/30",
                    isCurrent && "ring-4 ring-primary/20"
                  )}
                />
                <span className="font-medium text-foreground text-xs">
                  {s.label}
                </span>
              </div>
              <span className="pl-4 font-mono text-muted-foreground text-xs">
                {s.at ? formatTimelineTimestamp(s.at) : "—"}
              </span>
              {i < steps.length - 1 && (
                <span
                  aria-hidden
                  className="absolute top-[7px] left-[14px] -z-10 hidden h-px w-full bg-border sm:block"
                />
              )}
            </div>
          );
        })}
      </div>

      <div className="flex flex-wrap gap-x-6 gap-y-1 border-t pt-3 text-xs">
        <span className="text-muted-foreground">
          Duration{" "}
          <span className="ml-1 font-mono text-foreground">{duration}</span>
        </span>
        <span className="text-muted-foreground">
          Priority{" "}
          <span className="ml-1 font-mono text-foreground">{run.priority}</span>
        </span>
        {run.execution_mode && (
          <span className="text-muted-foreground">
            Mode{" "}
            <span className="ml-1 font-mono text-foreground">
              {run.execution_mode}
            </span>
          </span>
        )}
        {run.queue_name && (
          <span className="text-muted-foreground">
            Queue{" "}
            <span className="ml-1 font-mono text-foreground">
              {run.queue_name}
            </span>
          </span>
        )}
        {run.singleton_key && (
          <span className="text-muted-foreground">
            Key{" "}
            <span className="ml-1 font-mono text-foreground">
              {run.singleton_key}
            </span>
          </span>
        )}
      </div>
    </div>
  );
}

function LogViewer({
  events,
  isError,
  isLoading,
}: {
  events: RunEvent[];
  isError: boolean;
  isLoading: boolean;
}) {
  if (isError) {
    return (
      <div
        className="rounded-lg bg-muted p-6 text-center text-muted-foreground text-sm"
        role="status"
      >
        Log events are unavailable right now.
      </div>
    );
  }

  if (events.length === 0) {
    return (
      <div className="rounded-lg bg-muted p-6 text-center text-muted-foreground text-sm">
        {isLoading ? "Loading log events..." : "No log events for this run."}
      </div>
    );
  }

  return (
    <div className="max-h-[500px] overflow-auto rounded-lg bg-muted font-mono text-xs leading-relaxed">
      <ol className="divide-y divide-border/50">
        {events.map((evt) => {
          const level = (evt.level ?? "info").toLowerCase();
          const levelClass = LEVEL_COLOR[level] ?? "text-muted-foreground";
          return (
            <li
              className="grid grid-cols-[auto_4rem_1fr] gap-3 px-3 py-1.5 sm:px-4"
              key={evt.id}
            >
              <span className="text-muted-foreground/70">
                {new Date(evt.created_at).toISOString().slice(11, 23)}
              </span>
              <span className={cn("font-medium uppercase", levelClass)}>
                {level}
              </span>
              <span className="whitespace-pre-wrap break-words text-foreground">
                {evt.message}
              </span>
            </li>
          );
        })}
      </ol>
    </div>
  );
}
