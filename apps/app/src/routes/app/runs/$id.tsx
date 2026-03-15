import { HugeiconsIcon } from "@hugeicons/react";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@strait/ui/components/breadcrumb";
import { Button } from "@strait/ui/components/button";
import { Shell } from "@strait/ui/components/shell";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs";
import { useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { useState } from "react";
import { StatusBadge } from "@/components/dashboard/status-badge";
import type { JobRun, RunEvent, RunStatus } from "@/hooks/api/types";
import { runEventsQueryOptions, runQueryOptions } from "@/hooks/api/use-runs";
import { AlertIcon, RefreshIcon, XCircleIcon } from "@/lib/icons";

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
      {/* Breadcrumb */}
      <Breadcrumb>
        <BreadcrumbList>
          <BreadcrumbItem>
            <BreadcrumbLink>
              <Link to="/app/runs">Runs</Link>
            </BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbPage className="font-mono text-xs">
              {run.id}
            </BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>

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
            Job: <span className="font-mono">{run.job_id}</span>
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
        <div className="mb-6 flex gap-2 rounded-md border border-chart-4/30 bg-chart-4/5 p-4">
          <HugeiconsIcon
            className="mt-0.5 shrink-0 text-chart-4"
            icon={AlertIcon}
            size={16}
          />
          <div>
            <p className="font-medium text-chart-4 text-sm">Error</p>
            <p className="mt-0.5 text-muted-foreground text-xs">{run.error}</p>
          </div>
        </div>
      )}

      {/* Execution details */}
      <div className="mb-6 grid grid-cols-5 gap-4 rounded-md border p-4">
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
            run.finished_at ? new Date(run.finished_at).toLocaleString() : "-"
          }
        />
        <DetailCell label="Triggered By" value={run.triggered_by} />
        <DetailCell label="Attempt" value={String(run.attempt)} />
      </div>

      {/* Tabs */}
      <Tabs className="w-full" onValueChange={setActiveTab} value={activeTab}>
        <TabsList>
          <TabsTrigger value="logs">Logs</TabsTrigger>
          <TabsTrigger value="payload">Payload</TabsTrigger>
          <TabsTrigger value="response">Response</TabsTrigger>
        </TabsList>

        <TabsContent className="mt-6" value="logs">
          <div className="rounded-md border bg-muted/30 p-4">
            <pre className="max-h-[500px] overflow-auto font-mono text-muted-foreground text-xs leading-relaxed">
              {events && events.length > 0
                ? events
                    .map(
                      (evt) =>
                        `[${new Date(evt.created_at).toISOString()}] [${evt.level.toUpperCase()}] ${evt.message}`
                    )
                    .join("\n")
                : "No log events available for this run."}
            </pre>
          </div>
        </TabsContent>

        <TabsContent className="mt-6" value="payload">
          <div className="rounded-md border bg-muted/30 p-4">
            <pre className="max-h-[500px] overflow-auto font-mono text-muted-foreground text-xs leading-relaxed">
              {run.payload
                ? JSON.stringify(run.payload, null, 2)
                : "No payload"}
            </pre>
          </div>
        </TabsContent>

        <TabsContent className="mt-6" value="response">
          <div className="rounded-md border bg-muted/30 p-4">
            <pre className="max-h-[500px] overflow-auto font-mono text-muted-foreground text-xs leading-relaxed">
              {run.result
                ? JSON.stringify(run.result, null, 2)
                : "No response data"}
            </pre>
          </div>
        </TabsContent>
      </Tabs>
    </Shell>
  );
}

function DetailCell({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-1">
      <span className="font-medium text-[11px] text-muted-foreground uppercase">
        {label}
      </span>
      <span className="font-mono text-xs capitalize">{value}</span>
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
