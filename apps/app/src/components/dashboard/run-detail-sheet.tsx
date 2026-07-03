import { HugeiconsIcon } from "@hugeicons/react";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@strait/ui/components/accordion";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@strait/ui/components/alert";
import { Button } from "@strait/ui/components/button";
import { CodeBlock } from "@strait/ui/components/code-block";
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
import {
  Sheet,
  SheetContent,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@strait/ui/components/sheet";
import { StatusBadge } from "@strait/ui/components/status-badge";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Fragment } from "react";
import type {
  DisplayStatus,
  JobRun,
  PaginatedResponse,
  RunEvent,
} from "@/hooks/api/types";
import {
  runEventsQueryOptions,
  useCancelRun,
  useRetryRun,
} from "@/hooks/api/use-runs";
import {
  AlertIcon,
  BriefcaseIcon,
  ClockIcon,
  RefreshIcon,
  XCircleIcon,
} from "@/lib/icons";

type RunDetailSheetProps = {
  run: JobRun | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

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
  return `${(ms / 60_000).toFixed(1)}m`;
}

const RunDetailSheet = ({ run, open, onOpenChange }: RunDetailSheetProps) => {
  const retryRun = useRetryRun();
  const cancelRun = useCancelRun();
  const { data: eventsData } = useQuery({
    ...runEventsQueryOptions(run?.id ?? ""),
    enabled: Boolean(run?.id) && open,
  }) as { data: PaginatedResponse<RunEvent> | undefined };
  const events = eventsData?.data ?? [];

  if (!run) {
    return null;
  }

  const isFailed =
    run.status === "failed" ||
    run.status === "dead_letter" ||
    run.status === "crashed" ||
    run.status === "system_failed";

  const isActive =
    run.status === "executing" ||
    run.status === "queued" ||
    run.status === "waiting";

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent
        className="flex flex-col overflow-y-auto"
        data-testid="run-detail-sheet"
      >
        <SheetHeader>
          <SheetTitle className="font-mono text-sm">{run.id}</SheetTitle>
        </SheetHeader>

        <div className="mt-4 flex-1 space-y-6 overflow-y-auto px-6">
          {/* Status */}
          <div className="flex items-center gap-2">
            <StatusBadge status={run.status as DisplayStatus} />
          </div>

          {/* Job Link */}
          <DescriptionList orientation="horizontal" size="sm">
            <DescriptionTerm>
              <HugeiconsIcon className="size-3.5" icon={BriefcaseIcon} />
              Job
            </DescriptionTerm>
            <DescriptionDetails className="font-mono">
              {run.job_id}
            </DescriptionDetails>
          </DescriptionList>

          {/* Error Alert */}
          {isFailed && run.error && (
            <Alert variant="destructive">
              <HugeiconsIcon className="size-4" icon={AlertIcon} />
              <AlertTitle>Error</AlertTitle>
              <AlertDescription>{String(run.error)}</AlertDescription>
            </Alert>
          )}

          {/* Execution details */}
          <div>
            <h4 className="mb-3 font-medium text-muted-foreground text-xs uppercase">
              Execution details
            </h4>
            <DescriptionList orientation="horizontal" size="sm">
              <DescriptionTerm>Attempt</DescriptionTerm>
              <DescriptionDetails className="font-mono">
                {run.attempt}
              </DescriptionDetails>
              <DescriptionTerm>Triggered by</DescriptionTerm>
              <DescriptionDetails className="font-mono">
                {run.triggered_by}
              </DescriptionDetails>
              <DescriptionTerm>
                <HugeiconsIcon className="size-3" icon={ClockIcon} />
                Duration
              </DescriptionTerm>
              <DescriptionDetails className="font-mono">
                {formatDuration(
                  run.started_at ?? null,
                  run.finished_at ?? null
                )}
              </DescriptionDetails>
              <DescriptionTerm>Started</DescriptionTerm>
              <DescriptionDetails className="font-mono">
                {run.started_at ?? "-"}
              </DescriptionDetails>
              <DescriptionTerm>Finished</DescriptionTerm>
              <DescriptionDetails className="font-mono">
                {run.finished_at ?? "-"}
              </DescriptionDetails>
            </DescriptionList>
          </div>

          {/* Execution trace */}
          {run.execution_trace && (
            <div>
              <h4 className="mb-3 font-medium text-muted-foreground text-xs uppercase">
                Execution trace
              </h4>
              <DescriptionList orientation="horizontal" size="sm">
                {(
                  [
                    ["Queue wait", run.execution_trace.queue_wait_ms],
                    ["Dequeue", run.execution_trace.dequeue_ms],
                    ["Connect", run.execution_trace.connect_ms],
                    ["TTFB", run.execution_trace.ttfb_ms],
                    ["Transfer", run.execution_trace.transfer_ms],
                    ["Total", run.execution_trace.total_ms],
                  ] as const
                ).map(([label, ms]) => (
                  <Fragment key={label}>
                    <DescriptionTerm>{label}</DescriptionTerm>
                    <DescriptionDetails className="font-mono">
                      {ms}ms
                    </DescriptionDetails>
                  </Fragment>
                ))}
              </DescriptionList>
            </div>
          )}

          <Accordion
            defaultValue={["logs", "payload"]}
            multiple
            variant="outline"
          >
            <AccordionItem value="logs">
              <AccordionTrigger>Logs ({events.length})</AccordionTrigger>
              <AccordionContent>
                {events.length === 0 ? (
                  <Empty border={false} className="py-2">
                    <EmptyHeader>
                      <EmptyTitle>No log events</EmptyTitle>
                      <EmptyDescription>
                        Log events for this run will appear here.
                      </EmptyDescription>
                    </EmptyHeader>
                  </Empty>
                ) : (
                  <CodeBlock
                    code={events
                      .map(
                        (e) =>
                          `[${new Date(e.created_at).toISOString()}] [${e.level?.toUpperCase() ?? "INFO"}] ${e.message}`
                      )
                      .join("\n")}
                    maxHeight={200}
                    wrap
                  />
                )}
              </AccordionContent>
            </AccordionItem>
            <AccordionItem value="payload">
              <AccordionTrigger>Payload</AccordionTrigger>
              <AccordionContent>
                <CodeBlock
                  code={
                    run.payload
                      ? JSON.stringify(run.payload, null, 2)
                      : "No payload"
                  }
                  language="json"
                  maxHeight={200}
                  wrap
                />
              </AccordionContent>
            </AccordionItem>
            {run.result != null && (
              <AccordionItem value="result">
                <AccordionTrigger>Result</AccordionTrigger>
                <AccordionContent>
                  <CodeBlock
                    code={JSON.stringify(run.result, null, 2)}
                    language="json"
                    maxHeight={200}
                    wrap
                  />
                </AccordionContent>
              </AccordionItem>
            )}
          </Accordion>
        </div>
        <SheetFooter>
          <Button
            className="w-full"
            render={<Link params={{ id: run.id }} to="/app/runs/$id" />}
            variant="outline"
          >
            View details
          </Button>
          {isFailed && (
            <Button
              className="w-full"
              disabled={retryRun.isPending}
              onClick={() => retryRun.mutate({ run_id: run.id })}
              variant="outline"
            >
              <HugeiconsIcon className="mr-1.5 size-3.5" icon={RefreshIcon} />
              Retry
            </Button>
          )}
          {isActive && (
            <Button
              className="w-full"
              disabled={cancelRun.isPending}
              onClick={() => cancelRun.mutate({ run_id: run.id })}
              variant="outline"
            >
              <HugeiconsIcon className="mr-1.5 size-3.5" icon={XCircleIcon} />
              Cancel
            </Button>
          )}
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
};

export default RunDetailSheet;
