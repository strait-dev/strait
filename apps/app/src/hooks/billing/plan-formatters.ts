/**
 * Pure formatting utilities for plan data display.
 *
 * These functions transform raw plan values into human-readable strings
 * for the pricing page, comparison table, and billing dashboard.
 * Separated from use-plans.ts to allow testing without server-side imports.
 */

const MICRO_TO_DOLLARS = 1_000_000;

/**
 * Format a numeric limit value for display.
 * Returns "Unlimited" for `-1`, formatted number for >= 1000.
 *
 * @param value - The limit value. `-1` means unlimited.
 * @returns Formatted string (e.g. "Unlimited", "1,000", "50").
 */
export const formatLimit = (value: number): string => {
  if (value === -1) {
    return "Unlimited";
  }
  if (value >= 1000) {
    return value.toLocaleString("en-US");
  }
  return String(value);
};

/**
 * Format a micro-USD price amount for display.
 *
 * @param microusd - Price amount in micro-USD (1,000,000 = $1.00).
 * @returns Formatted string (e.g. "$19.99") or "-" for zero/negative.
 */
export const formatMicroUsdPrice = (microusd: number): string => {
  if (microusd <= 0) {
    return "-";
  }
  return `$${(microusd / MICRO_TO_DOLLARS).toFixed(2)}`;
};

/**
 * Format a retention days value for display.
 *
 * @param days - Number of retention days.
 * @returns Formatted string (e.g. "1 day", "30 days").
 */
export const formatRetention = (days: number): string => {
  if (days === 1) {
    return "1 day";
  }
  return `${days} days`;
};

/**
 * Format a cron minimum interval in seconds.
 *
 * @param seconds - Minimum interval in seconds. `0` means sub-second.
 * @returns Human-readable interval.
 */
export const formatCronInterval = (seconds: number): string => {
  if (seconds === 0) {
    return "sub-second";
  }
  if (seconds < 60) {
    return `${seconds} sec`;
  }
  const minutes = seconds / 60;
  return minutes === 1 ? "1 min" : `${minutes} min`;
};

/**
 * Format an RBAC level string for display.
 *
 * @param level - RBAC level ("", "basic", "full", "advanced").
 * @returns Capitalized level or "-" for empty string.
 */
export const formatRBAC = (level: string): string => {
  if (!level) {
    return "-";
  }
  return level.charAt(0).toUpperCase() + level.slice(1);
};

/**
 * Format a boolean feature flag for display.
 *
 * @param value - Whether the feature is available.
 * @returns "Yes" for true, "-" for false.
 */
export const formatBoolean = (value: boolean): string => (value ? "Yes" : "-");

/** Human-readable support level labels. */
const SUPPORT_LABELS: Record<string, string> = {
  community: "Community support",
  email_72h: "Email support (72h)",
  priority_24h: "Priority support (24h)",
  priority_slack_8h: "Priority support + Slack (8h)",
  dedicated: "Dedicated support + CSM",
};

/**
 * Format a support level identifier into a human-readable label.
 *
 * @param level - Support level identifier (e.g. "community", "dedicated").
 * @returns Human-readable label, or the raw level if not recognized.
 */
export const formatSupportLevel = (level: string): string =>
  SUPPORT_LABELS[level] ?? level;
