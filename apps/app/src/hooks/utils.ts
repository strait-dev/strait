/**
 * Default stale time for queries in milliseconds (5 minutes)
 * Queries will be considered fresh for this duration
 */
export const DEFAULT_STALE_TIME = 5 * 60 * 1000; // 300000ms

/**
 * Default garbage collection time for queries in milliseconds (15 minutes)
 * Unused query data will be garbage collected after this duration
 */
export const DEFAULT_GC_TIME = 15 * 60 * 1000; // 900000ms
