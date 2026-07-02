import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  DescriptionDetails,
  DescriptionList,
  DescriptionTerm,
} from "@strait/ui/components/description-list";
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
import {
  BriefcaseIcon,
  CalendarIcon,
  ClockIcon,
  RefreshIcon,
} from "@/lib/icons";

type ScheduleDetailSheetProps = {
  schedule: Job | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
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
            <DescriptionList orientation="horizontal" size="sm">
              <DescriptionTerm>
                <HugeiconsIcon className="size-3.5" icon={CalendarIcon} />
                Cron
              </DescriptionTerm>
              <DescriptionDetails className="font-mono">
                {schedule.cron || "-"}
              </DescriptionDetails>
              <DescriptionTerm>
                <HugeiconsIcon className="size-3.5" icon={ClockIcon} />
                Timeout
              </DescriptionTerm>
              <DescriptionDetails className="font-mono">
                {schedule.timeout_secs}s
              </DescriptionDetails>
              <DescriptionTerm>
                <HugeiconsIcon className="size-3.5" icon={RefreshIcon} />
                Max attempts
              </DescriptionTerm>
              <DescriptionDetails className="font-mono">
                {schedule.max_attempts}
              </DescriptionDetails>
              <DescriptionTerm>
                <HugeiconsIcon className="size-3.5" icon={BriefcaseIcon} />
                Job
              </DescriptionTerm>
              <DescriptionDetails className="font-mono">
                {schedule.name}
              </DescriptionDetails>
            </DescriptionList>
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
