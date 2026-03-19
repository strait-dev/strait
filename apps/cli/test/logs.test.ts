import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { run } from "@stricli/core";

import { app } from "../src/cli";
import { buildContext } from "../src/context";

type HttpCall = {
  readonly url: URL;
  readonly method: string;
  readonly headers: Record<string, string>;
  readonly body?: unknown;
};

const temporaryDirectories: string[] = [];

const createTemporaryDirectory = async (): Promise<string> => {
  const directory = await mkdtemp(join(tmpdir(), "strait-cli-logs-"));
  temporaryDirectories.push(directory);
  return directory;
};

const runCommand = async (
  args: readonly string[]
): Promise<{
  readonly stdout: string;
  readonly stderr: string;
}> => {
  let stdout = "";
  let stderr = "";

  const originalStdoutWrite = process.stdout.write.bind(process.stdout);
  const originalStderrWrite = process.stderr.write.bind(process.stderr);

  process.stdout.write = ((chunk: string | Uint8Array) => {
    stdout += chunk.toString();
    return true;
  }) as typeof process.stdout.write;

  process.stderr.write = ((chunk: string | Uint8Array) => {
    stderr += chunk.toString();
    return true;
  }) as typeof process.stderr.write;

  try {
    await run(app, args, buildContext(process));
  } finally {
    process.stdout.write = originalStdoutWrite;
    process.stderr.write = originalStderrWrite;
  }

  return { stdout, stderr };
};

const createMockSSEResponse = (frames: string[]): Response => {
  const encoder = new TextEncoder();
  const stream = new ReadableStream({
    start(controller) {
      for (const frame of frames) {
        controller.enqueue(encoder.encode(frame));
      }
      controller.close();
    },
  });

  return new Response(stream, {
    status: 200,
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
    },
  });
};

let originalFetch: typeof globalThis.fetch;
let originalEnv: NodeJS.ProcessEnv;

beforeEach(() => {
  originalFetch = globalThis.fetch;
  originalEnv = { ...process.env };
  process.env.STRAIT_SERVER = "https://api.example.com";
  process.env.STRAIT_API_KEY = "strait_live_test";
});

afterEach(async () => {
  globalThis.fetch = originalFetch;
  process.env = originalEnv;

  await Promise.all(
    temporaryDirectories
      .splice(0)
      .map((directory) => rm(directory, { recursive: true, force: true }))
  );
});

describe("logs command", () => {
  test("--run flag fetches run details and displays log output", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    globalThis.fetch = ((input: RequestInfo | URL, _init?: RequestInit) => {
      const url = new URL(String(input));

      if (url.pathname === "/v1/runs/run-123") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              id: "run-123",
              status: "completed",
              output: "processing complete",
              events: [
                {
                  timestamp: "2026-03-19T10:00:00Z",
                  level: "info",
                  message: "started",
                },
                {
                  timestamp: "2026-03-19T10:01:00Z",
                  level: "info",
                  message: "processing complete",
                },
              ],
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }
          )
        );
      }

      return Promise.resolve(
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand(["logs", "--run", "run-123", "--json"]);
    const lines = result.stdout.trim().split("\n").filter(Boolean);

    expect(lines.length).toBe(2);
    for (const line of lines) {
      const parsed = JSON.parse(line) as { message: string };
      expect(parsed).toHaveProperty("message");
    }
  });

  test("--job flag resolves slug to latest run ID", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    const calls: HttpCall[] = [];

    globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      const method = init?.method ?? "GET";
      const headers: Record<string, string> = {};
      if (init?.headers) {
        for (const [key, value] of Object.entries(
          init.headers as Record<string, string>
        )) {
          headers[key] = value;
        }
      }
      calls.push({ url, method, headers });

      if (url.pathname === "/v1/jobs") {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              { id: "job-1", slug: "process-payment", name: "Process Payment" },
            ]),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }
          )
        );
      }

      if (url.pathname === "/v1/runs") {
        return Promise.resolve(
          new Response(
            JSON.stringify([
              {
                id: "run-456",
                job_id: "job-1",
                status: "completed",
              },
            ]),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }
          )
        );
      }

      if (url.pathname === "/v1/runs/run-456") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              id: "run-456",
              output: "done",
              events: [
                {
                  timestamp: "2026-03-19T10:00:00Z",
                  level: "info",
                  message: "done",
                },
              ],
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }
          )
        );
      }

      return Promise.resolve(
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand([
      "logs",
      "--job",
      "process-payment",
      "--json",
    ]);

    expect(calls.some((c) => c.url.pathname === "/v1/jobs")).toBe(true);
    expect(calls.some((c) => c.url.pathname === "/v1/runs")).toBe(true);
    expect(calls.some((c) => c.url.pathname === "/v1/runs/run-456")).toBe(true);
    expect(result.stdout.trim().length).toBeGreaterThan(0);
  });

  test("--job flag with unknown slug produces error", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    globalThis.fetch = ((input: RequestInfo | URL, _init?: RequestInit) => {
      const url = new URL(String(input));

      if (url.pathname === "/v1/jobs") {
        return Promise.resolve(
          new Response(JSON.stringify([{ id: "job-1", slug: "other-job" }]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          })
        );
      }

      return Promise.resolve(
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand(["logs", "--job", "nonexistent", "--json"]);

    const hasError =
      result.stderr.toLowerCase().includes("not found") ||
      result.stderr.toLowerCase().includes("error") ||
      result.stdout === "";
    expect(hasError).toBe(true);
  });

  test("--level error filters out info and warn events", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    globalThis.fetch = ((input: RequestInfo | URL, _init?: RequestInit) => {
      const url = new URL(String(input));

      if (url.pathname === "/v1/runs/run-789") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              id: "run-789",
              events: [
                {
                  level: "info",
                  message: "started",
                  timestamp: "2026-03-19T10:00:00Z",
                },
                {
                  level: "warn",
                  message: "slow",
                  timestamp: "2026-03-19T10:01:00Z",
                },
                {
                  level: "error",
                  message: "failed",
                  timestamp: "2026-03-19T10:02:00Z",
                },
              ],
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }
          )
        );
      }

      return Promise.resolve(
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand([
      "logs",
      "--run",
      "run-789",
      "--level",
      "error",
      "--json",
    ]);
    const lines = result.stdout.trim().split("\n").filter(Boolean);

    expect(lines).toHaveLength(1);
    const parsed = JSON.parse(lines[0]) as { message: string };
    expect(parsed.message).toBe("failed");
  });

  test("--level warn filters out info but keeps error and warn", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    globalThis.fetch = ((input: RequestInfo | URL, _init?: RequestInit) => {
      const url = new URL(String(input));

      if (url.pathname === "/v1/runs/run-789") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              id: "run-789",
              events: [
                {
                  level: "info",
                  message: "started",
                  timestamp: "2026-03-19T10:00:00Z",
                },
                {
                  level: "warn",
                  message: "slow",
                  timestamp: "2026-03-19T10:01:00Z",
                },
                {
                  level: "error",
                  message: "failed",
                  timestamp: "2026-03-19T10:02:00Z",
                },
              ],
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }
          )
        );
      }

      return Promise.resolve(
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand([
      "logs",
      "--run",
      "run-789",
      "--level",
      "warn",
      "--json",
    ]);
    const lines = result.stdout.trim().split("\n").filter(Boolean);

    expect(lines).toHaveLength(2);
    const messages = lines.map(
      (l) => (JSON.parse(l) as { message: string }).message
    );
    expect(messages).toContain("slow");
    expect(messages).toContain("failed");
    expect(messages).not.toContain("started");
  });

  test("--json outputs NDJSON format (one JSON object per line)", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    globalThis.fetch = ((input: RequestInfo | URL, _init?: RequestInit) => {
      const url = new URL(String(input));

      if (url.pathname === "/v1/runs/run-789") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              id: "run-789",
              events: [
                {
                  level: "info",
                  message: "first",
                  timestamp: "2026-03-19T10:00:00Z",
                },
                {
                  level: "info",
                  message: "second",
                  timestamp: "2026-03-19T10:01:00Z",
                },
              ],
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }
          )
        );
      }

      return Promise.resolve(
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand(["logs", "--run", "run-789", "--json"]);
    const lines = result.stdout.trim().split("\n").filter(Boolean);

    expect(lines).toHaveLength(2);
    for (const line of lines) {
      const parsed = JSON.parse(line) as Record<string, unknown>;
      expect(parsed).toHaveProperty("timestamp");
      expect(parsed).toHaveProperty("level");
      expect(parsed).toHaveProperty("message");
    }
  });

  test("plain text output includes formatted timestamps and level labels", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    globalThis.fetch = ((input: RequestInfo | URL, _init?: RequestInit) => {
      const url = new URL(String(input));

      if (url.pathname === "/v1/runs/run-789") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              id: "run-789",
              events: [
                {
                  level: "info",
                  message: "hello",
                  timestamp: "2026-03-19T10:00:00Z",
                },
              ],
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }
          )
        );
      }

      return Promise.resolve(
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand(["logs", "--run", "run-789"]);

    expect(result.stdout).toContain("[2026-03-19T10:00:00Z]");
    expect(result.stdout).toContain("INFO");
    expect(result.stdout).toContain("hello");
  });

  test("--follow --run connects to SSE endpoint", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    globalThis.fetch = ((input: RequestInfo | URL, _init?: RequestInit) => {
      const url = new URL(String(input));

      if (url.pathname.includes("/stream")) {
        return Promise.resolve(
          createMockSSEResponse([
            'data: {"level":"info","message":"hello","timestamp":"2026-03-19T10:00:00Z"}\n\n',
            'data: {"level":"error","message":"boom","timestamp":"2026-03-19T10:01:00Z"}\n\n',
          ])
        );
      }

      return Promise.resolve(
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand([
      "logs",
      "--follow",
      "--run",
      "run-sse",
      "--json",
    ]);
    const lines = result.stdout.trim().split("\n").filter(Boolean);

    expect(lines).toHaveLength(2);
    const first = JSON.parse(lines[0]) as { message: string };
    const second = JSON.parse(lines[1]) as { message: string };
    expect(first.message).toBe("hello");
    expect(second.message).toBe("boom");
  });

  test("--follow skips SSE keepalive comments", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    globalThis.fetch = ((input: RequestInfo | URL, _init?: RequestInit) => {
      const url = new URL(String(input));

      if (url.pathname.includes("/stream")) {
        return Promise.resolve(
          createMockSSEResponse([
            ": keepalive\n\n",
            'data: {"level":"info","message":"real","timestamp":"2026-03-19T10:00:00Z"}\n\n',
          ])
        );
      }

      return Promise.resolve(
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand([
      "logs",
      "--follow",
      "--run",
      "run-sse",
      "--json",
    ]);
    const lines = result.stdout.trim().split("\n").filter(Boolean);

    expect(lines).toHaveLength(1);
    const parsed = JSON.parse(lines[0]) as { message: string };
    expect(parsed.message).toBe("real");
  });

  test("--follow --level filters SSE events", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    globalThis.fetch = ((input: RequestInfo | URL, _init?: RequestInit) => {
      const url = new URL(String(input));

      if (url.pathname.includes("/stream")) {
        return Promise.resolve(
          createMockSSEResponse([
            'data: {"level":"info","message":"hello","timestamp":"2026-03-19T10:00:00Z"}\n\n',
            'data: {"level":"error","message":"boom","timestamp":"2026-03-19T10:01:00Z"}\n\n',
          ])
        );
      }

      return Promise.resolve(
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand([
      "logs",
      "--follow",
      "--run",
      "run-sse",
      "--level",
      "error",
      "--json",
    ]);
    const lines = result.stdout.trim().split("\n").filter(Boolean);

    expect(lines).toHaveLength(1);
    const parsed = JSON.parse(lines[0]) as { message: string };
    expect(parsed.message).toBe("boom");
  });

  test("--follow --job resolves to latest run then streams", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    const calls: HttpCall[] = [];

    globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      const method = init?.method ?? "GET";
      calls.push({ url, method, headers: {} });

      if (url.pathname === "/v1/jobs") {
        return Promise.resolve(
          new Response(JSON.stringify([{ id: "job-1", slug: "my-job" }]), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          })
        );
      }

      if (url.pathname === "/v1/runs") {
        return Promise.resolve(
          new Response(
            JSON.stringify([{ id: "run-stream", job_id: "job-1" }]),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }
          )
        );
      }

      if (url.pathname.includes("/stream")) {
        return Promise.resolve(
          createMockSSEResponse([
            'data: {"level":"info","message":"streamed","timestamp":"2026-03-19T10:00:00Z"}\n\n',
          ])
        );
      }

      return Promise.resolve(
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand([
      "logs",
      "--follow",
      "--job",
      "my-job",
      "--json",
    ]);

    expect(calls.some((c) => c.url.pathname === "/v1/jobs")).toBe(true);
    expect(calls.some((c) => c.url.pathname === "/v1/runs")).toBe(true);
    expect(calls.some((c) => c.url.pathname.includes("run-stream"))).toBe(true);

    const lines = result.stdout.trim().split("\n").filter(Boolean);
    expect(lines).toHaveLength(1);
  });

  test("no --run or --job produces clear error message", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    globalThis.fetch = ((_input: RequestInfo | URL) => {
      return Promise.resolve(
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand(["logs", "--json"]);

    const hasError =
      result.stderr.toLowerCase().includes("required") ||
      result.stderr.toLowerCase().includes("error") ||
      result.stdout === "";
    expect(hasError).toBe(true);
  });

  test("--follow sends correct headers to SSE endpoint", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");
    process.env.STRAIT_API_KEY = "test_api_key";

    const calls: HttpCall[] = [];

    globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      const headers: Record<string, string> = {};
      if (init?.headers) {
        for (const [key, value] of Object.entries(
          init.headers as Record<string, string>
        )) {
          headers[key] = value;
        }
      }
      calls.push({ url, method: init?.method ?? "GET", headers });

      if (url.pathname.includes("/stream")) {
        return Promise.resolve(
          createMockSSEResponse([
            'data: {"level":"info","message":"test","timestamp":"2026-03-19T10:00:00Z"}\n\n',
          ])
        );
      }

      return Promise.resolve(
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    await runCommand(["logs", "--follow", "--run", "run-headers", "--json"]);

    const sseCall = calls.find((c) => c.url.pathname.includes("/stream"));
    expect(sseCall).toBeDefined();
    expect(sseCall?.headers.Accept).toBe("text/event-stream");
    expect(sseCall?.headers.Authorization).toBe("Bearer test_api_key");
  });

  test("API calls use correct authentication and project context", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");
    process.env.STRAIT_API_KEY = "test_api_key";

    const calls: HttpCall[] = [];

    globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      const headers: Record<string, string> = {};
      if (init?.headers) {
        for (const [key, value] of Object.entries(
          init.headers as Record<string, string>
        )) {
          headers[key] = value;
        }
      }
      calls.push({ url, method: init?.method ?? "GET", headers });

      if (url.pathname === "/v1/runs/run-auth") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              id: "run-auth",
              events: [
                {
                  level: "info",
                  message: "test",
                  timestamp: "2026-03-19T10:00:00Z",
                },
              ],
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }
          )
        );
      }

      return Promise.resolve(
        new Response(JSON.stringify({}), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    await runCommand(["logs", "--run", "run-auth", "--json"]);

    const authCalls = calls.filter((c) => c.headers.Authorization);
    expect(authCalls.length).toBeGreaterThan(0);
    expect(authCalls[0].headers.Authorization).toContain("Bearer test_api_key");
  });
});
