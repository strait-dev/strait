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
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import StatusBadge from "@/components/dashboard/status-badge";
import DetailCell from "@/components/runs/detail-cell";
import ExecutionTraceBar from "@/components/runs/execution-trace-bar";
import type {
  JobRun,
  PaginatedResponse,
  RunEvent,
  RunStatus,
} from "@/hooks/api/types";
import { runEventsQueryOptions, runQueryOptions } from "@/hooks/api/use-runs";
import { formatDuration } from "@/lib/format";
import { AlertCircleIcon, RefreshIcon, XCircleIcon } from "@/lib/icons";

export const Route = createFileRoute("/app/runs/$id")({
  loader: async ({ context, params }) => {
    await Promise.all([
      context.queryClient.ensureQueryData(runQueryOptions(params.id)),
      context.queryClient.ensureQueryData(runEventsQueryOptions(params.id)),
    ]);
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

function RunDetailPage() {
  const { id } = Route.useParams();
  const { data: run } = useSuspenseQuery(runQueryOptions(id)) as {
    data: JobRun | undefined;
  };
  const { data: eventsData } = useSuspenseQuery(runEventsQueryOptions(id)) as {
    data: PaginatedResponse<RunEvent> | undefined;
  };
  const events = eventsData?.data ?? [];
  const [activeTab, setActiveTab] = useState("logs");

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
      <div className="flex flex-col gap-3 pt-4 pb-6 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex flex-col gap-2 overflow-hidden">
          <div className="flex items-center gap-3">
            <h1 className="truncate font-mono font-normal text-lg tracking-tight sm:text-xl">
              {run.id}
            </h1>
            <StatusBadge showDot status={run.status as RunStatus} />
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
          <pre className="max-h-[500px] overflow-auto whitespace-pre-wrap break-all rounded-lg bg-muted p-3 font-mono text-xs leading-relaxed sm:p-4">
            {events && events.length > 0
              ? events
                  .map(
                    (evt) =>
                      `[${new Date(evt.created_at).toISOString()}] [${(evt.level ?? "info").toUpperCase()}] ${evt.message}`
                  )
                  .join("\n")
              : "No log events available for this run."}
          </pre>
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
