/**
 * Default stale time for queries in milliseconds (5 minutes)
 * Queries will be considered fresh for this duration
 */
export const DEFAULT_STALE_TIME = 5 * 60 * 1000; // 300000ms

/**
 * Stale time for high-churn views like runs lists (30 seconds)
 */
export const HIGH_CHURN_STALE_TIME = 30 * 1000; // 30000ms

/**
 * Refetch interval for live views like the activity feed (10 seconds)
 */
export const LIVE_REFETCH_INTERVAL = 10 * 1000; // 10000ms

/**
 * Default garbage collection time for queries in milliseconds (15 minutes)
 * Unused query data will be garbage collected after this duration
 */
export const DEFAULT_GC_TIME = 15 * 60 * 1000; // 900000ms
