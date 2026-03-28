import { describe, expect, it, vi } from "vitest";

import { StraitAPIError } from "./errors";
import { StraitHTTPClient } from "./http";

function createClient(fetch: typeof globalThis.fetch) {
  return new StraitHTTPClient({
    baseUrl: "http://localhost:8080",
    runId: "run-1",
    runToken: "token-1",
    fetch,
    retry: {
      maxAttempts: 3,
      baseDelayMs: 0,
      maxDelayMs: 0,
    },
  });
}

describe("StraitHTTPClient", () => {
  it("retries retryable API responses and eventually succeeds", async () => {
    const fetchMock = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ detail: "try again" }), {
          status: 500,
          headers: {
            "content-type": "application/json",
          },
        })
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ status: "ok" }), {
          status: 200,
          headers: {
            "content-type": "application/json",
          },
        })
      );

    const client = createClient(fetchMock);
    const result = await client.post<{ status: string }>(
      "/checkpoint",
      {
        state: {
          phase: "planning",
        },
      },
      {
        retryable: true,
      }
    );

    expect(result).toEqual({ status: "ok" });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("does not retry non-retryable API responses", async () => {
    const fetchMock = vi.fn<typeof globalThis.fetch>().mockResolvedValue(
      new Response(JSON.stringify({ detail: "bad request" }), {
        status: 400,
        headers: {
          "content-type": "application/json",
        },
      })
    );

    const client = createClient(fetchMock);

    await expect(
      client.post(
        "/checkpoint",
        {
          state: {
            phase: "planning",
          },
        },
        {
          retryable: true,
        }
      )
    ).rejects.toBeInstanceOf(StraitAPIError);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("retries transient fetch errors for retryable requests", async () => {
    const fetchMock = vi
      .fn<typeof globalThis.fetch>()
      .mockRejectedValueOnce(new Error("connection reset"))
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ status: "ok" }), {
          status: 200,
          headers: {
            "content-type": "application/json",
          },
        })
      );

    const client = createClient(fetchMock);
    const result = await client.get<{ status: string }>("/state", {
      retryable: true,
    });

    expect(result).toEqual({ status: "ok" });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
