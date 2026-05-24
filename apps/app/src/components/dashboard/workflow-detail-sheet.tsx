import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
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
import FeatureBadge from "@/components/billing/feature-badge";
import type { Workflow, WorkflowStepType } from "@/hooks/api/types";
import { useTriggerWorkflow } from "@/hooks/api/use-workflows";
import { ClockIcon, PlayActionIcon, TagIcon } from "@/lib/icons";
import StatusBadge from "./status-badge";

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

const StatCell = ({
  label,
  value,
}: {
  label: string;
  value: string | number;
}) => (
  <div className="rounded-md border p-3 text-center">
    <p className="font-normal text-lg">{value}</p>
    <p className="text-muted-foreground text-xs">{label}</p>
  </div>
);

const WorkflowDetailSheet = ({
  workflow,
  open,
  onOpenChange,
}: WorkflowDetailSheetProps) => {
  const triggerWorkflow = useTriggerWorkflow();

  if (!workflow) {
    return null;
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="flex flex-col overflow-y-auto">
        <SheetHeader>
          <SheetTitle>{workflow.name}</SheetTitle>
        </SheetHeader>

        <div className="mt-4 flex-1 space-y-6 overflow-y-auto px-6">
          {/* Status */}
          <div className="flex items-center gap-2">
            <StatusBadge
              showDot
              status={workflow.enabled ? "completed" : "paused"}
            />
            <span className="text-muted-foreground text-sm">
              v{workflow.version}
            </span>
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
                  { name: "notify-sub", type: "sub_workflow" as const },
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
                      "size-2 shrink-0 rounded-full",
                      STEP_TYPE_COLORS[step.type]
                    )}
                  />
                  <span className="text-sm">{step.name}</span>
                  <span className="ml-auto flex items-center text-muted-foreground text-sm">
                    {step.type}
                    {step.type === "approval" && (
                      <FeatureBadge feature="approval_gates" />
                    )}
                    {step.type === "sub_workflow" && (
                      <FeatureBadge feature="sub_workflows" />
                    )}
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
                  <HugeiconsIcon className="size-3" icon={ClockIcon} />
                  Timeout
                </span>
                <span className="font-mono text-sm">
                  {workflow.timeout_secs}s
                </span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">Max Concurrent</span>
                <span className="font-mono text-sm">
                  {workflow.max_concurrent_runs}
                </span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">
                  Max Parallel Steps
                </span>
                <span className="font-mono text-sm">
                  {workflow.max_parallel_steps}
                </span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">Schedule</span>
                <span className="font-mono text-sm">
                  {workflow.cron || "Manual"}
                </span>
              </div>
            </div>
          </div>

          {/* Tags */}
          {workflow.tags && Object.keys(workflow.tags).length > 0 && (
            <div>
              <h4 className="mb-2 flex items-center gap-1.5 font-medium text-muted-foreground text-xs uppercase">
                <HugeiconsIcon className="size-3" icon={TagIcon} />
                Tags
              </h4>
              <div className="flex flex-wrap gap-1.5">
                {Object.entries(workflow.tags).map(([key, val]) => (
                  <Badge key={key} variant="secondary">
                    {key}: {val}
                  </Badge>
                ))}
              </div>
            </div>
          )}
        </div>

        <SheetFooter>
          <Button
            className="w-full"
            render={
              <Link params={{ id: workflow.id }} to="/app/workflows/$id" />
            }
            variant="outline"
          >
            View details
          </Button>
          <Button
            className="w-full"
            disabled={triggerWorkflow.isPending}
            onClick={() => triggerWorkflow.mutate({ workflowId: workflow.id })}
          >
            <HugeiconsIcon className="mr-1.5 size-3.5" icon={PlayActionIcon} />
            Trigger
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
};

export default WorkflowDetailSheet;
