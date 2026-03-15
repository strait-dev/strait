import type { BadgeProps } from "@strait/ui/components/badge";
import { Badge } from "@strait/ui/components/badge";
import { cn } from "@strait/ui/utils/index";
import type { DisplayStatus, StepRunStatus } from "@/hooks/api/types";

type StatusBadgeStatus = DisplayStatus | StepRunStatus;

type StatusConfig = {
  label: string;
  variant: BadgeProps["variant"];
  dotClassName: string;
};

const STATUS_CONFIG: Record<StatusBadgeStatus, StatusConfig> = {
  queued: {
    label: "Queued",
    variant: "secondary-light",
    dotClassName: "bg-muted-foreground",
  },
  dequeued: {
    label: "Dequeued",
    variant: "secondary-light",
    dotClassName: "bg-muted-foreground",
  },
  executing: {
    label: "Executing",
    variant: "info-light",
    dotClassName: "bg-info animate-pulse",
  },
  running: {
    label: "Running",
    variant: "info-light",
    dotClassName: "bg-info animate-pulse",
  },
  completed: {
    label: "Completed",
    variant: "success-light",
    dotClassName: "bg-success",
  },
  failed: {
    label: "Failed",
    variant: "destructive-light",
    dotClassName: "bg-destructive",
  },
  timed_out: {
    label: "Timed Out",
    variant: "destructive-light",
    dotClassName: "bg-destructive",
  },
  crashed: {
    label: "Crashed",
    variant: "destructive-light",
    dotClassName: "bg-destructive",
  },
  system_failed: {
    label: "System Failed",
    variant: "destructive-light",
    dotClassName: "bg-destructive",
  },
  canceled: {
    label: "Canceled",
    variant: "secondary-light",
    dotClassName: "bg-muted-foreground",
  },
  expired: {
    label: "Expired",
    variant: "secondary-light",
    dotClassName: "bg-muted-foreground",
  },
  dead_letter: {
    label: "Dead Letter",
    variant: "destructive-light",
    dotClassName: "bg-destructive",
  },
  replay_staged: {
    label: "Replay Staged",
    variant: "warning-light",
    dotClassName: "bg-warning",
  },
  delayed: {
    label: "Delayed",
    variant: "warning-light",
    dotClassName: "bg-warning",
  },
  waiting: {
    label: "Waiting",
    variant: "warning-light",
    dotClassName: "bg-warning",
  },
  pending: {
    label: "Pending",
    variant: "secondary-light",
    dotClassName: "bg-muted-foreground",
  },
  paused: {
    label: "Paused",
    variant: "warning-light",
    dotClassName: "bg-warning",
  },
  skipped: {
    label: "Skipped",
    variant: "secondary-light",
    dotClassName: "bg-muted-foreground",
  },
};

const SIZE_MAP = {
  xs: "xs",
  sm: "sm",
  md: "default",
} as const;

const DOT_SIZES = {
  xs: "size-1",
  sm: "size-1.5",
  md: "size-1.5",
} as const;

type StatusBadgeProps = {
  status: StatusBadgeStatus;
  size?: "xs" | "sm" | "md";
  showDot?: boolean;
};

export function StatusBadge({
  status,
  size = "sm",
  showDot = true,
}: StatusBadgeProps) {
  const config = STATUS_CONFIG[status] ?? STATUS_CONFIG.pending;
  return (
    <Badge size={SIZE_MAP[size]} variant={config.variant}>
      {showDot && (
        <span
          className={cn(
            "shrink-0 rounded-full",
            DOT_SIZES[size],
            config.dotClassName
          )}
        />
      )}
      {config.label}
    </Badge>
  );
}
