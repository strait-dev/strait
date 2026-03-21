import { Badge } from "@strait/ui/components/badge";
import { cn } from "@strait/ui/utils/index";
import type { DisplayStatus, StepRunStatus } from "@/hooks/api/types";
import { RUN_STATUS_CONFIG } from "@/lib/status";

type StatusBadgeStatus = DisplayStatus | StepRunStatus;

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

const StatusBadge = ({
  status,
  size = "md",
  showDot = true,
}: StatusBadgeProps) => {
  const config = RUN_STATUS_CONFIG[status] ?? RUN_STATUS_CONFIG.pending;
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
};

export default StatusBadge;
