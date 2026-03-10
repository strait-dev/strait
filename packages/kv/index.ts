import { Redis } from "@upstash/redis";

const url = process.env.UPSTASH_REDIS_REST_URL;
const token = process.env.UPSTASH_REDIS_REST_TOKEN;

if (!(url && token)) {
  throw new Error(
    "Missing UPSTASH_REDIS_REST_URL or UPSTASH_REDIS_REST_TOKEN environment variables"
  );
}

const redis = new Redis({
  url,
  token,
});

interface SetOptions {
  ex?: number; // TTL in milliseconds
}

export const kv = {
  async get(key: string): Promise<string | null> {
    return await redis.get(key);
  },

  async set(key: string, value: string, opts?: SetOptions): Promise<void> {
    if (opts?.ex) {
      // Convert milliseconds to seconds for Redis EX command
      const seconds = Math.ceil(opts.ex / 1000);
      await redis.setex(key, seconds, value);
    } else {
      await redis.set(key, value);
    }
  },

  async del(key: string): Promise<void> {
    await redis.del(key);
  },
};
