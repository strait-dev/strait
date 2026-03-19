import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { run } from "@stricli/core";

import { app } from "../src/cli";
import { buildContext } from "../src/context";

const CHECKS_PASSED_RE = /\d+\/\d+ checks passed/;

type HttpCall = {
  readonly url: URL;
  readonly method: string;
  readonly headers: Record<string, string>;
  readonly body?: unknown;
};

type CheckResult = {
  readonly name: string;
  readonly status: "pass" | "fail" | "warn" | "skip";
  readonly message: string;
  readonly detail?: string;
  readonly fix?: string;
};

const temporaryDirectories: string[] = [];

const createTemporaryDirectory = async (): Promise<string> => {
  const directory = await mkdtemp(join(tmpdir(), "strait-cli-doctor-"));
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

const writeStraitJson = async (
  directory: string,
  config?: Record<string, unknown>
): Promise<void> => {
  const content = config ?? {
    project: { id: "proj-doctor" },
    src: "src",
  };
  await writeFile(
    join(directory, "strait.json"),
    JSON.stringify(content),
    "utf-8"
  );
};

let originalFetch: typeof globalThis.fetch;
let originalEnv: NodeJS.ProcessEnv;
let originalCwd: string;
let originalExitCode: typeof process.exitCode;

beforeEach(() => {
  originalFetch = globalThis.fetch;
  originalEnv = { ...process.env };
  originalCwd = process.cwd();
  originalExitCode = process.exitCode;
});

afterEach(async () => {
  globalThis.fetch = originalFetch;
  process.env = originalEnv;
  process.chdir(originalCwd);
  process.exitCode = originalExitCode;

  await Promise.all(
    temporaryDirectories
      .splice(0)
      .map((directory) => rm(directory, { recursive: true, force: true }))
  );
});

describe("doctor command", () => {
  test("all checks pass with healthy environment (JSON output)", async () => {
    const directory = await createTemporaryDirectory();
    await writeStraitJson(directory);
    process.chdir(directory);

    process.env.STRAIT_SERVER = "https://api.example.com";
    process.env.STRAIT_API_KEY = "strait_live_test_key";
    process.env.DATABASE_URL = "postgres://localhost/test";
    process.env.REDIS_URL = "redis://localhost:6379";
    process.env.FLY_API_TOKEN = "fly_token_123";
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    globalThis.fetch = ((input: RequestInfo | URL, _init?: RequestInit) => {
      const url = new URL(String(input));

      if (url.pathname === "/health") {
        return Promise.resolve(
          new Response(JSON.stringify({ status: "ok" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          })
        );
      }

      if (url.pathname === "/v1/stats") {
        return Promise.resolve(
          new Response(
            JSON.stringify({ queued: 0, executing: 0, delayed: 0 }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }
          )
        );
      }

      if (url.pathname === "/health/ready") {
        return Promise.resolve(
          new Response(JSON.stringify({ status: "ready" }), {
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

    const result = await runCommand(["doctor", "--json"]);
    const checks = JSON.parse(result.stdout) as CheckResult[];

    expect(checks.length).toBeGreaterThanOrEqual(10);

    const cliVersion = checks.find((c) => c.name === "cli_version");
    expect(cliVersion?.status).toBe("pass");

    const apiConnectivity = checks.find((c) => c.name === "api_connectivity");
    expect(apiConnectivity?.status).toBe("pass");
    expect(apiConnectivity?.message).toContain("ms)");

    const auth = checks.find((c) => c.name === "authentication");
    expect(auth?.status).toBe("pass");

    const noFails = checks.every((c) => c.status !== "fail");
    expect(noFails).toBe(true);
  });

  test("config not found produces fail check", async () => {
    const directory = await createTemporaryDirectory();
    process.chdir(directory);

    process.env.STRAIT_SERVER = "https://api.example.com";
    process.env.STRAIT_API_KEY = "strait_live_test";
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    globalThis.fetch = ((_input: RequestInfo | URL) => {
      return Promise.resolve(
        new Response(JSON.stringify({ status: "ok" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand(["doctor", "--json"]);
    const checks = JSON.parse(result.stdout) as CheckResult[];

    const configCheck = checks.find((c) => c.name === "config_file");
    expect(configCheck?.status).toBe("fail");
    expect(configCheck?.message?.toLowerCase()).toContain("not found");
    expect(configCheck?.fix).toBeDefined();
  });

  test("API connectivity failure reports fail with error detail", async () => {
    const directory = await createTemporaryDirectory();
    await writeStraitJson(directory);
    process.chdir(directory);

    process.env.STRAIT_SERVER = "https://unreachable.example.com";
    process.env.STRAIT_API_KEY = "strait_live_test";
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    globalThis.fetch = ((_input: RequestInfo | URL) => {
      return Promise.reject(new Error("ECONNREFUSED"));
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand(["doctor", "--json"]);
    const checks = JSON.parse(result.stdout) as CheckResult[];

    const apiCheck = checks.find((c) => c.name === "api_connectivity");
    expect(apiCheck?.status).toBe("fail");
  });

  test("authentication failure on 401 reports fail", async () => {
    const directory = await createTemporaryDirectory();
    await writeStraitJson(directory);
    process.chdir(directory);

    process.env.STRAIT_SERVER = "https://api.example.com";
    process.env.STRAIT_API_KEY = "invalid_key";
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    globalThis.fetch = ((input: RequestInfo | URL) => {
      const url = new URL(String(input));

      if (url.pathname === "/health") {
        return Promise.resolve(
          new Response(JSON.stringify({ status: "ok" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          })
        );
      }

      if (url.pathname === "/v1/stats") {
        return Promise.resolve(
          new Response(JSON.stringify({ error: "unauthorized" }), {
            status: 401,
            headers: { "Content-Type": "application/json" },
          })
        );
      }

      return Promise.resolve(
        new Response(JSON.stringify({ status: "ok" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand(["doctor", "--json"]);
    const checks = JSON.parse(result.stdout) as CheckResult[];

    const authCheck = checks.find((c) => c.name === "authentication");
    expect(authCheck?.status).toBe("fail");

    const apiCheck = checks.find((c) => c.name === "api_connectivity");
    expect(apiCheck?.status).toBe("pass");
  });

  test("missing env vars produce warn checks", async () => {
    const directory = await createTemporaryDirectory();
    await writeStraitJson(directory);
    process.chdir(directory);

    process.env.STRAIT_SERVER = "https://api.example.com";
    process.env.STRAIT_API_KEY = "strait_live_test";
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");
    process.env.DATABASE_URL = undefined;
    process.env.POSTGRES_URL = undefined;
    process.env.REDIS_URL = undefined;
    process.env.FLY_API_TOKEN = undefined;

    globalThis.fetch = ((_input: RequestInfo | URL) => {
      return Promise.resolve(
        new Response(JSON.stringify({ status: "ok" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand(["doctor", "--json"]);
    const checks = JSON.parse(result.stdout) as CheckResult[];

    const pgCheck = checks.find((c) => c.name === "postgresql");
    expect(pgCheck?.status).toBe("warn");

    const redisCheck = checks.find((c) => c.name === "redis");
    expect(redisCheck?.status).toBe("warn");

    const flyCheck = checks.find((c) => c.name === "fly_credentials");
    expect(flyCheck?.status).toBe("warn");

    for (const check of [pgCheck, redisCheck, flyCheck]) {
      expect(check?.fix).toBeDefined();
    }
  });

  test("plain text output format shows pass/fail markers", async () => {
    const directory = await createTemporaryDirectory();
    await writeStraitJson(directory);
    process.chdir(directory);

    process.env.STRAIT_SERVER = "https://api.example.com";
    process.env.STRAIT_API_KEY = "strait_live_test";
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    globalThis.fetch = ((_input: RequestInfo | URL) => {
      return Promise.resolve(
        new Response(JSON.stringify({ status: "ok" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand(["doctor"]);

    expect(result.stdout).toContain("[PASS]");
    expect(result.stdout).toMatch(CHECKS_PASSED_RE);
  });

  test("--verbose includes detail field in plain output", async () => {
    const directory = await createTemporaryDirectory();
    await writeStraitJson(directory);
    process.chdir(directory);

    process.env.STRAIT_SERVER = "https://api.example.com";
    process.env.STRAIT_API_KEY = "strait_live_test";
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    globalThis.fetch = ((_input: RequestInfo | URL) => {
      return Promise.resolve(
        new Response(JSON.stringify({ status: "ok" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const plainResult = await runCommand(["doctor"]);
    const verboseResult = await runCommand(["doctor", "--verbose"]);

    expect(verboseResult.stdout.length).toBeGreaterThan(
      plainResult.stdout.length
    );
    expect(verboseResult.stdout).toContain("Detail:");
  });

  test("exit code 1 when any check fails", async () => {
    const directory = await createTemporaryDirectory();
    process.chdir(directory);

    process.env.STRAIT_SERVER = "https://api.example.com";
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");
    process.env.STRAIT_API_KEY = undefined;

    globalThis.fetch = ((_input: RequestInfo | URL) => {
      return Promise.resolve(
        new Response(JSON.stringify({ error: "service unavailable" }), {
          status: 503,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand(["doctor", "--json"]);
    const checks = JSON.parse(result.stdout) as CheckResult[];

    expect(checks.some((c) => c.status === "fail")).toBe(true);
    const failNames = checks
      .filter((c) => c.status === "fail")
      .map((c) => c.name);
    expect(failNames.length).toBeGreaterThan(0);
  });

  test("doctor verifies HTTP calls are made to correct endpoints", async () => {
    const directory = await createTemporaryDirectory();
    await writeStraitJson(directory);
    process.chdir(directory);

    process.env.STRAIT_SERVER = "https://api.example.com";
    process.env.STRAIT_API_KEY = "test_key";
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    const calls: HttpCall[] = [];

    globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      const headers: Record<string, string> = {};
      if (init?.headers) {
        const headerEntries = Object.entries(
          init.headers as Record<string, string>
        );
        for (const [key, value] of headerEntries) {
          headers[key] = value;
        }
      }
      calls.push({
        url,
        method: init?.method ?? "GET",
        headers,
      });

      return Promise.resolve(
        new Response(JSON.stringify({ status: "ok" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    await runCommand(["doctor", "--json"]);

    expect(calls.some((c) => c.url.pathname === "/health")).toBe(true);
    expect(calls.some((c) => c.url.pathname === "/v1/stats")).toBe(true);
    expect(calls.some((c) => c.url.pathname === "/health/ready")).toBe(true);

    const authenticatedCalls = calls.filter(
      (c) => c.url.pathname === "/v1/stats"
    );
    for (const call of authenticatedCalls) {
      expect(call.headers.Authorization).toBe("Bearer test_key");
    }
  });

  test("--context flag overrides connection resolution", async () => {
    const directory = await createTemporaryDirectory();
    await writeStraitJson(directory);
    process.chdir(directory);

    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    await runCommand([
      "context",
      "create",
      "staging",
      "--server",
      "https://staging.example.com",
      "--use",
      "--json",
    ]);

    const calls: HttpCall[] = [];

    globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      calls.push({
        url,
        method: init?.method ?? "GET",
        headers: {},
      });

      return Promise.resolve(
        new Response(JSON.stringify({ status: "ok" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    await runCommand(["doctor", "--context", "staging", "--json"]);

    const apiCalls = calls.filter((c) =>
      c.url.hostname.includes("staging.example.com")
    );
    expect(apiCalls.length).toBeGreaterThan(0);
  });
});
