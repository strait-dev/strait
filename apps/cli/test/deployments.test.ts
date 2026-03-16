import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
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
  const directory = await mkdtemp(join(tmpdir(), "strait-cli-deployments-"));
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

const writeProjectConfig = async (directory: string): Promise<string> => {
  const configPath = join(directory, "strait.config.ts");
  await writeFile(
    configPath,
    `export default {
  project: { id: "proj-deploy", name: "Deploy Project" },
  runtime: { kind: "node" },
  build: { outDir: "out" },
  deploy: { defaultEnvironment: "staging" },
  jobs: [{ name: "Job A", slug: "job-a", endpointUrl: "https://jobs.example.com/a" }],
  workflows: [{ name: "Workflow A", slug: "wf-a", steps: [{ name: "step-a", job: "job-a" }] }]
};
`
  );

  return configPath;
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

describe("deployment lifecycle commands", () => {
  test("deploy --dryRun emits create/finalize payload plan", async () => {
    const directory = await createTemporaryDirectory();
    const configPath = await writeProjectConfig(directory);

    globalThis.fetch = ((_input: RequestInfo | URL, _init?: RequestInit) => {
      throw new Error("deploy --dryRun should not call fetch");
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand([
      "deploy",
      "--config",
      configPath,
      "--dryRun",
      "--json",
    ]);

    const payload = JSON.parse(result.stdout) as {
      readonly action: string;
      readonly create: {
        readonly project_id: string;
        readonly environment: string;
        readonly checksum: string;
      };
      readonly finalize: {
        readonly project_id: string;
        readonly environment: string;
      };
    };

    expect(payload.action).toBe("deploy");
    expect(payload.create.project_id).toBe("proj-deploy");
    expect(payload.create.environment).toBe("staging");
    expect(payload.create.checksum.length).toBe(64);
    expect(payload.finalize).toEqual({
      project_id: "proj-deploy",
      environment: "staging",
    });
  });

  test("deploy writes manifest and finalizes deployment version", async () => {
    const directory = await createTemporaryDirectory();
    const configPath = await writeProjectConfig(directory);
    const calls: HttpCall[] = [];

    globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      const method = init?.method ?? "GET";
      const body =
        typeof init?.body === "string"
          ? (JSON.parse(init.body) as unknown)
          : undefined;

      calls.push({ url, method, body });

      if (method === "POST" && url.pathname === "/v1/deployments") {
        return Promise.resolve(
          new Response(JSON.stringify({ id: "dep-1", status: "draft" }), {
            status: 201,
            headers: { "Content-Type": "application/json" },
          })
        );
      }

      if (
        method === "POST" &&
        url.pathname === "/v1/deployments/dep-1/finalize"
      ) {
        return Promise.resolve(
          new Response(JSON.stringify({ id: "dep-1", status: "finalized" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          })
        );
      }

      return Promise.resolve(new Response("not found", { status: 404 }));
    }) as unknown as typeof globalThis.fetch;

    const result = await runCommand([
      "deploy",
      "--config",
      configPath,
      "--json",
    ]);
    const payload = JSON.parse(result.stdout) as {
      readonly manifest_path: string;
      readonly deployment: { readonly id: string; readonly status: string };
    };

    expect(payload.deployment.id).toBe("dep-1");
    expect(payload.deployment.status).toBe("finalized");

    const manifestText = await readFile(payload.manifest_path, "utf8");
    const manifest = JSON.parse(manifestText) as {
      readonly project: { readonly id: string };
      readonly runtime: string;
    };
    expect(manifest.project.id).toBe("proj-deploy");
    expect(manifest.runtime).toBe("node");

    const createCall = calls.find(
      (call) =>
        call.method === "POST" && call.url.pathname === "/v1/deployments"
    );
    expect(createCall?.body).toEqual(
      expect.objectContaining({
        project_id: "proj-deploy",
        environment: "staging",
        runtime: "node",
      })
    );

    const finalizeCall = calls.find(
      (call) =>
        call.method === "POST" &&
        call.url.pathname === "/v1/deployments/dep-1/finalize"
    );
    expect(finalizeCall?.body).toEqual({
      project_id: "proj-deploy",
      environment: "staging",
    });
  });

  test("promote and rollback hit deployment mutation endpoints", async () => {
    const directory = await createTemporaryDirectory();
    const configPath = await writeProjectConfig(directory);
    const calls: HttpCall[] = [];

    globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input));
      const method = init?.method ?? "GET";
      const body =
        typeof init?.body === "string"
          ? (JSON.parse(init.body) as unknown)
          : undefined;

      calls.push({ url, method, body });

      if (
        method === "POST" &&
        url.pathname === "/v1/deployments/dep-1/promote"
      ) {
        return Promise.resolve(
          new Response(JSON.stringify({ id: "dep-1", status: "promoted" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          })
        );
      }

      if (
        method === "POST" &&
        url.pathname === "/v1/deployments/dep-0/rollback"
      ) {
        return Promise.resolve(
          new Response(JSON.stringify({ id: "dep-0", status: "promoted" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          })
        );
      }

      return Promise.resolve(new Response("not found", { status: 404 }));
    }) as unknown as typeof globalThis.fetch;

    const promoteResult = await runCommand([
      "promote",
      "dep-1",
      "--config",
      configPath,
      "--env",
      "production",
      "--json",
    ]);
    const promotePayload = JSON.parse(promoteResult.stdout) as {
      readonly status: string;
    };
    expect(promotePayload.status).toBe("promoted");

    const rollbackResult = await runCommand([
      "rollback",
      "--to",
      "dep-0",
      "--config",
      configPath,
      "--env",
      "production",
      "--json",
    ]);
    const rollbackPayload = JSON.parse(rollbackResult.stdout) as {
      readonly id: string;
      readonly status: string;
    };
    expect(rollbackPayload.id).toBe("dep-0");
    expect(rollbackPayload.status).toBe("promoted");

    const promoteCall = calls.find(
      (call) =>
        call.method === "POST" &&
        call.url.pathname === "/v1/deployments/dep-1/promote"
    );
    expect(promoteCall?.body).toEqual({
      project_id: "proj-deploy",
      environment: "production",
    });

    const rollbackCall = calls.find(
      (call) =>
        call.method === "POST" &&
        call.url.pathname === "/v1/deployments/dep-0/rollback"
    );
    expect(rollbackCall?.body).toEqual({
      project_id: "proj-deploy",
      environment: "production",
    });
  });
});
