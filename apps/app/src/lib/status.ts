/**
 * Centralized status styling and configuration for all entities.
 * Import from here instead of defining inline in routes/components.
 */

// Event trigger status styles (used by events page and logs page)
export const EVENT_STATUS_STYLES: Record<
  string,
  { dot: string; label: string; badge: string }
> = {
  pending: {
    dot: "bg-chart-3",
    label: "Pending",
    badge: "bg-chart-3/10 text-chart-3 border-chart-3/20",
  },
  received: {
    dot: "bg-info",
    label: "Received",
    badge: "bg-info/10 text-info border-info/20",
  },
  expired: {
    dot: "bg-warning",
    label: "Expired",
    badge: "bg-warning/10 text-warning border-warning/20",
  },
  failed: {
    dot: "bg-destructive",
    label: "Failed",
    badge: "bg-destructive/10 text-destructive border-destructive/20",
  },
  canceled: {
    dot: "bg-muted-foreground",
    label: "Canceled",
    badge: "bg-muted-foreground/10 text-muted-foreground border-muted-foreground/20",
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
  "replay_staged",
  "crashed",
  "system_failed",
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
