import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@strait/ui/components/sheet";
import { cn } from "@strait/ui/utils/index";
import type { Workflow, WorkflowStepType } from "@/hooks/api/types";
import {
  ClockIcon,
  PauseActionIcon,
  PlayActionIcon,
  TagIcon,
} from "@/lib/icons";
import { StatusBadge } from "./status-badge";

type WorkflowDetailSheetProps = {
  workflow: Workflow | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const STEP_TYPE_COLORS: Record<WorkflowStepType, string> = {
  job: "bg-chart-2",
  approval: "bg-chart-3",
  sub_workflow: "bg-chart-5",
  wait_for_event: "bg-chart-1",
  sleep: "bg-muted-foreground",
};

function StatCell({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-md border p-3 text-center">
      <p className="font-normal text-lg">{value}</p>
      <p className="text-muted-foreground text-xs">{label}</p>
    </div>
  );
}

export function WorkflowDetailSheet({
  workflow,
  open,
  onOpenChange,
}: WorkflowDetailSheetProps) {
  if (!workflow) {
    return null;
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="overflow-y-auto">
        <SheetHeader>
          <SheetTitle>{workflow.name}</SheetTitle>
        </SheetHeader>

        <div className="mt-4 space-y-6">
          {/* Status */}
          <div className="flex items-center gap-2">
            <StatusBadge
              showDot
              size="sm"
              status={workflow.enabled ? "completed" : "paused"}
            />
            <span className="text-muted-foreground text-xs">
              v{workflow.version}
            </span>
          </div>

          {/* Quick Actions */}
          <div className="flex gap-2">
            <Button className="flex-1" size="sm">
              <HugeiconsIcon
                className="mr-1.5"
                icon={PlayActionIcon}
                size={14}
              />
              Trigger
            </Button>
            <Button className="flex-1" size="sm" variant="outline">
              <HugeiconsIcon
                className="mr-1.5"
                icon={PauseActionIcon}
                size={14}
              />
              Pause
            </Button>
          </div>

          {/* Stats Grid */}
          <div className="grid grid-cols-3 gap-2">
            <StatCell label="Success %" value="96.5%" />
            <StatCell label="Runs" value="584" />
            <StatCell label="Last Run" value="8m ago" />
          </div>

          {/* Step List Preview */}
          <div>
            <h4 className="mb-3 font-medium text-muted-foreground text-xs uppercase">
              Steps
            </h4>
            <div className="space-y-2">
              {(
                [
                  { name: "validate-input", type: "job" as const },
                  { name: "process-payment", type: "job" as const },
                  { name: "mgr-approval", type: "approval" as const },
                  { name: "send-notification", type: "job" as const },
                  { name: "cool-down", type: "sleep" as const },
                ] as const
              ).map((step) => (
                <div
                  className="flex items-center gap-2 rounded-md border px-3 py-2"
                  key={step.name}
                >
                  <span
                    className={cn(
                      "h-2 w-2 shrink-0 rounded-full",
                      STEP_TYPE_COLORS[step.type]
                    )}
                  />
                  <span className="text-sm">{step.name}</span>
                  <span className="ml-auto text-muted-foreground text-xs">
                    {step.type}
                  </span>
                </div>
              ))}
            </div>
          </div>

          {/* Configuration */}
          <div>
            <h4 className="mb-3 font-medium text-muted-foreground text-xs uppercase">
              Configuration
            </h4>
            <div className="space-y-2.5">
              <div className="flex items-center justify-between text-sm">
                <span className="flex items-center gap-1.5 text-muted-foreground">
                  <HugeiconsIcon icon={ClockIcon} size={12} />
                  Timeout
                </span>
                <span className="font-mono text-xs">
                  {workflow.timeout_secs}s
                </span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">Max Concurrent</span>
                <span className="font-mono text-xs">
                  {workflow.max_concurrent_runs}
                </span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">
                  Max Parallel Steps
                </span>
                <span className="font-mono text-xs">
                  {workflow.max_parallel_steps}
                </span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">Schedule</span>
                <span className="font-mono text-xs">
                  {workflow.cron || "Manual"}
                </span>
              </div>
            </div>
          </div>

          {/* Tags */}
          {workflow.tags && Object.keys(workflow.tags).length > 0 && (
            <div>
              <h4 className="mb-2 flex items-center gap-1.5 font-medium text-muted-foreground text-xs uppercase">
                <HugeiconsIcon icon={TagIcon} size={12} />
                Tags
              </h4>
              <div className="flex flex-wrap gap-1.5">
                {Object.entries(workflow.tags).map(([key, val]) => (
                  <Badge className="text-xs" key={key} variant="secondary">
                    {key}: {val}
                  </Badge>
                ))}
              </div>
            </div>
          )}

          {/* Recent Runs */}
          <div>
            <h4 className="mb-2 font-medium text-muted-foreground text-xs uppercase">
              Recent Runs
            </h4>
            <div className="space-y-1.5">
              {[
                { id: "wfr_1", status: "completed" as const, time: "8m ago" },
                { id: "wfr_2", status: "running" as const, time: "45m ago" },
                { id: "wfr_3", status: "failed" as const, time: "2h ago" },
              ].map((run) => (
                <div
                  className="flex items-center justify-between rounded-md border px-3 py-2"
                  key={run.id}
                >
                  <span className="font-mono text-xs">{run.id}</span>
                  <div className="flex items-center gap-2">
                    <StatusBadge size="xs" status={run.status} />
                    <span className="text-muted-foreground text-xs">
                      {run.time}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
}
