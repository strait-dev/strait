/**
 * Pure utility functions for agent billing data formatting.
 * Separated from the server function hooks to enable unit testing
 * without pulling in the server-side import chain.
 */

/**
 * Formats micro-USD to a human-readable USD string.
 * Returns "-" for negative values (disabled state).
 */
export function formatMicroUsd(microusd: number): string {
  if (microusd < 0) {
    return "-";
  }
  return `$${(microusd / 1_000_000).toFixed(2)}`;
}

/**
 * Formats a token count with K/M suffix for display.
 */
export function formatTokenCount(count: number): string {
  if (count >= 1_000_000) {
    return `${(count / 1_000_000).toFixed(1)}M`;
  }
  if (count >= 1000) {
    return `${(count / 1000).toFixed(1)}K`;
  }
  return count.toLocaleString();
}

/**
 * Returns a display-friendly agent plan tier name.
 */
export function formatAgentPlanTier(tier: string): string {
  const tierMap: Record<string, string> = {
    agent_free: "Free",
    agent_maker: "Maker",
    agent_growth: "Growth",
    agent_enterprise: "Enterprise",
  };
  return tierMap[tier] ?? tier;
}

/**
 * Computes the credit usage percentage (0-100+).
 * Can exceed 100% when in overage.
 */
export function computeCreditPercent(
  usedUsd: number,
  includedUsd: number
): number {
  if (includedUsd <= 0) {
    return 0;
  }
  return (usedUsd / includedUsd) * 100;
}

/** Spending limit preset amounts in USD. */
export const AGENT_SPENDING_PRESETS = [25, 50, 100, 250, 500] as const;

/** Converts a USD amount to micro-USD. */
export function usdToMicrousd(usd: number): number {
  return Math.round(usd * 1_000_000);
}
