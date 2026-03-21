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

/** Shorthand category colors for charts that group by outcome. */
export const CHART_COLORS = {
  success: "var(--success)",
  error: "var(--destructive)",
  active: "var(--info)",
  warning: "var(--warning)",
  neutral: "var(--muted-foreground)",
  compute: "var(--info)",
} as const;
