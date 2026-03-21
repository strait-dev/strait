const TIER_RANK: Record<string, number> = {
  free: 0,
  starter: 1,
  pro: 2,
  enterprise: 3,
};

/** Returns true if switching from `currentTier` to `targetTier` is a downgrade. */
export function isDowngrade(
  currentTier: string | undefined,
  targetTier: string | undefined
): boolean {
  if (!(currentTier && targetTier)) {
    return false;
  }
  return (TIER_RANK[targetTier] ?? 0) < (TIER_RANK[currentTier] ?? 0);
}
