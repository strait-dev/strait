import { Redis } from "@upstash/redis";

const kv =
  process.env.UPSTASH_REDIS_REST_URL && process.env.UPSTASH_REDIS_REST_TOKEN
    ? new Redis({
        url: process.env.UPSTASH_REDIS_REST_URL,
        token: process.env.UPSTASH_REDIS_REST_TOKEN,
      })
    : null;

/**
 * In-memory fallback store for when Redis is unavailable or read-only.
 * Keys auto-expire based on TTL. Not suitable for multi-instance deployments
 * but works for single-server and development.
 */
const memoryStore = new Map<string, { value: string; expiresAt: number }>();

function memoryGet(key: string): string | null {
  const entry = memoryStore.get(key);
  if (!entry) {
    return null;
  }
  if (Date.now() > entry.expiresAt) {
    memoryStore.delete(key);
    return null;
  }
  return entry.value;
}

function memorySet(key: string, value: string, ttlSeconds: number): void {
  memoryStore.set(key, {
    value,
    expiresAt: Date.now() + ttlSeconds * 1000,
  });
}

/** Whether Redis writes have failed and we're using the memory fallback. */
let useMemoryFallback = false;

/**
 * Get a value from the KV store. Tries Redis first, falls back to memory.
 */
export async function kvGet(key: string): Promise<string | null> {
  if (!useMemoryFallback && kv) {
    try {
      const value = await kv.get(key);
      if (value != null) {
        return value.toString();
      }
    } catch {
      // Redis read failed, fall through to memory
    }
  }
  return memoryGet(key);
}

/**
 * Set a value in the KV store. Tries Redis first, falls back to in-memory
 * store with TTL expiration if Redis writes are not permitted.
 */
export async function kvSet(
  key: string,
  value: string,
  opts?: { ex: number }
): Promise<void> {
  const ttl = opts?.ex ?? 300;

  if (!useMemoryFallback && kv) {
    try {
      await kv.set(key, value, opts);
      return;
    } catch {
      useMemoryFallback = true;
    }
  }

  memorySet(key, value, ttl);
}
