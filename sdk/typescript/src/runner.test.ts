import { describe, expect, it, beforeEach, afterEach, mock, spyOn } from "bun:test";

// Mock fetch at module level.
const fetchCalls: Array<{ url: string; init: RequestInit }> = [];
const fetchResponses: Array<Response> = [];

globalThis.fetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
  const url = typeof input === "string" ? input : input.toString();
  fetchCalls.push({ url, init: init ?? {} });
  if (fetchResponses.length > 0) return fetchResponses.shift()!;
  return new Response(JSON.stringify({}), { status: 200 });
};

// Stub process.exit.
let exitCalled: number | null = null;
const origExit = process.exit;
process.exit = ((code?: number) => { exitCalled = code ?? 0; }) as never;

import { StraitClient } from "./client";
import { StraitRunner } from "./runner";

function okResponse(body: unknown = {}): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } });
}

function makeEnv(overrides: Record<string, string> = {}): Record<string, string> {
  return {
    STRAIT_RUN_ID: "run-123",
    STRAIT_SDK_TOKEN: "tok-secret",
    STRAIT_API_URL: "https://api.test.com",
    STRAIT_JOB_SLUG: "my-job",
    STRAIT_ATTEMPT: "2",
    STRAIT_PAYLOAD_MODE: "inline",
    STRAIT_PAYLOAD: '{"key":"value"}',
    ...overrides,
  };
}

function setEnv(env: Record<string, string>): void {
  for (const [k, v] of Object.entries(env)) process.env[k] = v;
}

function clearEnv(): void {
  for (const key of Object.keys(process.env)) {
    if (key.startsWith("STRAIT_")) delete process.env[key];
  }
}

beforeEach(() => {
  fetchCalls.length = 0;
  fetchResponses.length = 0;
  exitCalled = null;
  clearEnv();
});

afterEach(() => clearEnv());

describe("StraitRunner", () => {
  it("calls /complete when handler succeeds", async () => {
    fetchResponses.push(okResponse(), okResponse()); // heartbeat, complete
    setEnv(makeEnv());

    const runner = StraitRunner.fromEnv();
    await runner.run(async () => ({ answer: 42 }));

    const completeCall = fetchCalls.find(c => c.url.includes("/complete"));
    expect(completeCall).toBeDefined();
    const body = JSON.parse(completeCall!.init.body as string);
    expect(body.result).toEqual({ answer: 42 });
  });

  it("calls /fail when handler throws", async () => {
    fetchResponses.push(okResponse(), okResponse());
    setEnv(makeEnv());

    const runner = StraitRunner.fromEnv();
    await runner.run(async () => { throw new Error("handler broke"); });

    const failCall = fetchCalls.find(c => c.url.includes("/fail"));
    expect(failCall).toBeDefined();
    const body = JSON.parse(failCall!.init.body as string);
    expect(body.error).toBe("handler broke");
  });

  it("passes inline payload from STRAIT_PAYLOAD", async () => {
    fetchResponses.push(okResponse());
    setEnv(makeEnv());

    const runner = StraitRunner.fromEnv();
    let capturedPayload: unknown;
    await runner.run(async (ctx) => { capturedPayload = ctx.payload; return "ok"; });

    expect(capturedPayload).toEqual({ key: "value" });
  });

  it("fetches payload when mode is fetch", async () => {
    fetchResponses.push(okResponse({ payload: { fetched: true } }), okResponse());
    setEnv(makeEnv({ STRAIT_PAYLOAD_MODE: "fetch" }));

    const runner = StraitRunner.fromEnv();
    let capturedPayload: unknown;
    await runner.run(async (ctx) => { capturedPayload = ctx.payload; return "ok"; });

    expect(capturedPayload).toEqual({ fetched: true });
    expect(fetchCalls.some(c => c.url.includes("/payload"))).toBe(true);
  });

  it("throws when STRAIT_RUN_ID is missing", () => {
    setEnv(makeEnv());
    delete process.env.STRAIT_RUN_ID;
    expect(() => StraitRunner.fromEnv()).toThrow("STRAIT_RUN_ID");
  });

  it("throws when STRAIT_SDK_TOKEN is missing", () => {
    setEnv(makeEnv());
    delete process.env.STRAIT_SDK_TOKEN;
    expect(() => StraitRunner.fromEnv()).toThrow("STRAIT_SDK_TOKEN");
  });

  it("includes Authorization header on all calls", async () => {
    fetchResponses.push(okResponse(), okResponse());
    setEnv(makeEnv());

    const runner = StraitRunner.fromEnv();
    await runner.run(async () => "ok");

    for (const call of fetchCalls) {
      const headers = call.init.headers as Record<string, string>;
      expect(headers?.Authorization).toBe("Bearer tok-secret");
    }
  });

  it("reads STRAIT_SECRET_* env vars into ctx.secrets", async () => {
    fetchResponses.push(okResponse());
    setEnv({ ...makeEnv(), STRAIT_SECRET_DB_URL: "pg://db", STRAIT_SECRET_API_KEY: "sk-123" });

    const runner = StraitRunner.fromEnv();
    let secrets: Record<string, string> = {};
    await runner.run(async (ctx) => { secrets = ctx.secrets; return "ok"; });

    expect(secrets.DB_URL).toBe("pg://db");
    expect(secrets.API_KEY).toBe("sk-123");
  });
});

describe("StraitClient", () => {
  it("complete sends POST", async () => {
    fetchResponses.push(okResponse());
    const client = new StraitClient("https://api.test.com", "tok-123");
    await client.complete("run-1", { done: true });

    expect(fetchCalls[0].url).toBe("https://api.test.com/sdk/v1/runs/run-1/complete");
    expect(fetchCalls[0].init.method).toBe("POST");
  });

  it("fail sends POST with error", async () => {
    fetchResponses.push(okResponse());
    const client = new StraitClient("https://api.test.com", "tok-123");
    await client.fail("run-1", "oops", "RuntimeError");

    const body = JSON.parse(fetchCalls[0].init.body as string);
    expect(body.error).toBe("oops");
    expect(body.error_class).toBe("RuntimeError");
  });

  it("heartbeat sends POST", async () => {
    fetchResponses.push(okResponse());
    const client = new StraitClient("https://api.test.com", "tok-123");
    await client.heartbeat("run-1");

    expect(fetchCalls[0].url).toContain("/heartbeat");
  });

  it("fetchPayload sends GET", async () => {
    fetchResponses.push(okResponse({ payload: { x: 1 } }));
    const client = new StraitClient("https://api.test.com", "tok-123");
    const result = await client.fetchPayload("run-1");

    expect(result).toEqual({ x: 1 });
    expect(fetchCalls[0].init.method).toBe("GET");
  });

  it("log sends POST", async () => {
    fetchResponses.push(okResponse());
    const client = new StraitClient("https://api.test.com", "tok-123");
    await client.log("run-1", "info", "hello");

    const body = JSON.parse(fetchCalls[0].init.body as string);
    expect(body.level).toBe("info");
    expect(body.message).toBe("hello");
  });
});
