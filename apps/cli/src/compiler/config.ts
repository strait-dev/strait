import { constants } from "node:fs";
import { access, readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";

import { defineConfig, type StraitProjectConfig } from "./types";

const defaultConfigCandidates = [
  "strait.json",
  "strait.config.ts",
  "strait.config.mts",
  "strait.config.js",
  "strait.config.mjs",
  "strait.config.cjs",
] as const;

type ProjectConfigModule = {
  readonly default?: unknown;
  readonly config?: unknown;
};

export type ProjectConfigLoadOptions = {
  readonly cwd?: string;
  readonly configPath?: string;
  readonly candidates?: readonly string[];
};

const fileExists = async (path: string): Promise<boolean> => {
  try {
    await access(path, constants.F_OK);
    return true;
  } catch {
    return false;
  }
};

const isProjectConfig = (value: unknown): value is StraitProjectConfig => {
  if (typeof value !== "object" || value === null) {
    return false;
  }

  const record = value as Record<string, unknown>;
  if (typeof record.project !== "object" || record.project === null) {
    return false;
  }

  const projectRecord = record.project as Record<string, unknown>;
  return (
    typeof projectRecord.id === "string" && projectRecord.id.trim().length > 0
  );
};

const resolveExport = (value: unknown): Promise<unknown> => {
  if (typeof value === "function") {
    return Promise.resolve((value as () => unknown)());
  }
  return Promise.resolve(value);
};

/**
 * Discovers project config file path.
 */
export const findProjectConfigFile = async (
  options?: ProjectConfigLoadOptions
): Promise<string | undefined> => {
  const cwd = options?.cwd ?? process.cwd();

  if (options?.configPath) {
    const explicitPath = resolve(cwd, options.configPath);
    return (await fileExists(explicitPath)) ? explicitPath : undefined;
  }

  const candidates = options?.candidates ?? defaultConfigCandidates;
  for (const candidate of candidates) {
    const candidatePath = resolve(cwd, candidate);
    if (await fileExists(candidatePath)) {
      return candidatePath;
    }
  }

  return undefined;
};

type StraitJsonSchema = {
  readonly project?: { readonly id?: string; readonly name?: string };
  readonly src?: string;
  readonly runtime?: string;
  readonly build?: { readonly out_dir?: string };
  readonly deploy?: { readonly default_environment?: string };
};

const mapJsonToProjectConfig = (
  json: StraitJsonSchema
): StraitProjectConfig | null => {
  if (!json.project?.id || json.project.id.trim().length === 0) {
    return null;
  }

  const src = json.src ?? "src";

  return defineConfig({
    project: {
      id: json.project.id,
      ...(json.project.name ? { name: json.project.name } : {}),
    },
    dirs: { jobs: src, workflows: src },
    runtime: { kind: (json.runtime as "node" | "bun") ?? "node" },
    build: { outDir: json.build?.out_dir ?? ".strait" },
    ...(json.deploy?.default_environment
      ? { deploy: { defaultEnvironment: json.deploy.default_environment } }
      : {}),
    jobs: [],
    workflows: [],
  });
};

/**
 * Loads and validates project config from `strait.json` or `strait.config.*`.
 */
export const loadProjectConfig = async (
  options?: ProjectConfigLoadOptions
): Promise<{ readonly path: string; readonly config: StraitProjectConfig }> => {
  const configPath = await findProjectConfigFile(options);
  if (!configPath) {
    throw new Error(
      `No project config file found. Looked for ${(
        options?.candidates ?? defaultConfigCandidates
      )
        .map((candidate) => `'${candidate}'`)
        .join(", ")} in ${options?.cwd ?? process.cwd()}.`
    );
  }

  if (configPath.endsWith(".json")) {
    const content = await readFile(configPath, "utf-8");
    const json = JSON.parse(content) as StraitJsonSchema;
    const mapped = mapJsonToProjectConfig(json);
    if (!mapped) {
      throw new Error(
        `Invalid Strait project config in '${configPath}'. Expected { project: { id: string }, ... }.`
      );
    }
    return { path: configPath, config: mapped };
  }

  console.warn(
    "strait.config.ts is deprecated. Run 'strait init' to migrate to strait.json."
  );

  const modulePath = pathToFileURL(configPath).href;
  const imported = (await import(
    `${modulePath}?t=${Date.now()}`
  )) as ProjectConfigModule;

  const rawExport = await resolveExport(imported.default ?? imported.config);
  if (!isProjectConfig(rawExport)) {
    throw new Error(
      `Invalid Strait project config in '${configPath}'. Expected { project: { id: string }, ... }.`
    );
  }

  const normalizedConfig: StraitProjectConfig = defineConfig({
    ...rawExport,
    runtime: rawExport.runtime ?? { kind: "node" },
    build: rawExport.build ?? { outDir: ".strait" },
  });

  return {
    path: configPath,
    config: normalizedConfig,
  };
};
