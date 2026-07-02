import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  DescriptionDetails,
  DescriptionList,
  DescriptionTerm,
} from "@strait/ui/components/description-list";
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
import type { Job } from "@/hooks/api/types";
import { useTriggerJob } from "@/hooks/api/use-jobs";
import {
  ClockIcon,
  GlobeIcon,
  PlayActionIcon,
  RefreshIcon,
  TagIcon,
} from "@/lib/icons";

type JobDetailSheetProps = {
  job: Job | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const JobDetailSheet = ({ job, open, onOpenChange }: JobDetailSheetProps) => {
  const triggerJob = useTriggerJob();

  if (!job) {
    return null;
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="flex flex-col overflow-y-auto">
        <SheetHeader>
          <SheetTitle>{job.name}</SheetTitle>
        </SheetHeader>

        <div className="mt-4 flex-1 space-y-6 overflow-y-auto px-6">
          {/* Status */}
          <div className="flex items-center gap-2">
            <StatusBadge
              showDot
              status={job.enabled ? "completed" : "paused"}
            />
            <span className="text-muted-foreground text-sm">
              {job.enabled ? "Enabled" : "Disabled"}
            </span>
          </div>

          {/* Stats Grid */}
          <div className="grid grid-cols-3 gap-2">
            <MetricCard size="sm" title="Success %" value="98.2%" />
            <MetricCard size="sm" title="Runs" value="1,247" />
            <MetricCard size="sm" title="Last run" value="2m ago" />
          </div>

          {/* Configuration */}
          <div>
            <h4 className="mb-3 font-medium text-muted-foreground text-xs uppercase">
              Configuration
            </h4>
            <DescriptionList orientation="horizontal" size="sm">
              <DescriptionTerm>
                <HugeiconsIcon className="size-3.5 shrink-0" icon={GlobeIcon} />
                Endpoint
              </DescriptionTerm>
              <DescriptionDetails className="truncate font-mono">
                {job.endpoint_url || "-"}
              </DescriptionDetails>
              <DescriptionTerm>
                <HugeiconsIcon className="size-3.5 shrink-0" icon={ClockIcon} />
                Schedule
              </DescriptionTerm>
              <DescriptionDetails className="font-mono">
                {job.cron || "Manual"}
              </DescriptionDetails>
              <DescriptionTerm>
                <HugeiconsIcon
                  className="size-3.5 shrink-0"
                  icon={RefreshIcon}
                />
                Retry
              </DescriptionTerm>
              <DescriptionDetails className="font-mono">
                {job.max_attempts} attempts
              </DescriptionDetails>
              <DescriptionTerm>
                <HugeiconsIcon className="size-3.5 shrink-0" icon={ClockIcon} />
                Timeout
              </DescriptionTerm>
              <DescriptionDetails className="font-mono">
                {job.timeout_secs}s
              </DescriptionDetails>
            </DescriptionList>
          </div>

          {/* Tags */}
          {job.tags && Object.keys(job.tags).length > 0 && (
            <div>
              <h4 className="mb-2 flex items-center gap-1.5 font-medium text-muted-foreground text-xs uppercase">
                <HugeiconsIcon className="size-3" icon={TagIcon} />
                Tags
              </h4>
              <div className="flex flex-wrap gap-1.5">
                {Object.entries(job.tags).map(([key, val]) => (
                  <Badge key={key} variant="secondary">
                    {key}: {val}
                  </Badge>
                ))}
              </div>
            </div>
          )}

          {/* Recent runs Preview */}
          <div>
            <h4 className="mb-2 font-medium text-muted-foreground text-xs uppercase">
              Recent runs
            </h4>
            <ItemGroup className="gap-2">
              {[
                { id: "run_1", status: "completed" as const, time: "2m ago" },
                { id: "run_2", status: "completed" as const, time: "1h ago" },
                { id: "run_3", status: "failed" as const, time: "3h ago" },
              ].map((run) => (
                <Item key={run.id} size="xs" variant="outline">
                  <ItemContent>
                    <ItemTitle className="font-mono">{run.id}</ItemTitle>
                  </ItemContent>
                  <ItemActions>
                    <StatusBadge status={run.status} />
                    <span className="text-muted-foreground text-sm">
                      {run.time}
                    </span>
                  </ItemActions>
                </Item>
              ))}
            </ItemGroup>
          </div>
        </div>

        <SheetFooter>
          <Button
            className="w-full"
            render={<Link params={{ id: job.id }} to="/app/jobs/$id" />}
            variant="outline"
          >
            View details
          </Button>
          <Button
            className="w-full"
            disabled={triggerJob.isPending}
            onClick={() => triggerJob.mutate({ id: job.id })}
          >
            <HugeiconsIcon className="mr-1.5 size-3.5" icon={PlayActionIcon} />
            Trigger
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
};

export default JobDetailSheet;
