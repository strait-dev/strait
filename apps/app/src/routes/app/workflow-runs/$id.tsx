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
import { cn } from "@strait/ui/utils/index";
import { useQuery, useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { useState } from "react";
import DetailPageSkeleton from "@/components/common/detail-page-skeleton";
import EntityNotFound from "@/components/common/entity-not-found";
import ErrorComponent from "@/components/common/error-component";
import StatusBadge from "@/components/dashboard/status-badge";
import WorkflowRunActions from "@/components/dashboard/workflow-run-actions";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type {
  Workflow,
  WorkflowRun,
  WorkflowRunStatus,
  WorkflowStepRun,
} from "@/hooks/api/types";
import {
  workflowQueryOptions,
  workflowRunQueryOptions,
  workflowRunStepsQueryOptions,
} from "@/hooks/api/use-workflows";
import { formatDuration } from "@/lib/format";
import { ChevronLeftIcon, ChevronRightIcon, CopyIcon } from "@/lib/icons";
import { isPartOfChain } from "@/lib/workflow-continue";

export const Route = createFileRoute("/app/workflow-runs/$id")({
  head: () => ({ meta: [{ title: "Workflow run · Strait" }] }),
  loader: async ({ context, params }) => {
    await context.queryClient.ensureQueryData(
      workflowRunQueryOptions(params.id)
    );
    await context.queryClient
      .prefetchQuery(workflowRunStepsQueryOptions(params.id))
      .catch(() => undefined);
  },
  pendingComponent: DetailPageSkeleton,
  errorComponent: ErrorComponent,
  component: WorkflowRunDetailPage,
});

function WorkflowRunDetailPage() {
  const { id } = Route.useParams();
  usePageEvent("workflow_run_detail_viewed", { workflow_run_id: id });
  const { data: run } = useSuspenseQuery(workflowRunQueryOptions(id)) as {
    data: WorkflowRun | undefined;
  };
  const {
    data: steps,
    isError: stepsError,
    isLoading: stepsLoading,
  } = useQuery({
    ...workflowRunStepsQueryOptions(id),
    throwOnError: false,
  }) as {
    data: WorkflowStepRun[] | undefined;
    isError: boolean;
    isLoading: boolean;
  };
  const { data: workflow } = useQuery({
    ...workflowQueryOptions(run?.workflow_id ?? ""),
    enabled: Boolean(run?.workflow_id),
    throwOnError: false,
  }) as { data: Workflow | undefined };
  const [copied, setCopied] = useState(false);

  if (!run) {
    return (
      <Shell>
        <EntityNotFound backTo="/app/workflows" entity="Workflow run" />
      </Shell>
    );
  }

  const handleCopyId = async () => {
    try {
      await navigator.clipboard.writeText(run.id);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard API unavailable; silently no-op.
    }
  };

  return (
    <Shell>
      {/* Header */}
      <div className="flex flex-col gap-3 pt-4 pb-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex flex-col gap-2 overflow-hidden">
          <div className="flex items-center gap-3">
            <h1 className="truncate text-balance font-mono font-normal text-xl tracking-tight">
              {run.id}
            </h1>
            <StatusBadge showDot status={run.status as WorkflowRunStatus} />
            <Button
              aria-label="Copy run id"
              onClick={handleCopyId}
              size="icon"
              variant="ghost"
            >
              <HugeiconsIcon
                className="size-4"
                icon={copied ? ChevronRightIcon : CopyIcon}
              />
            </Button>
          </div>
          <p className="text-pretty text-muted-foreground text-sm">
            Workflow:{" "}
            <Link
              className="font-medium underline underline-offset-2"
              params={{ id: run.workflow_id }}
              to="/app/workflows/$id"
            >
              {workflow?.name ?? run.workflow_id}
            </Link>
            <span className="mx-2 text-muted-foreground/40">·</span>v
            {run.workflow_version}
            <span className="mx-2 text-muted-foreground/40">·</span>
            triggered by {run.triggered_by}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Link params={{ id: run.workflow_id }} to="/app/workflows/$id">
            <Button variant="outline">
              <HugeiconsIcon
                className="mr-1.5"
                icon={ChevronLeftIcon}
                size={14}
              />
              Workflow
            </Button>
          </Link>
          <WorkflowRunActions run={run} />
        </div>
      </div>

      {/* Lineage — only for runs in a continuation chain */}
      {isPartOfChain(run) && <LineageCard run={run} />}

      {/* What happened — timeline */}
      <Card className="mb-6">
        <CardHeader>
          <CardTitle>What happened</CardTitle>
        </CardHeader>
        <CardContent>
          <Timeline run={run} />
        </CardContent>
      </Card>

      {/* Step runs */}
      <Card className="mb-6">
        <CardHeader>
          <CardTitle>Step runs ({steps?.length ?? 0})</CardTitle>
        </CardHeader>
        <CardContent>
          <StepRunList
            isError={stepsError}
            isLoading={stepsLoading}
            steps={steps ?? []}
          />
        </CardContent>
      </Card>

      {/* Payload */}
      <Card>
        <CardHeader>
          <CardTitle>Payload</CardTitle>
        </CardHeader>
        <CardContent>
          <pre className="max-h-[400px] overflow-auto whitespace-pre-wrap break-all rounded-lg bg-muted p-3 font-mono text-xs leading-relaxed sm:p-4">
            {run.payload ? JSON.stringify(run.payload, null, 2) : "No payload"}
          </pre>
        </CardContent>
      </Card>
    </Shell>
  );
}

function LineageCard({ run }: { run: WorkflowRun }) {
  return (
    <Card className="mb-6">
      <CardHeader>
        <CardTitle>Continuation lineage</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap items-center gap-x-6 gap-y-3 text-sm">
          <span className="text-muted-foreground">
            Depth{" "}
            <span className="ml-1 font-mono text-foreground">
              #{run.lineage_depth}
            </span>
          </span>
          {run.continued_from_workflow_run_id ? (
            <span className="text-muted-foreground">
              Continued from{" "}
              <Link
                className="font-mono text-foreground underline underline-offset-2"
                params={{ id: run.continued_from_workflow_run_id }}
                to="/app/workflow-runs/$id"
              >
                {run.continued_from_workflow_run_id.slice(0, 8)}
              </Link>
            </span>
          ) : (
            <span className="text-muted-foreground">
              Continued from{" "}
              <span className="ml-1 font-mono text-foreground">
                — (chain root)
              </span>
            </span>
          )}
          {run.continued_to_workflow_run_id && (
            <span className="text-muted-foreground">
              Continued to{" "}
              <Link
                className="font-mono text-foreground underline underline-offset-2"
                params={{ id: run.continued_to_workflow_run_id }}
                to="/app/workflow-runs/$id"
              >
                {run.continued_to_workflow_run_id.slice(0, 8)}
              </Link>
            </span>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

type TimelineStep = {
  label: string;
  at: string | null | undefined;
};

function formatTimelineTimestamp(value: string) {
  return `${new Date(value).toISOString().replace("T", " ").slice(0, 19)} UTC`;
}

function Timeline({ run }: { run: WorkflowRun }) {
  const steps: TimelineStep[] = [
    { label: "Created", at: run.created_at },
    { label: "Started", at: run.started_at },
    { label: "Finished", at: run.finished_at },
  ];
  const duration = formatDuration(
    run.started_at ?? null,
    run.finished_at ?? null
  );

  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
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
          Version{" "}
          <span className="ml-1 font-mono text-foreground">
            v{run.workflow_version}
          </span>
        </span>
      </div>
    </div>
  );
}

function StepRunList({
  steps,
  isError,
  isLoading,
}: {
  steps: WorkflowStepRun[];
  isError: boolean;
  isLoading: boolean;
}) {
  if (isError) {
    return (
      <div
        className="rounded-lg bg-muted p-6 text-center text-muted-foreground text-sm"
        role="status"
      >
        Step runs are unavailable right now.
      </div>
    );
  }

  if (steps.length === 0) {
    return (
      <div className="rounded-lg bg-muted p-6 text-center text-muted-foreground text-sm">
        {isLoading ? "Loading step runs…" : "No step runs for this run."}
      </div>
    );
  }

  return (
    <ol className="flex flex-col gap-1.5">
      {steps.map((step) => (
        <li
          className="flex items-center gap-3 rounded-md border border-transparent px-3 py-2 hover:bg-accent"
          key={step.id}
        >
          <StatusBadge showDot size="xs" status={step.status} />
          <span className="font-medium text-sm">{step.step_ref}</span>
          <Badge size="xs" variant="outline">
            attempt {step.attempt}
          </Badge>
          {step.job_run_id && (
            <Link
              className="ml-auto font-mono text-muted-foreground text-xs underline underline-offset-2"
              params={{ id: step.job_run_id }}
              to="/app/runs/$id"
            >
              {step.job_run_id.slice(0, 8)}
            </Link>
          )}
        </li>
      ))}
    </ol>
  );
}
