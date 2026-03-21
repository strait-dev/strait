import { Effect } from "effect";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ApiError,
  apiEffect,
  runWithFallback,
  runWithSentryReport,
} from "@/lib/effect-api.server";

const mockApiRequest = vi.fn();
vi.mock("@/lib/api-client.server", () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}));

const mockCaptureException = vi.fn();
vi.mock("@/lib/sentry", () => ({
  captureException: (...args: unknown[]) => mockCaptureException(...args),
}));

afterEach(() => {
  vi.clearAllMocks();
});

describe("apiEffect", () => {
  it("returns data when apiRequest succeeds", async () => {
    mockApiRequest.mockResolvedValue({ id: "1" });

    const result = await Effect.runPromise(apiEffect("/v1/jobs"));

    expect(result).toEqual({ id: "1" });
    expect(mockApiRequest).toHaveBeenCalledWith("/v1/jobs", {});
  });

  it("produces ApiError with correct fields when apiRequest throws", async () => {
    const cause = new Error("network failure");
    mockApiRequest.mockRejectedValue(cause);

    const result = await Effect.runPromiseExit(
      apiEffect("/v1/jobs", { method: "POST" })
    );

    expect(result._tag).toBe("Failure");
    if (result._tag === "Failure") {
      const error = result.cause;
      // Extract the ApiError from the Cause
      const apiError = (error as { _tag: string; error: ApiError }).error;
      expect(apiError).toBeInstanceOf(ApiError);
      expect(apiError.path).toBe("/v1/jobs");
      expect(apiError.method).toBe("POST");
      expect(apiError.cause).toBe(cause);
    }
  });

  it("defaults method to GET when not specified", async () => {
    mockApiRequest.mockRejectedValue(new Error("fail"));

    const result = await Effect.runPromiseExit(apiEffect("/v1/test"));

    expect(result._tag).toBe("Failure");
    if (result._tag === "Failure") {
      const apiError = (result.cause as { _tag: string; error: ApiError })
        .error;
      expect(apiError.method).toBe("GET");
    }
  });
});

describe("runWithFallback", () => {
  it("returns data on success", async () => {
    mockApiRequest.mockResolvedValue({ count: 5 });

    const result = await runWithFallback(apiEffect("/v1/stats"), {
      count: 0,
    });

    expect(result).toEqual({ count: 5 });
    expect(mockCaptureException).not.toHaveBeenCalled();
  });

  it("returns fallback on failure", async () => {
    mockApiRequest.mockRejectedValue(new Error("fail"));

    const result = await runWithFallback(apiEffect("/v1/stats"), {
      count: 0,
    });

    expect(result).toEqual({ count: 0 });
  });

  it("calls captureException on failure with correct tags", async () => {
    const cause = new Error("server error");
    mockApiRequest.mockRejectedValue(cause);

    await runWithFallback(apiEffect("/v1/jobs", { method: "POST" }), undefined);

    expect(mockCaptureException).toHaveBeenCalledWith(cause, {
      tags: {
        location: "server_function",
        api_path: "/v1/jobs",
        api_method: "POST",
      },
    });
  });
});

describe("runWithSentryReport", () => {
  it("returns data on success", async () => {
    mockApiRequest.mockResolvedValue([{ id: "1" }]);

    const result = await runWithSentryReport(apiEffect("/v1/jobs"));

    expect(result).toEqual([{ id: "1" }]);
    expect(mockCaptureException).not.toHaveBeenCalled();
  });

  it("rejects with original error on failure", async () => {
    const cause = new Error("not found");
    mockApiRequest.mockRejectedValue(cause);

    await expect(runWithSentryReport(apiEffect("/v1/jobs"))).rejects.toThrow(
      "not found"
    );
  });

  it("calls captureException on failure with correct tags", async () => {
    const cause = new Error("forbidden");
    mockApiRequest.mockRejectedValue(cause);

    await runWithSentryReport(
      apiEffect("/v1/keys", { method: "DELETE" })
    ).catch(() => undefined);

    expect(mockCaptureException).toHaveBeenCalledWith(cause, {
      tags: {
        location: "server_function",
        api_path: "/v1/keys",
        api_method: "DELETE",
      },
    });
  });
});
