import { constants } from "node:fs";
import { access } from "node:fs/promises";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";

import { defineConfig, type StraitProjectConfig } from "./types";

const defaultConfigCandidates = [
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

/**
 * Loads and validates `strait.config.*` into project config shape.
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
