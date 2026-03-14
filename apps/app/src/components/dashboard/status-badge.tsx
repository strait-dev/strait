import { cn } from "@strait/ui/utils/index.ts";
import type { DisplayStatus, StepRunStatus } from "@/hooks/api/types.ts";

type StatusBadgeStatus = DisplayStatus | StepRunStatus;

type StatusConfig = {
  label: string;
  className: string;
  dotClassName: string;
};

const STATUS_CONFIG: Record<StatusBadgeStatus, StatusConfig> = {
  queued: {
    label: "Queued",
    className: "bg-chart-2/10 text-chart-2 border-chart-2/20",
    dotClassName: "bg-chart-2",
  },
  dequeued: {
    label: "Dequeued",
    className: "bg-chart-2/10 text-chart-2 border-chart-2/20",
    dotClassName: "bg-chart-2",
  },
  executing: {
    label: "Executing",
    className: "bg-chart-3/10 text-chart-3 border-chart-3/20",
    dotClassName: "bg-chart-3 animate-pulse",
  },
  running: {
    label: "Running",
    className: "bg-chart-3/10 text-chart-3 border-chart-3/20",
    dotClassName: "bg-chart-3 animate-pulse",
  },
  completed: {
    label: "Completed",
    className: "bg-chart-1/10 text-chart-1 border-chart-1/20",
    dotClassName: "bg-chart-1",
  },
  failed: {
    label: "Failed",
    className: "bg-chart-4/10 text-chart-4 border-chart-4/20",
    dotClassName: "bg-chart-4",
  },
  timed_out: {
    label: "Timed Out",
    className: "bg-chart-4/10 text-chart-4 border-chart-4/20",
    dotClassName: "bg-chart-4",
  },
  crashed: {
    label: "Crashed",
    className: "bg-chart-4/10 text-chart-4 border-chart-4/20",
    dotClassName: "bg-chart-4",
  },
  system_failed: {
    label: "System Failed",
    className: "bg-chart-4/10 text-chart-4 border-chart-4/20",
    dotClassName: "bg-chart-4",
  },
  canceled: {
    label: "Canceled",
    className: "bg-muted text-muted-foreground border-border",
    dotClassName: "bg-muted-foreground",
  },
  expired: {
    label: "Expired",
    className: "bg-muted text-muted-foreground border-border",
    dotClassName: "bg-muted-foreground",
  },
  dead_letter: {
    label: "Dead Letter",
    className: "bg-chart-4/10 text-chart-4 border-chart-4/20",
    dotClassName: "bg-chart-4",
  },
  replay_staged: {
    label: "Replay Staged",
    className: "bg-chart-5/10 text-chart-5 border-chart-5/20",
    dotClassName: "bg-chart-5",
  },
  delayed: {
    label: "Delayed",
    className: "bg-chart-5/10 text-chart-5 border-chart-5/20",
    dotClassName: "bg-chart-5",
  },
  waiting: {
    label: "Waiting",
    className: "bg-chart-5/10 text-chart-5 border-chart-5/20",
    dotClassName: "bg-chart-5",
  },
  pending: {
    label: "Pending",
    className: "bg-muted text-muted-foreground border-border",
    dotClassName: "bg-muted-foreground",
  },
  paused: {
    label: "Paused",
    className: "bg-chart-3/10 text-chart-3 border-chart-3/20",
    dotClassName: "bg-chart-3",
  },
  skipped: {
    label: "Skipped",
    className: "bg-muted text-muted-foreground border-border",
    dotClassName: "bg-muted-foreground",
  },
};

const SIZE_VARIANTS = {
  xs: { badge: "gap-1 px-1.5 py-px text-[10px]", dot: "h-1 w-1" },
  sm: { badge: "gap-1 px-1.5 py-0.5 text-[11px]", dot: "h-1.5 w-1.5" },
  md: { badge: "gap-1.5 px-2.5 py-1 text-xs", dot: "h-1.5 w-1.5" },
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
  const sizeVariant = SIZE_VARIANTS[size];

  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full border font-medium",
        sizeVariant.badge,
        config.className
      )}
    >
      {showDot && (
        <span
          className={cn(
            "shrink-0 rounded-full",
            sizeVariant.dot,
            config.dotClassName
          )}
        />
      )}
      {config.label}
    </span>
  );
}
