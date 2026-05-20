import { Redis } from "@upstash/redis";

/**
 * Lazily initialized Upstash Redis client singleton.
 *
 * Initialization is deferred because Cloudflare Workers only populate
 * `process.env` during request handling, not at module load time.
 * Returns `null` if the Upstash env vars are not configured.
 */
let _kv: Redis | null | undefined;

function getKv(): Redis | null {
  if (_kv === undefined) {
    _kv =
      process.env.UPSTASH_REDIS_REST_URL && process.env.UPSTASH_REDIS_REST_TOKEN
        ? new Redis({
            url: process.env.UPSTASH_REDIS_REST_URL,
            token: process.env.UPSTASH_REDIS_REST_TOKEN,
          })
        : null;
  }
  return _kv;
}

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

function memoryDelete(key: string): void {
  memoryStore.delete(key);
}

function memorySetIfAbsent(
  key: string,
  value: string,
  ttlSeconds: number
): boolean {
  if (memoryGet(key) !== null) {
    return false;
  }
  memorySet(key, value, ttlSeconds);
  return true;
}

/**
 * Get a value from the KV store. Tries Redis first, falls back to memory.
 */
export async function kvGet(key: string): Promise<string | null> {
  const kv = getKv();
  if (kv) {
    try {
      const value = await kv.get(key);
      if (value != null) {
        return value.toString();
      }
    } catch {
      throw new Error("KV read failed");
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
  const kv = getKv();

  if (kv) {
    try {
      await kv.set(key, value, opts);
      return;
    } catch {
      throw new Error("KV write failed");
    }
  }

  memorySet(key, value, ttl);
}

export async function kvDelete(key: string): Promise<void> {
  const kv = getKv();
  if (kv) {
    try {
      await kv.del(key);
      return;
    } catch {
      throw new Error("KV delete failed");
    }
  }
  memoryDelete(key);
}

export async function kvGetDelete(key: string): Promise<string | null> {
  const kv = getKv();
  if (kv) {
    try {
      const value = await kv.getdel(key);
      return value == null ? null : value.toString();
    } catch {
      throw new Error("KV consume failed");
    }
  }

  const value = memoryGet(key);
  memoryDelete(key);
  return value;
}

export async function kvSetIfAbsent(
  key: string,
  value: string,
  opts?: { ex: number }
): Promise<boolean> {
  const ttl = opts?.ex ?? 300;
  const kv = getKv();
  if (kv) {
    try {
      const result = opts
        ? await kv.set(key, value, { ex: opts.ex, nx: true })
        : await kv.set(key, value, { nx: true });
      return result === "OK";
    } catch {
      throw new Error("KV conditional write failed");
    }
  }

  return memorySetIfAbsent(key, value, ttl);
}
