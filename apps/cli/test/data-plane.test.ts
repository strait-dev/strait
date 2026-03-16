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
  readonly body?: unknown;
};

const temporaryDirectories: string[] = [];

const createTemporaryDirectory = async (): Promise<string> => {
  const directory = await mkdtemp(join(tmpdir(), "strait-cli-data-plane-"));
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

describe("data-plane command groups", () => {
  test("stats command requests /v1/stats", async () => {
    const calls: HttpCall[] = [];

    globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      calls.push({
        url,
        method: init?.method ?? "GET",
      });

      return Promise.resolve(
        new Response(JSON.stringify({ queued: 1, executing: 2, delayed: 3 }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand(["stats", "--json"]);
    const payload = JSON.parse(result.stdout) as {
      readonly queued: number;
      readonly executing: number;
      readonly delayed: number;
    };

    expect(payload.queued).toBe(1);
    expect(payload.executing).toBe(2);
    expect(payload.delayed).toBe(3);
    expect(calls.some((call) => call.url.pathname === "/v1/stats")).toBe(true);
  });

  test("api-keys list/create/revoke/rotate use expected API shapes", async () => {
    const calls: HttpCall[] = [];

    globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      const method = init?.method ?? "GET";
      const body =
        typeof init?.body === "string"
          ? (JSON.parse(init.body) as unknown)
          : undefined;

      calls.push({ url, method, body });

      if (method === "GET" && url.pathname === "/v1/api-keys") {
        return Promise.resolve(
          new Response(
            JSON.stringify([{ id: "key-1", key_prefix: "strait_" }]),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }
          )
        );
      }

      if (method === "POST" && url.pathname === "/v1/api-keys") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              id: "key-1",
              project_id: (body as { readonly project_id: string }).project_id,
              name: (body as { readonly name: string }).name,
              key_prefix: "strait_",
            }),
            {
              status: 201,
              headers: { "Content-Type": "application/json" },
            }
          )
        );
      }

      if (method === "DELETE" && url.pathname === "/v1/api-keys/key-1") {
        return Promise.resolve(new Response(null, { status: 204 }));
      }

      if (method === "POST" && url.pathname === "/v1/api-keys/key-1/rotate") {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              old_key_id: "key-1",
              new_key_id: "key-2",
            }),
            {
              status: 201,
              headers: { "Content-Type": "application/json" },
            }
          )
        );
      }

      return Promise.resolve(new Response("not found", { status: 404 }));
    }) as unknown as typeof globalThis.fetch;

    const listResult = await runCommand([
      "api-keys",
      "list",
      "--project",
      "proj-x",
      "--json",
    ]);
    const listPayload = JSON.parse(listResult.stdout) as Array<{
      readonly id: string;
    }>;
    expect(listPayload).toHaveLength(1);

    const createResult = await runCommand([
      "api-keys",
      "create",
      "--project",
      "proj-x",
      "--name",
      "CI Key",
      "--scopes",
      "runs.read,jobs.write",
      "--json",
    ]);
    const createPayload = JSON.parse(createResult.stdout) as {
      readonly project_id: string;
      readonly name: string;
    };
    expect(createPayload.project_id).toBe("proj-x");
    expect(createPayload.name).toBe("CI Key");

    const revokeResult = await runCommand([
      "api-keys",
      "revoke",
      "key-1",
      "--json",
    ]);
    const revokePayload = JSON.parse(revokeResult.stdout) as {
      readonly revoked: boolean;
      readonly id: string;
    };
    expect(revokePayload).toEqual({ revoked: true, id: "key-1" });

    const rotateResult = await runCommand([
      "api-keys",
      "rotate",
      "key-1",
      "--gracePeriodMinutes",
      "30",
      "--json",
    ]);
    const rotatePayload = JSON.parse(rotateResult.stdout) as {
      readonly old_key_id: string;
      readonly new_key_id: string;
    };
    expect(rotatePayload.old_key_id).toBe("key-1");
    expect(rotatePayload.new_key_id).toBe("key-2");

    const listCall = calls.find(
      (call) => call.method === "GET" && call.url.pathname === "/v1/api-keys"
    );
    expect(listCall?.url.searchParams.get("project_id")).toBe("proj-x");

    const createCall = calls.find(
      (call) => call.method === "POST" && call.url.pathname === "/v1/api-keys"
    );
    expect(createCall?.body).toEqual(
      expect.objectContaining({
        project_id: "proj-x",
        name: "CI Key",
        scopes: ["runs.read", "jobs.write"],
      })
    );

    const rotateCall = calls.find(
      (call) =>
        call.method === "POST" &&
        call.url.pathname === "/v1/api-keys/key-1/rotate"
    );
    expect(rotateCall?.body).toEqual({ grace_period_minutes: 30 });
  });

  test("secrets commands resolve project from context and handle delete 204", async () => {
    const profileDirectory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(profileDirectory, "profile.json");

    await runCommand([
      "context",
      "create",
      "dev",
      "--server",
      "https://api.example.com",
      "--project",
      "proj-c",
      "--use",
      "--json",
    ]);

    const calls: HttpCall[] = [];

    globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      const method = init?.method ?? "GET";
      const body =
        typeof init?.body === "string"
          ? (JSON.parse(init.body) as unknown)
          : undefined;
      calls.push({ url, method, body });

      if (method === "POST" && url.pathname === "/v1/secrets") {
        return Promise.resolve(
          new Response(
            JSON.stringify({ id: "secret-1", secret_key: "TOKEN" }),
            {
              status: 201,
              headers: { "Content-Type": "application/json" },
            }
          )
        );
      }

      if (method === "GET" && url.pathname === "/v1/secrets") {
        return Promise.resolve(
          new Response(
            JSON.stringify([{ id: "secret-1", secret_key: "TOKEN" }]),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }
          )
        );
      }

      if (method === "DELETE" && url.pathname === "/v1/secrets/secret-1") {
        return Promise.resolve(new Response(null, { status: 204 }));
      }

      return Promise.resolve(new Response("not found", { status: 404 }));
    }) as unknown as typeof globalThis.fetch;

    const createResult = await runCommand([
      "secrets",
      "create",
      "--context",
      "dev",
      "--key",
      "TOKEN",
      "--value",
      "s3cr3t",
      "--environment",
      "production",
      "--json",
    ]);
    const createPayload = JSON.parse(createResult.stdout) as {
      readonly id: string;
    };
    expect(createPayload.id).toBe("secret-1");

    const listResult = await runCommand([
      "secrets",
      "list",
      "--context",
      "dev",
      "--environment",
      "production",
      "--json",
    ]);
    const listPayload = JSON.parse(listResult.stdout) as Array<{
      readonly id: string;
    }>;
    expect(listPayload).toHaveLength(1);

    const deleteResult = await runCommand([
      "secrets",
      "delete",
      "secret-1",
      "--context",
      "dev",
      "--json",
    ]);
    const deletePayload = JSON.parse(deleteResult.stdout) as {
      readonly deleted: boolean;
      readonly id: string;
    };
    expect(deletePayload).toEqual({ deleted: true, id: "secret-1" });

    const createCall = calls.find(
      (call) => call.method === "POST" && call.url.pathname === "/v1/secrets"
    );
    expect(createCall?.body).toEqual(
      expect.objectContaining({
        project_id: "proj-c",
        secret_key: "TOKEN",
        secret_value: "s3cr3t",
        environment: "production",
      })
    );

    const listCall = calls.find(
      (call) => call.method === "GET" && call.url.pathname === "/v1/secrets"
    );
    expect(listCall?.url.searchParams.get("project_id")).toBe("proj-c");
    expect(listCall?.url.searchParams.get("environment")).toBe("production");
  });
});
