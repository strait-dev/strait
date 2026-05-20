import { beforeEach, describe, expect, it, vi } from "vitest";

const mockRedisDel = vi.fn();
const mockRedisGet = vi.fn();
const mockRedisGetDel = vi.fn();
const mockRedisSet = vi.fn();

vi.mock("@upstash/redis", () => ({
  Redis: class {
    del = mockRedisDel;
    get = mockRedisGet;
    getdel = mockRedisGetDel;
    set = mockRedisSet;
  },
}));

async function importKvWithRedis() {
  vi.resetModules();
  vi.stubEnv("UPSTASH_REDIS_REST_URL", "https://fake.upstash.io");
  vi.stubEnv("UPSTASH_REDIS_REST_TOKEN", "fake-token");
  return await import("@/lib/kv.server");
}

async function importKvWithoutRedis() {
  vi.resetModules();
  vi.stubEnv("UPSTASH_REDIS_REST_URL", "");
  vi.stubEnv("UPSTASH_REDIS_REST_TOKEN", "");
  return await import("@/lib/kv.server");
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.useRealTimers();
});

describe("kvGet", () => {
  it("returns Redis value when available", async () => {
    const { kvGet } = await importKvWithRedis();
    mockRedisGet.mockResolvedValue("cached-value");

    const result = await kvGet("test-key");

    expect(result).toBe("cached-value");
    expect(mockRedisGet).toHaveBeenCalledWith("test-key");
  });

  it("fails closed when Redis reads fail", async () => {
    const { kvGet } = await importKvWithRedis();
    mockRedisGet.mockRejectedValue(new Error("read error"));

    await expect(kvGet("test-key")).rejects.toThrow("KV read failed");
  });
});

describe("kvSet", () => {
  it("writes to Redis when available", async () => {
    const { kvSet } = await importKvWithRedis();
    mockRedisSet.mockResolvedValue("OK");

    await kvSet("redis-key", "redis-value", { ex: 300 });

    expect(mockRedisSet).toHaveBeenCalledWith("redis-key", "redis-value", {
      ex: 300,
    });
  });

  it("does not permanently bypass Redis after a transient write failure", async () => {
    const { kvSet } = await importKvWithRedis();
    mockRedisSet
      .mockRejectedValueOnce(new Error("temporary"))
      .mockResolvedValueOnce("OK");

    await expect(kvSet("key", "value", { ex: 60 })).rejects.toThrow(
      "KV write failed"
    );
    await kvSet("key", "value", { ex: 60 });

    expect(mockRedisSet).toHaveBeenCalledTimes(2);
  });
});

describe("one-time and conditional operations", () => {
  it("uses Redis GETDEL to consume values atomically", async () => {
    const { kvGetDelete } = await importKvWithRedis();
    mockRedisGetDel.mockResolvedValue("one-time-value");

    await expect(kvGetDelete("token")).resolves.toBe("one-time-value");
    expect(mockRedisGetDel).toHaveBeenCalledWith("token");
  });

  it("deletes Redis keys", async () => {
    const { kvDelete } = await importKvWithRedis();
    mockRedisDel.mockResolvedValue(1);

    await kvDelete("token");

    expect(mockRedisDel).toHaveBeenCalledWith("token");
  });

  it("uses Redis SET NX for conditional writes", async () => {
    const { kvSetIfAbsent } = await importKvWithRedis();
    mockRedisSet.mockResolvedValueOnce("OK").mockResolvedValueOnce(null);

    await expect(kvSetIfAbsent("cooldown", "now", { ex: 60 })).resolves.toBe(
      true
    );
    await expect(kvSetIfAbsent("cooldown", "later", { ex: 60 })).resolves.toBe(
      false
    );
    expect(mockRedisSet).toHaveBeenCalledWith("cooldown", "now", {
      ex: 60,
      nx: true,
    });
  });
});

describe("memory fallback without Redis configuration", () => {
  it("supports get, set, conditional set, and consume", async () => {
    const { kvGet, kvGetDelete, kvSet, kvSetIfAbsent } =
      await importKvWithoutRedis();

    await kvSet("mem-key", "mem-value", { ex: 60 });
    await expect(kvGet("mem-key")).resolves.toBe("mem-value");
    await expect(
      kvSetIfAbsent("mem-key", "new-value", { ex: 60 })
    ).resolves.toBe(false);
    await expect(kvGetDelete("mem-key")).resolves.toBe("mem-value");
    await expect(kvGet("mem-key")).resolves.toBe(null);
  });

  it("expires entries after TTL", async () => {
    const { kvGet, kvSet } = await importKvWithoutRedis();
    vi.useFakeTimers();

    await kvSet("expiring-key", "temp-value", { ex: 10 });
    await expect(kvGet("expiring-key")).resolves.toBe("temp-value");

    vi.advanceTimersByTime(11_000);

    await expect(kvGet("expiring-key")).resolves.toBe(null);
  });
});
