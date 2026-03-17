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
import { useState } from "react";
import { StatusBadge } from "@/components/dashboard/status-badge";
import type {
  ExecutionTrace,
  JobRun,
  RunEvent,
  RunStatus,
} from "@/hooks/api/types";
import { runEventsQueryOptions, runQueryOptions } from "@/hooks/api/use-runs";
import { AlertCircleIcon, RefreshIcon, XCircleIcon } from "@/lib/icons";

export const Route = createFileRoute("/app/runs/$id")({
  loader: async ({ context, params }) => {
    await Promise.all([
      context.queryClient.ensureQueryData(runQueryOptions(params.id)),
      context.queryClient.ensureQueryData(runEventsQueryOptions(params.id)),
    ]);
  },
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

function RunDetailPage() {
  const { id } = Route.useParams();
  const { data: run } = useSuspenseQuery(runQueryOptions(id)) as {
    data: JobRun | null;
  };
  const { data: events } = useSuspenseQuery(runEventsQueryOptions(id)) as {
    data: RunEvent[];
  };
  const [activeTab, setActiveTab] = useState("logs");

  if (!run) {
    return (
      <Shell>
        <div className="flex items-center justify-center py-20">
          <p className="text-muted-foreground">Run not found.</p>
        </div>
      </Shell>
    );
  }

  const isFailed = FAILED_STATUSES.has(run.status);
  const isActive = ACTIVE_STATUSES.has(run.status);

  return (
    <Shell>
      {/* Header */}
      <div className="flex items-start justify-between pt-4 pb-6">
        <div className="flex flex-col gap-2">
          <div className="flex items-center gap-3">
            <h1 className="text-balance font-mono font-normal text-xl tracking-tight">
              {run.id}
            </h1>
            <StatusBadge showDot status={run.status} />
          </div>
          <p className="text-pretty text-muted-foreground text-sm">
            Job:{" "}
            <span className="font-mono underline underline-offset-2">
              {run.job_id}
            </span>
          </p>
        </div>
        <div className="flex gap-2">
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

      {/* Error banner */}
      {isFailed && run.error && (
        <Alert className="mb-6" variant="destructive">
          <HugeiconsIcon icon={AlertCircleIcon} size={16} />
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{run.error}</AlertDescription>
        </Alert>
      )}

      {/* Execution Overview */}
      <Card className="mb-6">
        <CardHeader>
          <CardTitle>Execution Overview</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-x-6 gap-y-3">
            <DetailCell label="Status" value={run.status} />
            <DetailCell
              label="Duration"
              value={formatDuration(run.started_at, run.finished_at)}
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

      {/* Execution Trace */}
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

      {/* Tabs */}
      <Tabs className="w-full" onValueChange={setActiveTab} value={activeTab}>
        <TabsList>
          <TabsTrigger value="logs">Logs</TabsTrigger>
          <TabsTrigger value="payload">Payload</TabsTrigger>
          <TabsTrigger value="response">Response</TabsTrigger>
        </TabsList>

        <TabsContent className="mt-6" value="logs">
          <pre className="max-h-[500px] overflow-auto rounded-lg bg-muted p-4 font-mono text-xs leading-relaxed">
            {events && events.length > 0
              ? events
                  .map(
                    (evt) =>
                      `[${new Date(evt.created_at).toISOString()}] [${evt.level.toUpperCase()}] ${evt.message}`
                  )
                  .join("\n")
              : "No log events available for this run."}
          </pre>
        </TabsContent>

        <TabsContent className="mt-6" value="payload">
          <pre className="max-h-[500px] overflow-auto rounded-lg bg-muted p-4 font-mono text-xs leading-relaxed">
            {run.payload ? JSON.stringify(run.payload, null, 2) : "No payload"}
          </pre>
        </TabsContent>

        <TabsContent className="mt-6" value="response">
          <pre className="max-h-[500px] overflow-auto rounded-lg bg-muted p-4 font-mono text-xs leading-relaxed">
            {run.result
              ? JSON.stringify(run.result, null, 2)
              : "No response data"}
          </pre>
        </TabsContent>
      </Tabs>
    </Shell>
  );
}

function DetailCell({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-muted-foreground text-xs">{label}</span>
      <span className="font-mono text-sm">{value}</span>
    </div>
  );
}

const TRACE_SEGMENTS: {
  key: keyof ExecutionTrace;
  label: string;
  color: string;
}[] = [
  { key: "queue_wait_ms", label: "Queue Wait", color: "bg-blue-500" },
  { key: "dequeue_ms", label: "Dequeue", color: "bg-indigo-500" },
  { key: "dispatch_ms", label: "Dispatch", color: "bg-violet-500" },
  { key: "connect_ms", label: "Connect", color: "bg-amber-500" },
  { key: "ttfb_ms", label: "TTFB", color: "bg-emerald-500" },
  { key: "transfer_ms", label: "Transfer", color: "bg-cyan-500" },
];

function ExecutionTraceBar({ trace }: { trace: ExecutionTrace }) {
  const total = trace.total_ms || 1;

  return (
    <div className="space-y-3">
      {/* Bar visualization */}
      <div className="flex h-6 w-full overflow-hidden rounded-md">
        {TRACE_SEGMENTS.map((seg) => {
          const ms = trace[seg.key];
          const pct = (ms / total) * 100;
          if (pct < 0.5) {
            return null;
          }
          return (
            <div
              className={`${seg.color} opacity-80`}
              key={seg.key}
              style={{ width: `${pct}%` }}
              title={`${seg.label}: ${ms}ms`}
            />
          );
        })}
      </div>

      {/* Legend / key-value list */}
      <div className="grid grid-cols-2 gap-x-6 gap-y-2 sm:grid-cols-3">
        {TRACE_SEGMENTS.map((seg) => (
          <div className="flex items-center gap-2" key={seg.key}>
            <span
              className={`inline-block h-2.5 w-2.5 rounded-sm ${seg.color} opacity-80`}
            />
            <span className="text-muted-foreground text-xs">{seg.label}</span>
            <span className="font-mono text-xs">{trace[seg.key]}ms</span>
          </div>
        ))}
        <div className="flex items-center gap-2">
          <span className="inline-block h-2.5 w-2.5 rounded-sm bg-foreground/20" />
          <span className="text-muted-foreground text-xs">Total</span>
          <span className="font-mono text-xs">{trace.total_ms}ms</span>
        </div>
      </div>
    </div>
  );
}

function formatDuration(start: string | null, end: string | null): string {
  if (!start) {
    return "-";
  }
  const s = new Date(start).getTime();
  const e = end ? new Date(end).getTime() : Date.now();
  const ms = e - s;
  if (ms < 1000) {
    return `${ms}ms`;
  }
  if (ms < 60_000) {
    return `${(ms / 1000).toFixed(1)}s`;
  }
  const mins = Math.floor(ms / 60_000);
  const secs = Math.round((ms % 60_000) / 1000);
  return `${mins}m ${secs}s`;
}
