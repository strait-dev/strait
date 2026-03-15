import { constants } from "node:fs";
import { access, readFile } from "node:fs/promises";
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
  "strait.json",
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
  /**
   * Whether to apply environment variable overrides on top of the config file.
   *
   * When `true` (default), the following env vars override file-based values:
   * - `STRAIT_BASE_URL` → `baseUrl`
   * - `STRAIT_API_KEY` → `auth.token` (with `auth.type` = "bearer")
   * - `STRAIT_AUTH_TYPE` → `auth.type` ("bearer" | "apiKey" | "runToken")
   * - `STRAIT_TIMEOUT_MS` → `timeoutMs`
   *
   * @default true
   */
  readonly envOverrides?: boolean;
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

/**
 * Applies environment variable overrides to a raw config object.
 *
 * Supported env vars:
 * - `STRAIT_BASE_URL` → `baseUrl`
 * - `STRAIT_API_KEY` → `auth.token` (with `auth.type` defaulting to "bearer")
 * - `STRAIT_AUTH_TYPE` → `auth.type`
 * - `STRAIT_TIMEOUT_MS` → `timeoutMs`
 */
const applyEnvOverrides = (raw: unknown): unknown => {
  if (typeof raw !== "object" || raw === null) {
    return raw;
  }

  const config = { ...(raw as Record<string, unknown>) };

  const baseUrl = process.env.STRAIT_BASE_URL;
  if (baseUrl) {
    config.baseUrl = baseUrl;
  }

  const apiKey = process.env.STRAIT_API_KEY;
  const authType = process.env.STRAIT_AUTH_TYPE;
  if (apiKey) {
    config.auth = {
      type: (authType as "bearer" | "apiKey" | "runToken") ?? "bearer",
      token: apiKey,
    };
  } else if (
    authType &&
    typeof config.auth === "object" &&
    config.auth !== null
  ) {
    config.auth = {
      ...(config.auth as Record<string, unknown>),
      type: authType,
    };
  }

  const timeoutMs = process.env.STRAIT_TIMEOUT_MS;
  if (timeoutMs) {
    const parsed = Number(timeoutMs);
    if (Number.isFinite(parsed) && parsed > 0) {
      config.timeoutMs = parsed;
    }
  }

  return config;
};

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
 * Maps the `sdk` section of a `strait.json` file to `StraitClientConfigInput` shape.
 */
const mapJsonConfigToClientInput = (json: Record<string, unknown>): unknown => {
  const sdk =
    typeof json.sdk === "object" && json.sdk !== null
      ? (json.sdk as Record<string, unknown>)
      : {};

  const baseUrl = (sdk.base_url as string) ?? "";
  const authType = (sdk.auth_type as string) ?? "apiKey";
  const timeoutMs = sdk.timeout_ms as number | undefined;

  // Token always comes from env var — never from the file
  const token = process.env.STRAIT_API_KEY ?? "";

  return {
    baseUrl,
    auth: { type: authType, token },
    ...(timeoutMs === undefined ? {} : { timeoutMs }),
  };
};

const isJsonConfigPath = (path: string): boolean => path.endsWith(".json");

/**
 * Loads and validates a Strait config file.
 *
 * Supports both `strait.json` (universal) and `strait.config.ts` (deprecated).
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

  let rawConfig: unknown;

  if (isJsonConfigPath(configPath)) {
    const content = await readFile(configPath, "utf-8");
    const json = JSON.parse(content) as Record<string, unknown>;
    rawConfig = mapJsonConfigToClientInput(json);
  } else {
    console.warn(
      "strait.config.ts is deprecated. Run 'strait init' to migrate to strait.json."
    );

    const moduleUrl = pathToFileURL(configPath).href;
    const importedModule = (await import(
      `${moduleUrl}?t=${Date.now()}`
    )) as StraitConfigModule;

    const resolvedExport = await resolveExportedConfig(
      importedModule.default ?? importedModule.config
    );
    rawConfig = unwrapConfigShape(resolvedExport);

    if (rawConfig === undefined) {
      throw new Error(
        `Config file '${configPath}' must export a config via default export or named export 'config'.`
      );
    }
  }

  const shouldApplyEnv = options?.envOverrides !== false;
  const finalConfig = shouldApplyEnv ? applyEnvOverrides(rawConfig) : rawConfig;

  try {
    const decoded = await Effect.runPromise(decodeConfig(finalConfig));
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

/**
 * Creates a fully bound SDK client from environment variables only.
 *
 * Requires at minimum `STRAIT_BASE_URL` and `STRAIT_API_KEY` to be set.
 * Optionally reads `STRAIT_AUTH_TYPE` (default: "bearer") and
 * `STRAIT_TIMEOUT_MS`.
 *
 * @example
 * ```ts
 * // With STRAIT_BASE_URL and STRAIT_API_KEY set:
 * const client = createClientFromEnv();
 * ```
 */
export const createClientFromEnv = (options?: {
  readonly fetch?: FetchLike;
}): StraitClient => {
  const baseUrl = process.env.STRAIT_BASE_URL;
  const apiKey = process.env.STRAIT_API_KEY;

  if (!baseUrl) {
    throw new Error("STRAIT_BASE_URL environment variable is required");
  }
  if (!apiKey) {
    throw new Error("STRAIT_API_KEY environment variable is required");
  }

  const authType =
    (process.env.STRAIT_AUTH_TYPE as "bearer" | "apiKey" | "runToken") ??
    "bearer";

  const timeoutMs = process.env.STRAIT_TIMEOUT_MS
    ? Number(process.env.STRAIT_TIMEOUT_MS)
    : undefined;

  return createClient(
    {
      baseUrl,
      auth: { type: authType, token: apiKey },
      ...(timeoutMs && Number.isFinite(timeoutMs) && timeoutMs > 0
        ? { timeoutMs }
        : {}),
    },
    { fetch: options?.fetch }
  );
};
