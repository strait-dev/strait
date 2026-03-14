import { constants } from "node:fs";
import { access } from "node:fs/promises";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { Effect, Schema } from "effect";

import { createClient, type StraitClient } from "../client";
import {
  normalizeBaseUrl,
  type StraitClientConfig,
  type StraitClientConfigInput,
  StraitClientConfigSchema,
} from "../config";
import type { FetchLike } from "../runtime";

const defaultConfigCandidates = [
  "strait.config.ts",
  "strait.config.mts",
  "strait.config.js",
  "strait.config.mjs",
  "strait.config.cjs",
] as const;

type StraitConfigModuleExport =
  | StraitClientConfigInput
  | { readonly client: StraitClientConfigInput };

type StraitConfigFactory = () =>
  | StraitConfigModuleExport
  | Promise<StraitConfigModuleExport>;

type StraitConfigResolvedExport =
  | StraitConfigModuleExport
  | StraitConfigFactory;

type StraitConfigModule = {
  readonly default?: StraitConfigResolvedExport;
  readonly config?: StraitConfigResolvedExport;
};

/**
 * Options used by config-file discovery and loading helpers.
 */
export type StraitConfigFileOptions = {
  /** Working directory where discovery begins. Defaults to `process.cwd()`. */
  readonly cwd?: string;
  /** Explicit config file path. Overrides candidate discovery. */
  readonly configPath?: string;
  /** Candidate file names searched under `cwd` in order. */
  readonly candidates?: readonly string[];
};

/**
 * Type helper for `strait.config.ts` authoring.
 */
export const defineStraitConfig = <TConfig extends StraitClientConfigInput>(
  config: TConfig
): TConfig => config;

const fileExists = async (path: string): Promise<boolean> => {
  try {
    await access(path, constants.F_OK);
    return true;
  } catch {
    return false;
  }
};

/**
 * Finds the first matching Strait config file.
 */
export const findStraitConfigFile = async (
  options?: StraitConfigFileOptions
): Promise<string | undefined> => {
  const cwd = options?.cwd ?? process.cwd();

  if (options?.configPath) {
    const absoluteConfigPath = resolve(cwd, options.configPath);
    return (await fileExists(absoluteConfigPath))
      ? absoluteConfigPath
      : undefined;
  }

  const candidates = options?.candidates ?? defaultConfigCandidates;
  for (const candidate of candidates) {
    const absoluteCandidatePath = resolve(cwd, candidate);
    if (await fileExists(absoluteCandidatePath)) {
      return absoluteCandidatePath;
    }
  }

  return undefined;
};

const decodeConfig = Schema.decodeUnknown(StraitClientConfigSchema);

const resolveExportedConfig = (
  value: StraitConfigResolvedExport | undefined
): Promise<unknown> => {
  if (typeof value === "function") {
    return Promise.resolve(value());
  }

  return Promise.resolve(value);
};

const unwrapConfigShape = (value: unknown): unknown => {
  if (
    typeof value === "object" &&
    value !== null &&
    "client" in value &&
    typeof (value as { readonly client: unknown }).client === "object" &&
    (value as { readonly client: unknown }).client !== null
  ) {
    return (value as { readonly client: unknown }).client;
  }

  return value;
};

/**
 * Loads and validates a Strait config file.
 *
 * Supported module exports:
 * - `export default defineStraitConfig({...})`
 * - `export const config = defineStraitConfig({...})`
 * - `export default () => ({...})`
 */
export const loadStraitConfig = async (
  options?: StraitConfigFileOptions
): Promise<StraitClientConfig> => {
  const configPath = await findStraitConfigFile(options);
  if (!configPath) {
    throw new Error(
      `No Strait config file found. Looked for ${(
        options?.candidates ?? defaultConfigCandidates
      )
        .map((candidate) => `'${candidate}'`)
        .join(", ")} in ${options?.cwd ?? process.cwd()}.`
    );
  }

  const moduleUrl = pathToFileURL(configPath).href;
  const importedModule = (await import(
    `${moduleUrl}?t=${Date.now()}`
  )) as StraitConfigModule;

  const resolvedExport = await resolveExportedConfig(
    importedModule.default ?? importedModule.config
  );
  const rawConfig = unwrapConfigShape(resolvedExport);

  if (rawConfig === undefined) {
    throw new Error(
      `Config file '${configPath}' must export a config via default export or named export 'config'.`
    );
  }

  try {
    const decoded = await Effect.runPromise(decodeConfig(rawConfig));
    return {
      ...decoded,
      baseUrl: normalizeBaseUrl(decoded.baseUrl),
    };
  } catch (error) {
    throw new Error(`Invalid Strait config in '${configPath}'.`, {
      cause: error,
    });
  }
};

/**
 * Creates a fully bound SDK client from `strait.config.ts` discovery/loading.
 */
export const createClientFromConfigFile = async (
  options?: StraitConfigFileOptions & { readonly fetch?: FetchLike }
): Promise<StraitClient> => {
  const config = await loadStraitConfig(options);
  return createClient(config, { fetch: options?.fetch });
};
