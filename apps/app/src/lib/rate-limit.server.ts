import { kvIncrementWithTtl } from "@/lib/kv.server";

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
  const count = await kvIncrementWithTtl(key, { ex: windowSeconds });

  if (count > limit) {
    throw new Error("Too many requests");
  }
}
