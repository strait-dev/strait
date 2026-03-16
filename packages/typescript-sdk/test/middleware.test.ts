import { describe, expect, test } from "bun:test";

import type { Middleware } from "../src/client";

describe("Middleware type", () => {
  test("onRequest middleware receives correct context shape", async () => {
    let captured: Record<string, unknown> = {};

    const middleware: Middleware = {
      onRequest: (ctx) => {
        captured = { ...ctx };
      },
    };

    await middleware.onRequest?.({
      method: "POST",
      url: "https://api.strait.io/v1/jobs",
      headers: { Authorization: "Bearer token" },
      body: { name: "test" },
    });

    expect(captured.method).toBe("POST");
    expect(captured.url).toBe("https://api.strait.io/v1/jobs");
    expect(captured.headers).toEqual({ Authorization: "Bearer token" });
    expect(captured.body).toEqual({ name: "test" });
  });

  test("onResponse middleware receives status and duration", async () => {
    let captured: Record<string, unknown> = {};

    const middleware: Middleware = {
      onResponse: (ctx) => {
        captured = { ...ctx };
      },
    };

    await middleware.onResponse?.({
      method: "GET",
      url: "https://api.strait.io/v1/runs/run_1",
      status: 200,
      durationMs: 42,
    });

    expect(captured.status).toBe(200);
    expect(captured.durationMs).toBe(42);
  });

  test("onError middleware receives error context", async () => {
    let captured: Record<string, unknown> = {};

    const middleware: Middleware = {
      onError: (ctx) => {
        captured = { ...ctx };
      },
    };

    const testError = new Error("connection refused");
    await middleware.onError?.({
      method: "POST",
      url: "https://api.strait.io/v1/jobs",
      error: testError,
    });

    expect(captured.error).toBe(testError);
    expect(captured.method).toBe("POST");
  });

  test("middleware hooks are optional", () => {
    const middleware: Middleware = {};
    expect(middleware.onRequest).toBeUndefined();
    expect(middleware.onResponse).toBeUndefined();
    expect(middleware.onError).toBeUndefined();
  });

  test("multiple middleware can be composed as an array", async () => {
    const calls: string[] = [];

    const mw1: Middleware = {
      onRequest: () => {
        calls.push("mw1");
      },
    };
    const mw2: Middleware = {
      onRequest: () => {
        calls.push("mw2");
      },
    };

    const middlewares: readonly Middleware[] = [mw1, mw2];

    const ctx = {
      method: "GET",
      url: "https://api.strait.io",
      headers: {},
    };

    for (const mw of middlewares) {
      await mw.onRequest?.(ctx);
    }

    expect(calls).toEqual(["mw1", "mw2"]);
  });

  test("async middleware hooks are awaitable", async () => {
    let resolved = false;

    const middleware: Middleware = {
      onRequest: async () => {
        await Promise.resolve();
        resolved = true;
      },
    };

    await middleware.onRequest?.({
      method: "POST",
      url: "https://api.strait.io",
      headers: {},
    });

    expect(resolved).toBe(true);
  });
});
