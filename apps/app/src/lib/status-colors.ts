/**
 * Semantic chart colors mapped to FSM status categories.
 * Must stay in sync with StatusBadge config in status-badge.tsx.
 *
 * Categories:
 *  success  (green)  — completed
 *  error    (red)    — failed, timed_out, crashed, system_failed, dead_letter
 *  active   (blue)   — executing, running
 *  warning  (amber)  — replay_staged, delayed, waiting, paused
 *  neutral  (gray)   — queued, dequeued, canceled, expired, pending, skipped
 */

export const STATUS_CHART_COLORS = {
  // Success
  completed: "var(--success)",

  // Error
  failed: "var(--destructive)",
  timed_out: "var(--destructive)",
  crashed: "var(--destructive)",
  system_failed: "var(--destructive)",
  dead_letter: "var(--destructive)",

  // Active
  executing: "var(--info)",
  running: "var(--info)",

  // Warning
  replay_staged: "var(--warning)",
  delayed: "var(--warning)",
  waiting: "var(--warning)",
  paused: "var(--warning)",

  // Neutral
  queued: "var(--muted-foreground)",
  dequeued: "var(--muted-foreground)",
  canceled: "var(--muted-foreground)",
  expired: "var(--muted-foreground)",
  pending: "var(--muted-foreground)",
  skipped: "var(--muted-foreground)",
} as const;

/** Shorthand category colors for charts that group by outcome. */
export const CHART_COLORS = {
  success: "var(--success)",
  error: "var(--destructive)",
  active: "var(--info)",
  warning: "var(--warning)",
  neutral: "var(--muted-foreground)",
} as const;
