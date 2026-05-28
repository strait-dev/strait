/**
 * Centralized status styling and configuration for all entities.
 * Import from here instead of defining inline in routes/components.
 */

// Event trigger status styles (used by events page and logs page)
export const EVENT_STATUS_STYLES: Record<
  string,
  { dot: string; label: string; badge: string }
> = {
  waiting: {
    dot: "bg-info",
    label: "Waiting",
    badge: "bg-info/10 text-info border-info/20",
  },
  received: {
    dot: "bg-info",
    label: "Received",
    badge: "bg-info/10 text-info border-info/20",
  },
  timed_out: {
    dot: "bg-warning",
    label: "Timed Out",
    badge: "bg-warning/10 text-warning border-warning/20",
  },
  canceled: {
    dot: "bg-muted-foreground",
    label: "Canceled",
    badge:
      "bg-muted-foreground/10 text-muted-foreground border-muted-foreground/20",
  },
};

export const EVENT_STATUSES = Object.keys(EVENT_STATUS_STYLES);

// Run status filter options (used by runs, DLQ pages)
export const RUN_STATUS_OPTIONS = [
  "queued",
  "executing",
  "completed",
  "failed",
  "timed_out",
  "canceled",
  "dead_letter",
  "crashed",
  "system_failed",
] as const;

// Workflow run status filter options (used by the workflow detail Recent Runs
// tab). Mirrors domain.WorkflowRunStatus, including the continue-as-new
// terminal state "continued".
export const WORKFLOW_RUN_STATUS_OPTIONS = [
  "pending",
  "running",
  "paused",
  "completed",
  "failed",
  "timed_out",
  "canceled",
  "continued",
] as const;

// Job/Schedule/Workflow enabled/disabled options
export const ENABLED_STATUS_OPTIONS = ["Enabled", "Disabled"] as const;

// Webhook status options
export const WEBHOOK_STATUS_OPTIONS = ["Active", "Inactive"] as const;

// DLQ error type options
export const DLQ_ERROR_TYPES = [
  "timeout",
  "crash",
  "oom",
  "runtime",
  "network",
  "permission",
  "dependency",
  "configuration",
  "rate_limit",
  "internal",
] as const;

// Run status badge configuration (used by StatusBadge component)
export type RunStatusConfig = {
  label: string;
  variant:
    | "default"
    | "secondary-light"
    | "info-light"
    | "success-light"
    | "destructive-light"
    | "warning-light";
  dotClassName: string;
};

export const RUN_STATUS_CONFIG: Record<string, RunStatusConfig> = {
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
  continued: {
    label: "Continued",
    variant: "secondary-light",
    dotClassName: "bg-muted-foreground",
  },
  skipped: {
    label: "Skipped",
    variant: "secondary-light",
    dotClassName: "bg-muted-foreground",
  },
};
