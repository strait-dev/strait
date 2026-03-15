import { afterEach, beforeEach, describe, expect, test } from "bun:test";

import { run } from "@stricli/core";

import { app } from "../src/cli";
import { buildContext } from "../src/context";

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

let originalFetch: typeof globalThis.fetch;
let originalEnv: NodeJS.ProcessEnv;

beforeEach(() => {
  originalFetch = globalThis.fetch;
  originalEnv = { ...process.env };
  process.env.STRAIT_SERVER = "https://api.example.com";
  process.env.STRAIT_API_KEY = "strait_live_test";
});

afterEach(() => {
  globalThis.fetch = originalFetch;
  process.env = originalEnv;
});

describe("operational command parity routes", () => {
  test("list and get commands hit expected endpoint families", async () => {
    const calls: string[] = [];

    globalThis.fetch = ((input: RequestInfo | URL, _init?: RequestInit) => {
      calls.push(String(input));

      const url = new URL(String(input));
      const isListPath =
        url.pathname === "/v1/events" ||
        url.pathname === "/v1/jobs" ||
        url.pathname === "/v1/runs" ||
        url.pathname === "/v1/workflows" ||
        url.pathname === "/v1/workflow-runs";
      const body = isListPath
        ? { items: [{ id: "id-1", status: "ok" }] }
        : { id: "id-1", status: "ok" };

      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const matrix = [
      {
        command: "jobs",
        listPath: "/v1/jobs",
        getPath: "/v1/jobs/job-1",
      },
      {
        command: "runs",
        listPath: "/v1/runs",
        getPath: "/v1/runs/run-1",
      },
      {
        command: "workflows",
        listPath: "/v1/workflows",
        getPath: "/v1/workflows/wf-1",
      },
      {
        command: "workflow-runs",
        listPath: "/v1/workflow-runs",
        getPath: "/v1/workflow-runs/wfr-1",
      },
      {
        command: "events",
        listPath: "/v1/events",
        getPath: "/v1/events/event-a",
      },
    ] as const;

    for (const entry of matrix) {
      const listResult = await runCommand([
        entry.command,
        "list",
        "--project",
        "proj-a",
        "--json",
      ]);
      const listPayload = JSON.parse(listResult.stdout) as {
        readonly items: unknown[];
      };
      expect(Array.isArray(listPayload.items)).toBe(true);

      const id = entry.getPath.split("/").at(-1) ?? "id-1";
      const getResult = await runCommand([
        entry.command,
        "get",
        id,
        "--project",
        "proj-a",
        "--json",
      ]);
      const getPayload = JSON.parse(getResult.stdout) as {
        readonly id: string;
      };
      expect(getPayload.id).toBe("id-1");
    }

    for (const entry of matrix) {
      expect(
        calls.some((url) => url.includes(`${entry.listPath}?project_id=proj-a`))
      ).toBe(true);
      expect(calls.some((url) => url.endsWith(entry.getPath))).toBe(true);
    }
  });
});
