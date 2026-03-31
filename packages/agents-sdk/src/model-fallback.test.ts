import { describe, expect, it, vi } from "vitest";
import { isRetriableError, withModelFallback } from "./model-fallback";

describe("isRetriableError", () => {
  it("detects rate limit by status 429", () => {
    const err = Object.assign(new Error("too many requests"), { status: 429 });
    expect(isRetriableError(err, ["rate_limit"])).toBe(true);
  });

  it("detects rate limit by message", () => {
    expect(
      isRetriableError(new Error("rate limit exceeded"), ["rate_limit"])
    ).toBe(true);
  });

  it("detects timeout by message", () => {
    expect(isRetriableError(new Error("request timed out"), ["timeout"])).toBe(
      true
    );
  });

  it("detects unavailable by status 503", () => {
    const err = Object.assign(new Error("service unavailable"), {
      status: 503,
    });
    expect(isRetriableError(err, ["unavailable"])).toBe(true);
  });

  it("returns false for auth errors", () => {
    const err = Object.assign(new Error("unauthorized"), { status: 401 });
    expect(
      isRetriableError(err, ["rate_limit", "timeout", "unavailable"])
    ).toBe(false);
  });

  it("returns false for content policy errors", () => {
    expect(
      isRetriableError(new Error("content policy violation"), ["rate_limit"])
    ).toBe(false);
  });

  it("returns false for null", () => {
    expect(isRetriableError(null, ["rate_limit"])).toBe(false);
  });
});

describe("withModelFallback", () => {
  it("returns primary result when no error", async () => {
    const result = await withModelFallback(
      (model) => Promise.resolve(`result-from-${model}`),
      "gpt-5.4",
      { fallbacks: ["gpt-5.4-mini"] }
    );
    expect(result).toBe("result-from-gpt-5.4");
  });

  it("falls back to next model on rate limit", async () => {
    let callCount = 0;
    const result = await withModelFallback(
      (model) => {
        callCount++;
        if (callCount === 1) {
          throw Object.assign(new Error("rate limit"), { status: 429 });
        }
        return Promise.resolve(`result-from-${model}`);
      },
      "claude-sonnet-4-6",
      { fallbacks: ["gpt-5.4-mini"] }
    );
    expect(result).toBe("result-from-gpt-5.4-mini");
    expect(callCount).toBe(2);
  });

  it("calls onFallback callback", async () => {
    const onFallback = vi.fn();
    let callCount = 0;
    await withModelFallback(
      () => {
        callCount++;
        if (callCount === 1) {
          throw Object.assign(new Error("rate limit"), { status: 429 });
        }
        return Promise.resolve("ok");
      },
      "primary",
      { fallbacks: ["secondary"], onFallback }
    );
    expect(onFallback).toHaveBeenCalledWith(
      "primary",
      "secondary",
      expect.any(Error)
    );
  });

  it("does not fall back on auth errors", async () => {
    await expect(
      withModelFallback(
        () => {
          throw Object.assign(new Error("unauthorized"), { status: 401 });
        },
        "primary",
        { fallbacks: ["secondary"] }
      )
    ).rejects.toThrow("unauthorized");
  });

  it("exhausts all fallbacks and throws original error", async () => {
    const rateErr = Object.assign(new Error("rate limit"), { status: 429 });
    await expect(
      withModelFallback(
        () => {
          throw rateErr;
        },
        "a",
        { fallbacks: ["b", "c"] }
      )
    ).rejects.toThrow("rate limit");
  });

  it("handles empty fallback list", async () => {
    await expect(
      withModelFallback(
        () => {
          throw Object.assign(new Error("rate limit"), { status: 429 });
        },
        "primary",
        { fallbacks: [] }
      )
    ).rejects.toThrow("rate limit");
  });

  it("respects custom retryOn conditions", async () => {
    let callCount = 0;
    const result = await withModelFallback(
      (model) => {
        callCount++;
        if (callCount === 1) {
          throw new Error("request timed out");
        }
        return Promise.resolve(`result-from-${model}`);
      },
      "primary",
      { fallbacks: ["fallback"], retryOn: ["timeout"] }
    );
    expect(result).toBe("result-from-fallback");
  });

  it("does not retry on conditions not in retryOn", async () => {
    await expect(
      withModelFallback(
        () => {
          throw Object.assign(new Error("rate limit"), { status: 429 });
        },
        "primary",
        { fallbacks: ["fallback"], retryOn: ["timeout"] }
      )
    ).rejects.toThrow("rate limit");
  });
});
