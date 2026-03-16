import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { run } from "@stricli/core";

import { app } from "../src/cli";
import { buildContext } from "../src/context";

const temporaryDirectories: string[] = [];

const createTemporaryDirectory = async (): Promise<string> => {
  const directory = await mkdtemp(join(tmpdir(), "strait-cli-build-dev-"));
  temporaryDirectories.push(directory);
  return directory;
};

const runCommand = async (args: readonly string[]): Promise<string> => {
  let stdout = "";

  const originalStdoutWrite = process.stdout.write.bind(process.stdout);
  process.stdout.write = ((chunk: string | Uint8Array) => {
    stdout += chunk.toString();
    return true;
  }) as typeof process.stdout.write;

  try {
    await run(app, args, buildContext(process));
  } finally {
    process.stdout.write = originalStdoutWrite;
  }

  return stdout;
};

let originalEnv: NodeJS.ProcessEnv;

beforeEach(() => {
  originalEnv = { ...process.env };
});

afterEach(async () => {
  process.env = originalEnv;

  await Promise.all(
    temporaryDirectories
      .splice(0)
      .map((directory) => rm(directory, { recursive: true, force: true }))
  );
});

const writeConfig = async (directory: string): Promise<string> => {
  const outputDirectory = join(directory, "dist-manifest");
  const configPath = join(directory, "strait.config.ts");

  await writeFile(
    configPath,
    `export default {
  project: { id: "project-a", name: "Project A" },
  runtime: { kind: "bun" },
  build: { outDir: ${JSON.stringify(outputDirectory)} },
  jobs: [
    { name: "Job B", slug: "job-b", endpointUrl: "https://jobs.example.com/b" },
    { name: "Job A", slug: "job-a", endpointUrl: "https://jobs.example.com/a" }
  ],
  workflows: [
    { name: "Workflow 1", slug: "wf-1", steps: [{ name: "step-1", job: "job-a" }] }
  ]
};
`
  );

  return configPath;
};

describe("build/dev code-first pipeline", () => {
  test("build --dryRun emits deterministic manifest JSON", async () => {
    const directory = await createTemporaryDirectory();
    const configPath = await writeConfig(directory);

    const output = await runCommand([
      "build",
      "--config",
      configPath,
      "--dryRun",
      "--json",
    ]);

    const manifest = JSON.parse(output) as {
      readonly version: number;
      readonly runtime: string;
      readonly jobs: Array<{ readonly slug: string }>;
      readonly workflows: Array<{ readonly slug: string }>;
    };

    expect(manifest.version).toBe(1);
    expect(manifest.runtime).toBe("bun");
    expect(manifest.jobs.map((job) => job.slug)).toEqual(["job-a", "job-b"]);
    expect(manifest.workflows.map((workflow) => workflow.slug)).toEqual([
      "wf-1",
    ]);
  });

  test("build writes manifest file when not dry-run", async () => {
    const directory = await createTemporaryDirectory();
    const configPath = await writeConfig(directory);

    await runCommand(["build", "--config", configPath]);

    const manifestPath = join(directory, "dist-manifest", "manifest.json");
    const manifestText = await readFile(manifestPath, "utf8");
    const manifest = JSON.parse(manifestText) as {
      readonly build: { readonly outDir: string };
      readonly project: { readonly id: string };
    };

    expect(manifest.project.id).toBe("project-a");
    expect(manifest.build.outDir).toBe(join(directory, "dist-manifest"));
  });

  test("dev command returns project summary in JSON mode", async () => {
    const directory = await createTemporaryDirectory();
    const configPath = await writeConfig(directory);

    const output = await runCommand(["dev", "--config", configPath, "--json"]);
    const summary = JSON.parse(output) as {
      readonly runtime: string;
      readonly jobs: number;
      readonly workflows: number;
      readonly watch: boolean;
    };

    expect(summary.runtime).toBe("bun");
    expect(summary.jobs).toBe(2);
    expect(summary.workflows).toBe(1);
    expect(summary.watch).toBe(false);
  });
});
