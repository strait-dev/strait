import { describe, expect, test } from "bun:test";
import { Schema } from "effect";

import { createClient } from "../src/client";
import type { FetchLike } from "../src/runtime";

const makeJsonResponse = (status: number, body: unknown): Response =>
  new Response(JSON.stringify(body), {
    status,
    headers: {
      "Content-Type": "application/json",
    },
  });

describe("createClient", () => {
  test("injects auth header and resolves path params", async () => {
    let capturedRequest: RequestInit | undefined;
    let capturedUrl = "";

    const fetchImpl: FetchLike = (input, init) => {
      capturedUrl = String(input);
      capturedRequest = init;
      return Promise.resolve(makeJsonResponse(201, { ok: true }));
    };

    const client = createClient(
      {
        baseUrl: "https://strait.dev/",
        auth: { type: "runToken", token: "rt_123" },
      },
      { fetch: fetchImpl }
    );

    const result = await client.operationsPromise.postSdkV1RunsByRunIDLog({
      pathParams: { runID: "run-123" },
      body: { message: "hello" },
      successStatus: [201],
    });

    expect(result).toEqual({ ok: true });
    expect(capturedUrl).toBe("https://strait.dev/sdk/v1/runs/run-123/log");
    expect(
      (capturedRequest?.headers as Record<string, string>).Authorization
    ).toBe("Bearer rt_123");
  });

  test("fails when required path params are missing", async () => {
    const client = createClient({
      baseUrl: "https://strait.dev",
      auth: { type: "bearer", token: "abc" },
    });

    await expect(
      client.operationsPromise.getV1RunsByRunID()
    ).rejects.toMatchObject({
      _tag: "ValidationError",
      message: "missing path parameter: runID",
    });
  });

  test("maps 429 to RateLimitedError", async () => {
    const fetchImpl: FetchLike = () =>
      Promise.resolve(
        makeJsonResponse(429, { error: "per-run cost budget exceeded" })
      );

    const client = createClient(
      {
        baseUrl: "https://strait.dev",
        auth: { type: "bearer", token: "abc" },
      },
      { fetch: fetchImpl }
    );

    await expect(
      client.operationsPromise.postSdkV1RunsByRunIDUsage({
        pathParams: { runID: "run-1" },
        body: {
          provider: "openai",
          model: "gpt",
          prompt_tokens: 1,
          completion_tokens: 1,
        },
      })
    ).rejects.toMatchObject({ _tag: "RateLimitedError", status: 429 });
  });

  test("fails with DecodeError when response schema does not match", async () => {
    const fetchImpl: FetchLike = () =>
      Promise.resolve(makeJsonResponse(200, { status: 42 }));

    const client = createClient(
      {
        baseUrl: "https://strait.dev",
        auth: { type: "bearer", token: "abc" },
      },
      { fetch: fetchImpl }
    );

    await expect(
      client.operationsPromise.getHealth({
        responseSchema: Schema.Struct({ status: Schema.String }),
      })
    ).rejects.toMatchObject({ _tag: "DecodeError" });
  });

  test("uses generated request schema before transport", async () => {
    let fetchCalls = 0;

    const fetchImpl: FetchLike = () => {
      fetchCalls += 1;
      return Promise.resolve(makeJsonResponse(201, { ok: true }));
    };

    const client = createClient(
      {
        baseUrl: "https://strait.dev",
        auth: { type: "runToken", token: "rt_abc" },
      },
      { fetch: fetchImpl }
    );

    await expect(
      client.operationsPromise.postSdkV1RunsByRunIDUsage({
        pathParams: { runID: "run-1" },
        body: { model: "gpt-4o" },
      })
    ).rejects.toMatchObject({
      _tag: "DecodeError",
      message: "request schema encoding failed",
    });

    expect(fetchCalls).toBe(0);
  });

  test("uses generated response schema by default", async () => {
    const fetchImpl: FetchLike = () =>
      Promise.resolve(makeJsonResponse(200, { status: 42 }));

    const client = createClient(
      {
        baseUrl: "https://strait.dev",
        auth: { type: "bearer", token: "abc" },
      },
      { fetch: fetchImpl }
    );

    await expect(client.operationsPromise.getHealth()).rejects.toMatchObject({
      _tag: "DecodeError",
      message: "response schema validation failed",
    });
  });
});
