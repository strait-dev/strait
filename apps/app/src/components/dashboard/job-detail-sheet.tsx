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
import { Link } from "@tanstack/react-router";
import type { Job } from "@/hooks/api/types";
import {
  ClockIcon,
  GlobeIcon,
  PlayActionIcon,
  RefreshIcon,
  TagIcon,
} from "@/lib/icons";
import StatusBadge from "./status-badge";

type JobDetailSheetProps = {
  job: Job | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const StatCell = ({
  label,
  value,
}: {
  label: string;
  value: string | number;
}) => {
  return (
    <div className="rounded-md border p-3 text-center">
      <p className="font-normal text-lg">{value}</p>
      <p className="text-muted-foreground text-xs">{label}</p>
    </div>
  );
};

const DetailRow = ({
  icon,
  label,
  value,
}: {
  icon: any;
  label: string;
  value: string;
}) => {
  return (
    <div className="flex items-start justify-between gap-2 text-sm">
      <span className="flex shrink-0 items-center gap-2 text-muted-foreground">
        <HugeiconsIcon className="shrink-0" icon={icon} size={14} />
        {label}
      </span>
      <span className="truncate text-right font-mono text-sm">{value}</span>
    </div>
  );
};

const JobDetailSheet = ({ job, open, onOpenChange }: JobDetailSheetProps) => {
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
            <StatCell label="Success %" value="98.2%" />
            <StatCell label="Runs" value="1,247" />
            <StatCell label="Last Run" value="2m ago" />
          </div>

          {/* Configuration */}
          <div>
            <h4 className="mb-3 font-medium text-muted-foreground text-xs uppercase">
              Configuration
            </h4>
            <div className="space-y-2.5">
              <DetailRow
                icon={GlobeIcon}
                label="Endpoint"
                value={job.endpoint_url || "-"}
              />
              <DetailRow
                icon={ClockIcon}
                label="Schedule"
                value={job.cron || "Manual"}
              />
              <DetailRow
                icon={RefreshIcon}
                label="Retry"
                value={`${job.max_attempts} attempts`}
              />
              <DetailRow
                icon={ClockIcon}
                label="Timeout"
                value={`${job.timeout_secs}s`}
              />
            </div>
          </div>

          {/* Tags */}
          {job.tags && Object.keys(job.tags).length > 0 && (
            <div>
              <h4 className="mb-2 flex items-center gap-1.5 font-medium text-muted-foreground text-xs uppercase">
                <HugeiconsIcon icon={TagIcon} size={12} />
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

          {/* Recent Runs Preview */}
          <div>
            <h4 className="mb-2 font-medium text-muted-foreground text-xs uppercase">
              Recent Runs
            </h4>
            <div className="space-y-1.5">
              {[
                { id: "run_1", status: "completed" as const, time: "2m ago" },
                { id: "run_2", status: "completed" as const, time: "1h ago" },
                { id: "run_3", status: "failed" as const, time: "3h ago" },
              ].map((run) => (
                <div
                  className="flex items-center justify-between rounded-md border px-3 py-2"
                  key={run.id}
                >
                  <span className="font-mono text-sm">{run.id}</span>
                  <div className="flex items-center gap-2">
                    <StatusBadge status={run.status} />
                    <span className="text-muted-foreground text-sm">
                      {run.time}
                    </span>
                  </div>
                </div>
              ))}
            </div>
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
          <Button className="w-full">
            <HugeiconsIcon className="mr-1.5" icon={PlayActionIcon} size={14} />
            Trigger
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
};

export default JobDetailSheet;
