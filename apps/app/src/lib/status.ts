export const EVENT_STATUSES = [
  "waiting",
  "received",
  "timed_out",
  "canceled",
] as const;

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
