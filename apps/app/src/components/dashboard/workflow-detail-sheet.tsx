import { HugeiconsIcon } from "@hugeicons/react";
import type { BadgeProps } from "@strait/ui/components/badge";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  DescriptionDetails,
  DescriptionList,
  DescriptionTerm,
} from "@strait/ui/components/description-list";
import { FeatureBadge } from "@strait/ui/components/feature-lock";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemGroup,
  ItemTitle,
} from "@strait/ui/components/item";
import { MetricCard } from "@strait/ui/components/metric-card";
import {
  Sheet,
  SheetContent,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@strait/ui/components/sheet";
import { StatusBadge } from "@strait/ui/components/status-badge";
import { Link } from "@tanstack/react-router";
import type { Workflow, WorkflowStepType } from "@/hooks/api/types";
import { useTriggerWorkflow } from "@/hooks/api/use-workflows";
import { useCurrentPlan } from "@/hooks/billing/use-current-plan";
import { ClockIcon, PlayActionIcon, TagIcon } from "@/lib/icons";
import {
  canUseFeature,
  getFeatureMinimumPlanLabel,
  type PlanFeature,
} from "@/lib/plan-tiers";

type WorkflowDetailSheetProps = {
  workflow: Workflow | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const STEP_TYPE_BADGE_VARIANTS = {
  job: "info-light",
  approval: "warning-light",
  sub_workflow: "secondary-light",
  wait_for_event: "primary-light",
  sleep: "secondary-light",
} satisfies Record<WorkflowStepType, BadgeProps["variant"]>;

function renderFeatureBadge(currentPlan: string, feature: PlanFeature) {
  if (canUseFeature(currentPlan, feature)) {
    return null;
  }

  return (
    <FeatureBadge
      className="ml-1.5"
      plan={getFeatureMinimumPlanLabel(feature)}
      size="xs"
    />
  );
}

const WorkflowDetailSheet = ({
  workflow,
  open,
  onOpenChange,
}: WorkflowDetailSheetProps) => {
  const triggerWorkflow = useTriggerWorkflow();
  const currentPlan = useCurrentPlan();

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
            <MetricCard size="sm" title="Success %" value="96.5%" />
            <MetricCard size="sm" title="Runs" value="584" />
            <MetricCard size="sm" title="Last run" value="8m ago" />
          </div>

          {/* Step List Preview */}
          <div>
            <h4 className="mb-3 font-medium text-muted-foreground text-xs uppercase">
              Steps
            </h4>
            <ItemGroup className="gap-2">
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
                <Item key={step.name} size="xs" variant="outline">
                  <ItemContent>
                    <ItemTitle>{step.name}</ItemTitle>
                  </ItemContent>
                  <ItemActions>
                    <Badge
                      dot
                      radius="md"
                      size="xs"
                      variant={STEP_TYPE_BADGE_VARIANTS[step.type]}
                    >
                      {step.type}
                    </Badge>
                    {step.type === "approval" &&
                      renderFeatureBadge(currentPlan, "approval_gates")}
                    {step.type === "sub_workflow" &&
                      renderFeatureBadge(currentPlan, "sub_workflows")}
                  </ItemActions>
                </Item>
              ))}
            </ItemGroup>
          </div>

          {/* Configuration */}
          <div>
            <h4 className="mb-3 font-medium text-muted-foreground text-xs uppercase">
              Configuration
            </h4>
            <DescriptionList orientation="horizontal" size="sm">
              <DescriptionTerm>
                <HugeiconsIcon className="size-3" icon={ClockIcon} />
                Timeout
              </DescriptionTerm>
              <DescriptionDetails className="font-mono">
                {workflow.timeout_secs}s
              </DescriptionDetails>
              <DescriptionTerm>Max concurrent</DescriptionTerm>
              <DescriptionDetails className="font-mono">
                {workflow.max_concurrent_runs}
              </DescriptionDetails>
              <DescriptionTerm>Max parallel steps</DescriptionTerm>
              <DescriptionDetails className="font-mono">
                {workflow.max_parallel_steps}
              </DescriptionDetails>
              <DescriptionTerm>Schedule</DescriptionTerm>
              <DescriptionDetails className="font-mono">
                {workflow.cron || "Manual"}
              </DescriptionDetails>
            </DescriptionList>
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
