import { HugeiconsIcon } from "@hugeicons/react";
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
  BriefcaseIcon,
  CalendarIcon,
  ClockIcon,
  RefreshIcon,
} from "@/lib/icons";
import StatusBadge from "./status-badge";

type ScheduleDetailSheetProps = {
  schedule: Job | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
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
    <div className="flex items-center gap-2 text-sm">
      <HugeiconsIcon className="text-muted-foreground" icon={icon} size={14} />
      <span className="text-muted-foreground">{label}</span>
      <span className="ml-auto font-mono text-sm">{value}</span>
    </div>
  );
};

const ScheduleDetailSheet = ({
  schedule,
  open,
  onOpenChange,
}: ScheduleDetailSheetProps) => {
  if (!schedule) {
    return null;
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="flex flex-col overflow-y-auto">
        <SheetHeader>
          <SheetTitle>{schedule.name}</SheetTitle>
        </SheetHeader>

        <div className="mt-4 flex-1 space-y-6 overflow-y-auto px-6">
          {/* Status */}
          <div className="flex items-center gap-2">
            <StatusBadge
              showDot
              status={schedule.enabled ? "completed" : "paused"}
            />
            <span className="text-muted-foreground text-sm">
              {schedule.enabled ? "Active" : "Paused"}
            </span>
          </div>

          {/* Schedule Details */}
          <div>
            <h4 className="mb-3 font-medium text-muted-foreground text-xs uppercase">
              Schedule
            </h4>
            <div className="space-y-2.5">
              <DetailRow
                icon={CalendarIcon}
                label="Cron"
                value={schedule.cron || "-"}
              />
              <DetailRow
                icon={ClockIcon}
                label="Timeout"
                value={`${schedule.timeout_secs}s`}
              />
              <DetailRow
                icon={RefreshIcon}
                label="Max Attempts"
                value={`${schedule.max_attempts}`}
              />
              <DetailRow
                icon={BriefcaseIcon}
                label="Job"
                value={schedule.name}
              />
            </div>
          </div>

          {/* Description */}
          {schedule.description && (
            <div>
              <h4 className="mb-2 font-medium text-muted-foreground text-xs uppercase">
                Description
              </h4>
              <p className="text-pretty text-muted-foreground text-sm">
                {schedule.description}
              </p>
            </div>
          )}
        </div>

        <SheetFooter>
          <Button
            className="w-full"
            render={<Link params={{ id: schedule.id }} to="/app/jobs/$id" />}
            variant="outline"
          >
            View details
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
};

export default ScheduleDetailSheet;
