import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Sheet,
  SheetContent,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@strait/ui/components/sheet";
import { cn } from "@strait/ui/utils/index";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import type { DisplayStatus, JobRun } from "@/hooks/api/types";
import {
  AlertIcon,
  BriefcaseIcon,
  ChevronDownIcon,
  ClockIcon,
  RefreshIcon,
  XCircleIcon,
} from "@/lib/icons";
import StatusBadge from "./status-badge";

type RunDetailSheetProps = {
  run: JobRun | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const CollapsibleSection = ({
  title,
  children,
  defaultOpen = false,
}: {
  title: string;
  children: React.ReactNode;
  defaultOpen?: boolean;
}) => {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <div className="rounded-md border">
      <button
        className="flex w-full items-center justify-between px-3 py-2 font-medium text-sm hover:bg-muted/50"
        onClick={() => setOpen((o) => !o)}
        type="button"
      >
        {title}
        <HugeiconsIcon
          className={cn("transition-transform", open && "rotate-180")}
          icon={ChevronDownIcon}
          size={14}
        />
      </button>
      {open && <div className="border-t px-3 py-2">{children}</div>}
    </div>
  );
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
  if (!run) {
    return null;
  }

  const isFailed =
    run.status === "failed" ||
    run.status === "crashed" ||
    run.status === "system_failed";

  const isActive =
    run.status === "executing" ||
    run.status === "queued" ||
    run.status === "waiting";

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="flex flex-col overflow-y-auto">
        <SheetHeader>
          <SheetTitle className="font-mono text-sm">{run.id}</SheetTitle>
        </SheetHeader>

        <div className="mt-4 flex-1 space-y-6 overflow-y-auto px-6">
          {/* Status */}
          <div className="flex items-center gap-2">
            <StatusBadge status={run.status as DisplayStatus} />
          </div>

          {/* Job Link */}
          <div className="flex items-center gap-2 text-sm">
            <HugeiconsIcon
              className="text-muted-foreground"
              icon={BriefcaseIcon}
              size={14}
            />
            <span className="text-muted-foreground">Job</span>
            <span className="ml-auto font-mono text-sm">{run.job_id}</span>
          </div>

          {/* Error Alert */}
          {isFailed && run.error && (
            <div className="flex gap-2 rounded-md border border-chart-4/30 bg-chart-4/5 p-3">
              <HugeiconsIcon
                className="mt-0.5 shrink-0 text-chart-4"
                icon={AlertIcon}
                size={16}
              />
              <div>
                <p className="font-medium text-chart-4 text-sm">Error</p>
                <p className="mt-0.5 text-muted-foreground text-sm">
                  {String(run.error)}
                </p>
              </div>
            </div>
          )}

          {/* Execution Details */}
          <div>
            <h4 className="mb-3 font-medium text-muted-foreground text-xs uppercase">
              Execution Details
            </h4>
            <div className="space-y-2.5">
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">Attempt</span>
                <span className="font-mono text-sm">{run.attempt}</span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">Triggered by</span>
                <span className="font-mono text-sm">{run.triggered_by}</span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="flex items-center gap-1.5 text-muted-foreground">
                  <HugeiconsIcon icon={ClockIcon} size={12} />
                  Duration
                </span>
                <span className="font-mono text-sm">
                  {formatDuration(
                    run.started_at ?? null,
                    run.finished_at ?? null
                  )}
                </span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">Started</span>
                <span className="font-mono text-sm">
                  {run.started_at ?? "-"}
                </span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">Finished</span>
                <span className="font-mono text-sm">
                  {run.finished_at ?? "-"}
                </span>
              </div>
            </div>
          </div>

          {/* Execution Trace */}
          {run.execution_trace && (
            <div>
              <h4 className="mb-3 font-medium text-muted-foreground text-xs uppercase">
                Execution Trace
              </h4>
              <div className="space-y-1.5">
                {(
                  [
                    ["Queue Wait", run.execution_trace.queue_wait_ms],
                    ["Dequeue", run.execution_trace.dequeue_ms],
                    ["Connect", run.execution_trace.connect_ms],
                    ["TTFB", run.execution_trace.ttfb_ms],
                    ["Transfer", run.execution_trace.transfer_ms],
                    ["Total", run.execution_trace.total_ms],
                  ] as const
                ).map(([label, ms]) => (
                  <div
                    className="flex items-center justify-between text-xs"
                    key={label}
                  >
                    <span className="text-muted-foreground">{label}</span>
                    <span className="font-mono">{ms}ms</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Logs */}
          <CollapsibleSection defaultOpen title="Logs">
            <pre className="max-h-[200px] overflow-auto whitespace-pre-wrap text-muted-foreground text-xs">
              No logs available for this run.
            </pre>
          </CollapsibleSection>

          {/* Payload */}
          <CollapsibleSection defaultOpen title="Payload">
            <pre className="max-h-[200px] overflow-auto whitespace-pre-wrap text-muted-foreground text-xs">
              {run.payload
                ? JSON.stringify(run.payload, null, 2)
                : "No payload"}
            </pre>
          </CollapsibleSection>

          {/* Result */}
          {run.result != null && (
            <CollapsibleSection title="Result">
              <pre className="max-h-[200px] overflow-auto whitespace-pre-wrap text-muted-foreground text-xs">
                {JSON.stringify(run.result, null, 2)}
              </pre>
            </CollapsibleSection>
          )}
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
            <Button className="w-full" variant="outline">
              <HugeiconsIcon className="mr-1.5" icon={RefreshIcon} size={14} />
              Retry
            </Button>
          )}
          {isActive && (
            <Button className="w-full" variant="outline">
              <HugeiconsIcon className="mr-1.5" icon={XCircleIcon} size={14} />
              Cancel
            </Button>
          )}
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
};

export default RunDetailSheet;
