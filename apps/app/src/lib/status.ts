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
