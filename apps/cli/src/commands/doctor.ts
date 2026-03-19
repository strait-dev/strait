import { buildCommand } from "@stricli/core";
import { Effect } from "effect";

import { findProjectConfigFile, loadProjectConfig } from "../compiler";
import type { StraitCommandContext } from "../context";
import {
  ApiServiceTag,
  ConfigServiceTag,
  FsProcessServiceTag,
  type JsonApiRequest,
  RendererServiceTag,
} from "../runtime";
import type { ResolveConnectionInput } from "../runtime/config-service";

type DoctorFlags = {
  readonly json?: boolean;
  readonly fix?: boolean;
  readonly verbose?: boolean;
  readonly context?: string;
  readonly server?: string;
};

type CheckResult = {
  name: string;
  status: "pass" | "fail" | "warn" | "skip";
  message: string;
  detail?: string;
  fix?: string;
};

const formatCheckPlain = (check: CheckResult, verbose: boolean): string[] => {
  const tag = check.status.toUpperCase();
  const lines = [`[${tag}] ${check.message}`];

  if (check.status !== "pass" && check.fix) {
    lines.push(`       Fix: ${check.fix}`);
  }

  if (verbose && check.detail) {
    lines.push(`       Detail: ${check.detail}`);
  }

  return lines;
};

const checkEnvVar = (
  envName: string,
  checkName: string,
  label: string
): CheckResult => {
  const value = process.env[envName] ?? "";
  if (value.trim().length > 0) {
    return {
      name: checkName,
      status: "pass",
      message: `${label}: configured`,
      detail: `${envName} is set`,
    };
  }
  return {
    name: checkName,
    status: "warn",
    message: `${label}: not configured`,
    fix: `Set ${envName} environment variable`,
  };
};

const runConfigCheck = (): Effect.Effect<CheckResult> =>
  Effect.promise(async () => {
    const configPath = await findProjectConfigFile();
    if (!configPath) {
      return {
        name: "config_file",
        status: "fail" as const,
        message: "Config file not found",
        detail:
          "Looked for strait.json or strait.config.* in current directory",
        fix: "Run 'strait init' to create a project config file",
      };
    }
    try {
      await loadProjectConfig();
      return {
        name: "config_file",
        status: "pass" as const,
        message: `Config file: ${configPath} found and valid`,
        detail: `Loaded from ${configPath}`,
      };
    } catch (error) {
      return {
        name: "config_file",
        status: "fail" as const,
        message: `Config file invalid: ${error instanceof Error ? error.message : String(error)}`,
        detail: `File found at ${configPath} but failed validation`,
        fix: "Check project config format: { project: { id: string } }",
      };
    }
  });

const runApiConnectivityCheck = (
  apiService: { health: ApiServiceTag["Type"]["health"] },
  connectionInput: ResolveConnectionInput | undefined
): Effect.Effect<CheckResult> =>
  Effect.gen(function* () {
    const start = Date.now();
    const result = yield* Effect.either(apiService.health(connectionInput));
    const latencyMs = Date.now() - start;

    if (result._tag === "Right") {
      return {
        name: "api_connectivity",
        status: "pass" as const,
        message: `API reachable: status=${result.right.status} (${latencyMs}ms)`,
        detail: `Health endpoint responded in ${latencyMs}ms`,
      };
    }
    return {
      name: "api_connectivity",
      status: "fail" as const,
      message: `API unreachable: ${result.left.message}`,
      detail: result.left.cause
        ? String(result.left.cause)
        : "Connection failed",
    };
  });

const runAuthCheck = (
  apiService: { requestJson: ApiServiceTag["Type"]["requestJson"] },
  connectionInput: ResolveConnectionInput | undefined
): Effect.Effect<CheckResult> =>
  Effect.gen(function* () {
    const authRequest: JsonApiRequest = {
      method: "GET",
      path: "/v1/stats",
      connection: connectionInput,
    };
    const result = yield* Effect.either(
      apiService.requestJson<Record<string, unknown>>(authRequest)
    );

    if (result._tag === "Right") {
      const apiKey = process.env.STRAIT_API_KEY ?? "";
      const masked =
        apiKey.length > 3 ? `...${apiKey.slice(-3)}` : "(configured)";
      return {
        name: "authentication",
        status: "pass" as const,
        message: `Authenticated: key ${masked}`,
        detail: "API key accepted by server",
      };
    }

    const errorMessage = result.left.message;
    if (errorMessage.includes("401")) {
      return {
        name: "authentication",
        status: "fail" as const,
        message: "Authentication failed: invalid API key",
        detail: errorMessage,
        fix: "Run 'strait auth login' with a valid API key",
      };
    }

    if (
      !process.env.STRAIT_API_KEY ||
      process.env.STRAIT_API_KEY.trim().length === 0
    ) {
      return {
        name: "authentication",
        status: "fail" as const,
        message: "Authentication failed: no API key configured",
        fix: "Set STRAIT_API_KEY or run 'strait auth login'",
      };
    }

    return {
      name: "authentication",
      status: "fail" as const,
      message: `Authentication check failed: ${errorMessage}`,
      detail: errorMessage,
    };
  });

const runSdkCheck = (
  fsProcess: FsProcessServiceTag["Type"]
): Effect.Effect<CheckResult> =>
  Effect.gen(function* () {
    const packagePath = "node_modules/@strait/ts/package.json";
    const exists = yield* fsProcess.exists(packagePath);
    if (!exists) {
      return {
        name: "sdk_version",
        status: "warn" as const,
        message: "SDK not installed: @strait/ts not found in node_modules",
        fix: "Run 'npm install @strait/ts' or 'bun add @strait/ts'",
      };
    }

    const content = yield* Effect.either(fsProcess.readTextFile(packagePath));
    if (content._tag !== "Right") {
      return {
        name: "sdk_version",
        status: "warn" as const,
        message: "SDK package.json found but could not read",
      };
    }

    try {
      const pkg = JSON.parse(content.right) as { version?: string };
      return {
        name: "sdk_version",
        status: "pass" as const,
        message: `SDK installed: @strait/ts@${pkg.version ?? "unknown"}`,
        detail: `Found at ${packagePath}`,
      };
    } catch {
      return {
        name: "sdk_version",
        status: "warn" as const,
        message: "SDK package.json found but could not parse version",
      };
    }
  });

const runDockerCheck = (
  fsProcess: FsProcessServiceTag["Type"]
): Effect.Effect<CheckResult> =>
  Effect.gen(function* () {
    const result = yield* Effect.either(
      fsProcess.run("docker", ["info"], { timeoutMs: 5000 })
    );

    if (result._tag === "Right" && result.right.exitCode === 0) {
      return {
        name: "docker",
        status: "pass" as const,
        message: "Docker: running",
        detail: "docker info returned successfully",
      };
    }

    return {
      name: "docker",
      status: "fail" as const,
      message: "Docker: not running",
      detail:
        result._tag === "Right"
          ? `docker info exited with code ${result.right.exitCode}`
          : result.left.message,
      fix: "Start Docker Desktop or the Docker daemon",
    };
  });

const runMigrationsCheck = (
  apiService: { requestJson: ApiServiceTag["Type"]["requestJson"] },
  connectionInput: ResolveConnectionInput | undefined
): Effect.Effect<CheckResult> =>
  Effect.gen(function* () {
    const request: JsonApiRequest = {
      method: "GET",
      path: "/health/ready",
      requireAuth: false,
      connection: connectionInput,
    };
    const result = yield* Effect.either(
      apiService.requestJson<Record<string, unknown>>(request)
    );

    if (result._tag === "Right") {
      return {
        name: "migrations",
        status: "pass" as const,
        message: "Migrations: up to date",
        detail: "Backend readiness check passed",
      };
    }

    return {
      name: "migrations",
      status: "fail" as const,
      message: "Migrations: backend not ready",
      detail: result.left.message,
    };
  });

const runPostgresCheck = (): CheckResult => {
  const dbUrl = process.env.DATABASE_URL ?? process.env.POSTGRES_URL ?? "";
  if (dbUrl.trim().length > 0) {
    return {
      name: "postgresql",
      status: "pass",
      message: "PostgreSQL config: configured",
      detail: "DATABASE_URL or POSTGRES_URL is set",
    };
  }
  return {
    name: "postgresql",
    status: "warn",
    message: "PostgreSQL config: not configured",
    fix: "Set DATABASE_URL environment variable",
  };
};

/**
 * `strait doctor` command validates local environment and connectivity.
 */
export const doctorCommand = buildCommand({
  async func(this: StraitCommandContext, flags: DoctorFlags) {
    await this.runEffect(
      Effect.gen(function* () {
        const apiService = yield* ApiServiceTag;
        const configService = yield* ConfigServiceTag;
        const fsProcess = yield* FsProcessServiceTag;
        const renderer = yield* RendererServiceTag;

        const connection = yield* Effect.either(
          configService.resolveConnection({
            contextName: flags.context,
            serverUrl: flags.server,
            requireServer: false,
          })
        );

        const connectionInput =
          connection._tag === "Right"
            ? {
                contextName: connection.right.contextName,
                serverUrl: connection.right.serverUrl,
              }
            : undefined;

        const checks: CheckResult[] = [
          {
            name: "cli_version",
            status: "pass",
            message: "CLI version: 0.0.0-dev",
          },
          yield* runConfigCheck(),
          yield* runApiConnectivityCheck(apiService, connectionInput),
          yield* runAuthCheck(apiService, connectionInput),
          yield* runSdkCheck(fsProcess),
          yield* runDockerCheck(fsProcess),
          runPostgresCheck(),
          checkEnvVar("REDIS_URL", "redis", "Redis config"),
          yield* runMigrationsCheck(apiService, connectionInput),
          checkEnvVar("FLY_API_TOKEN", "fly_credentials", "Fly credentials"),
        ];

        const hasFail = checks.some((check) => check.status === "fail");

        if (flags.json) {
          yield* renderer.json(checks);
        } else {
          for (const check of checks) {
            for (const line of formatCheckPlain(
              check,
              Boolean(flags.verbose)
            )) {
              yield* renderer.line(line);
            }
          }
          const passCount = checks.filter((c) => c.status === "pass").length;
          yield* renderer.line(`\n${passCount}/${checks.length} checks passed`);
        }

        if (hasFail) {
          process.exitCode = 1;
        }
      })
    );
  },
  parameters: {
    positional: {
      kind: "tuple",
      parameters: [],
    },
    flags: {
      json: {
        kind: "boolean",
        brief: "Output JSON",
        optional: true,
      },
      fix: {
        kind: "boolean",
        brief: "Auto-run fixable items",
        optional: true,
      },
      verbose: {
        kind: "boolean",
        brief: "Show detailed check information",
        optional: true,
      },
      context: {
        kind: "parsed",
        parse: String,
        brief: "Context name override",
        optional: true,
      },
      server: {
        kind: "parsed",
        parse: String,
        brief: "Server URL override",
        optional: true,
      },
    },
  },
  docs: {
    brief: "Validate local environment and connectivity",
  },
});
