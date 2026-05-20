import { kvGet, kvSet } from "@/lib/kv.server";

type RateLimitOptions = {
  key: string;
  limit: number;
  windowSeconds: number;
};

export async function enforceRateLimit({
  key,
  limit,
  windowSeconds,
}: RateLimitOptions): Promise<void> {
  const stored = await kvGet(key);
  const count = stored ? Number.parseInt(stored, 10) : 0;

  if (Number.isFinite(count) && count >= limit) {
    throw new Error("Too many requests");
  }

  await kvSet(key, String((Number.isFinite(count) ? count : 0) + 1), {
    ex: windowSeconds,
  });
}
