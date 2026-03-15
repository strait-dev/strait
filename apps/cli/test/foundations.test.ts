import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { run } from "@stricli/core";

import { app } from "../src/cli";
import { buildContext } from "../src/context";

const createdTempDirs: string[] = [];

const createTempDir = async (): Promise<string> => {
  const directory = await mkdtemp(join(tmpdir(), "strait-cli-test-"));
  createdTempDirs.push(directory);
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

let originalEnv: NodeJS.ProcessEnv;
let originalFetch: typeof globalThis.fetch;

beforeEach(() => {
  originalEnv = { ...process.env };
  originalFetch = globalThis.fetch;
});

afterEach(async () => {
  process.env = originalEnv;
  globalThis.fetch = originalFetch;

  await Promise.all(
    createdTempDirs
      .splice(0)
      .map((directory) => rm(directory, { recursive: true, force: true }))
  );
});

describe("foundational command set", () => {
  test("context create/use/current/list JSON flow", async () => {
    const directory = await createTempDir();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    const createResult = await runCommand([
      "context",
      "create",
      "dev",
      "--server",
      "https://api.example.com",
      "--project",
      "proj-a",
      "--use",
      "--json",
    ]);

    const createdPayload = JSON.parse(createResult.stdout) as {
      readonly name: string;
      readonly serverUrl: string;
      readonly projectId?: string;
    };

    expect(createdPayload.name).toBe("dev");
    expect(createdPayload.serverUrl).toBe("https://api.example.com");
    expect(createdPayload.projectId).toBe("proj-a");

    const currentResult = await runCommand(["context", "current", "--json"]);
    const currentPayload = JSON.parse(currentResult.stdout) as {
      readonly name: string;
      readonly serverUrl: string;
      readonly projectId?: string;
      readonly hasApiKey: boolean;
    };

    expect(currentPayload.name).toBe("dev");
    expect(currentPayload.serverUrl).toBe("https://api.example.com");
    expect(currentPayload.hasApiKey).toBe(false);

    const listResult = await runCommand(["context", "list", "--json"]);
    const listPayload = JSON.parse(listResult.stdout) as Array<{
      readonly name: string;
      readonly active: boolean;
    }>;

    expect(listPayload).toHaveLength(1);
    expect(listPayload[0]).toEqual(
      expect.objectContaining({
        name: "dev",
        active: true,
      })
    );
  });

  test("auth login/whoami/logout flow", async () => {
    const directory = await createTempDir();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    await runCommand([
      "context",
      "create",
      "dev",
      "--server",
      "https://api.example.com",
      "--use",
      "--json",
    ]);

    await runCommand([
      "auth",
      "login",
      "--context",
      "dev",
      "--apiKey",
      "strait_live_123",
      "--json",
    ]);

    const whoamiResult = await runCommand(["auth", "whoami", "--json"]);
    const whoamiPayload = JSON.parse(whoamiResult.stdout) as {
      readonly contextName?: string;
      readonly hasApiKey: boolean;
    };

    expect(whoamiPayload.contextName).toBe("dev");
    expect(whoamiPayload.hasApiKey).toBe(true);

    await runCommand(["auth", "logout", "--json"]);

    const postLogoutWhoamiResult = await runCommand([
      "auth",
      "whoami",
      "--json",
    ]);
    const postLogoutWhoamiPayload = JSON.parse(
      postLogoutWhoamiResult.stdout
    ) as {
      readonly hasApiKey: boolean;
    };

    expect(postLogoutWhoamiPayload.hasApiKey).toBe(false);
  });

  test("health command hits server endpoint", async () => {
    const directory = await createTempDir();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");
    process.env.STRAIT_SERVER = "https://health.example.com";

    globalThis.fetch = ((_input: RequestInfo | URL, _init?: RequestInit) => {
      return Promise.resolve(
        new Response(JSON.stringify({ status: "ok" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand(["health", "--json"]);

    const payload = JSON.parse(result.stdout) as {
      readonly status: string;
      readonly serverUrl: string;
    };

    expect(payload.status).toBe("ok");
    expect(payload.serverUrl).toBe("https://health.example.com");
  });
});
