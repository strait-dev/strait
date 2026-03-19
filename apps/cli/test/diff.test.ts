import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { run } from "@stricli/core";

import { app } from "../src/cli";
import type { DiffResult } from "../src/commands/diff-helpers";
import { buildContext } from "../src/context";

const DIFF_SUMMARY_RE = /\d+ new, \d+ modified, \d+ removed/;

type HttpCall = {
  readonly url: URL;
  readonly method: string;
  readonly body?: unknown;
};

const temporaryDirectories: string[] = [];

const createTemporaryDirectory = async (): Promise<string> => {
  const directory = await mkdtemp(join(tmpdir(), "strait-cli-diff-"));
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

const writeProjectConfig = async (
  directory: string,
  config: {
    projectId?: string;
    jobs?: readonly Record<string, unknown>[];
    workflows?: readonly Record<string, unknown>[];
  }
): Promise<string> => {
  const configPath = join(directory, "strait.config.mjs");
  const jobs = (config.jobs ?? []).map((job) => ({
    slug: job.slug,
    name: job.name ?? job.slug,
    ...job,
  }));
  const workflows = (config.workflows ?? []).map((wf) => ({
    slug: wf.slug,
    name: wf.name ?? wf.slug,
    steps: wf.steps ?? [],
    ...wf,
  }));
  const content = `export default ${JSON.stringify(
    {
      project: { id: config.projectId ?? "proj-diff" },
      runtime: { kind: "node" },
      build: { outDir: ".strait" },
      jobs,
      workflows,
    },
    null,
    2
  )};\n`;
  await writeFile(configPath, content, "utf-8");
  return configPath;
};

const mockRemoteState = (
  jobs: readonly Record<string, unknown>[],
  workflows: readonly Record<string, unknown>[],
  calls?: HttpCall[]
): void => {
  globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (calls) {
      calls.push({ url, method });
    }

    if (url.pathname === "/v1/jobs") {
      return Promise.resolve(
        new Response(JSON.stringify(jobs), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
      );
    }

    if (url.pathname === "/v1/workflows") {
      return Promise.resolve(
        new Response(JSON.stringify(workflows), {
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

describe("diff command", () => {
  test("detects new jobs and workflows (additions)", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    const configPath = await writeProjectConfig(directory, {
      jobs: [
        { slug: "job-a", name: "Job A" },
        { slug: "job-b", name: "Job B" },
      ],
      workflows: [{ slug: "wf-a", name: "Workflow A", steps: [] }],
    });

    mockRemoteState([], []);

    const result = await runCommand(["diff", "--config", configPath, "--json"]);
    const diff = JSON.parse(result.stdout) as DiffResult;

    expect(diff.additions).toHaveLength(3);
    expect(diff.additions.every((e) => e.action === "add")).toBe(true);
    expect(diff.modifications).toHaveLength(0);
    expect(diff.removals).toHaveLength(0);
  });

  test("detects removed jobs and workflows (removals)", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    const configPath = await writeProjectConfig(directory, {
      jobs: [],
      workflows: [],
    });

    mockRemoteState(
      [{ slug: "old-job", name: "Old Job", status: "active" }],
      [{ slug: "old-wf", name: "Old Workflow" }]
    );

    const result = await runCommand(["diff", "--config", configPath, "--json"]);
    const diff = JSON.parse(result.stdout) as DiffResult;

    expect(diff.removals).toHaveLength(2);
    expect(diff.additions).toHaveLength(0);
    expect(diff.warnings.length).toBeGreaterThan(0);
    expect(diff.warnings.some((w) => w.includes("old-job"))).toBe(true);
  });

  test("detects modified jobs with per-field changes", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    const configPath = await writeProjectConfig(directory, {
      jobs: [{ slug: "payment", name: "Payment V2", timeout: 60 }],
    });

    mockRemoteState(
      [
        {
          slug: "payment",
          name: "Payment",
          timeout: 30,
          max_concurrency: 10,
        },
      ],
      []
    );

    const result = await runCommand(["diff", "--config", configPath, "--json"]);
    const diff = JSON.parse(result.stdout) as DiffResult;

    expect(diff.modifications).toHaveLength(1);
    expect(diff.modifications[0].slug).toBe("payment");

    const fields = diff.modifications[0].fields ?? [];
    expect(fields.some((f) => f.field === "name")).toBe(true);
    expect(fields.some((f) => f.field === "timeout")).toBe(true);

    const nameField = fields.find((f) => f.field === "name");
    expect(nameField?.local).toBe("Payment V2");
    expect(nameField?.remote).toBe("Payment");
  });

  test("no changes produces empty diff result", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    const configPath = await writeProjectConfig(directory, {
      jobs: [{ slug: "job-a", name: "Job A" }],
    });

    mockRemoteState([{ slug: "job-a", name: "Job A" }], []);

    const result = await runCommand(["diff", "--config", configPath, "--json"]);
    const diff = JSON.parse(result.stdout) as DiffResult;

    expect(diff.additions).toHaveLength(0);
    expect(diff.modifications).toHaveLength(0);
    expect(diff.removals).toHaveLength(0);
  });

  test("mixed additions, modifications, and removals", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    const configPath = await writeProjectConfig(directory, {
      jobs: [
        { slug: "new-job", name: "New Job" },
        { slug: "modified-job", name: "Modified V2" },
      ],
    });

    mockRemoteState(
      [
        { slug: "modified-job", name: "Modified V1" },
        { slug: "removed-job", name: "Removed Job" },
      ],
      []
    );

    const result = await runCommand(["diff", "--config", configPath, "--json"]);
    const diff = JSON.parse(result.stdout) as DiffResult;

    expect(diff.additions.some((e) => e.slug === "new-job")).toBe(true);
    expect(diff.modifications.some((e) => e.slug === "modified-job")).toBe(
      true
    );
    expect(diff.removals.some((e) => e.slug === "removed-job")).toBe(true);
  });

  test("warning for removing job with active status", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    const configPath = await writeProjectConfig(directory, {
      jobs: [],
    });

    mockRemoteState(
      [{ slug: "active-processor", name: "Active", status: "active" }],
      []
    );

    const result = await runCommand(["diff", "--config", configPath, "--json"]);
    const diff = JSON.parse(result.stdout) as DiffResult;

    expect(diff.removals.some((e) => e.slug === "active-processor")).toBe(true);
    expect(diff.warnings.length).toBeGreaterThan(0);
    expect(diff.warnings.some((w) => w.includes("active-processor"))).toBe(
      true
    );
  });

  test("plain text output contains diff markers without --json", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    const configPath = await writeProjectConfig(directory, {
      jobs: [{ slug: "new-job", name: "New Job" }],
    });

    mockRemoteState([{ slug: "removed-job", name: "Removed Job" }], []);

    const result = await runCommand(["diff", "--config", configPath]);

    expect(result.stdout).toContain("+ NEW");
    expect(result.stdout).toContain("- REMOVE");
    expect(result.stdout).toMatch(DIFF_SUMMARY_RE);
  });

  test("JSON output is valid DiffResult shape", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    const configPath = await writeProjectConfig(directory, {
      jobs: [{ slug: "job-a", name: "Job A" }],
    });

    mockRemoteState([], []);

    const result = await runCommand(["diff", "--config", configPath, "--json"]);
    const diff = JSON.parse(result.stdout) as DiffResult;

    expect(diff).toHaveProperty("additions");
    expect(diff).toHaveProperty("modifications");
    expect(diff).toHaveProperty("removals");
    expect(diff).toHaveProperty("warnings");

    for (const entry of diff.additions) {
      expect(entry).toHaveProperty("kind");
      expect(entry).toHaveProperty("slug");
      expect(entry).toHaveProperty("action");
    }
  });

  test("diff hits correct API endpoints with project_id", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    const configPath = await writeProjectConfig(directory, {
      projectId: "proj-diff",
      jobs: [{ slug: "job-a", name: "Job A" }],
    });

    const calls: HttpCall[] = [];
    mockRemoteState([], [], calls);

    await runCommand(["diff", "--config", configPath, "--json"]);

    const jobsCall = calls.find((c) => c.url.pathname === "/v1/jobs");
    expect(jobsCall).toBeDefined();
    expect(jobsCall?.url.searchParams.get("project_id")).toBe("proj-diff");

    const workflowsCall = calls.find((c) => c.url.pathname === "/v1/workflows");
    expect(workflowsCall).toBeDefined();
    expect(workflowsCall?.url.searchParams.get("project_id")).toBe("proj-diff");
  });

  test("--env flag is passed through for environment context", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    const configPath = await writeProjectConfig(directory, {
      jobs: [{ slug: "job-a", name: "Job A" }],
    });

    mockRemoteState([], []);

    const result = await runCommand([
      "diff",
      "--config",
      configPath,
      "--env",
      "production",
      "--json",
    ]);
    const diff = JSON.parse(result.stdout) as DiffResult;

    expect(diff).toHaveProperty("additions");
  });

  test("config not found produces clear error", async () => {
    const directory = await createTemporaryDirectory();
    process.env.STRAIT_PROFILE_PATH = join(directory, "cli.json");

    mockRemoteState([], []);

    const result = await runCommand([
      "diff",
      "--config",
      join(directory, "nonexistent.json"),
      "--json",
    ]);

    const hasError =
      result.stderr.toLowerCase().includes("config") ||
      result.stderr.toLowerCase().includes("error") ||
      result.stdout === "";
    expect(hasError).toBe(true);
  });
});
