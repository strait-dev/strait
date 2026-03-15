import { buildCommand, buildRouteMap } from "@stricli/core";
import { Effect } from "effect";

import type { StraitCommandContext } from "../context";
import {
  AuthServiceTag,
  ConfigServiceTag,
  MissingServerURLError,
  RendererServiceTag,
} from "../runtime";

type LoginFlags = {
  readonly context?: string;
  readonly apiKey?: string;
  readonly server?: string;
  readonly project?: string;
  readonly json?: boolean;
};

type LogoutFlags = {
  readonly context?: string;
  readonly json?: boolean;
};

type WhoamiFlags = {
  readonly context?: string;
  readonly json?: boolean;
};

const pickContextName = (
  explicit: string | undefined,
  active: string | undefined,
  fallback?: string
): string | undefined => explicit ?? active ?? fallback;

/**
 * `strait auth login` command implementation.
 */
export const authLoginCommand = buildCommand({
  async func(this: StraitCommandContext, flags: LoginFlags) {
    await this.runEffect(
      Effect.gen(function* () {
        const configService = yield* ConfigServiceTag;
        const authService = yield* AuthServiceTag;
        const renderer = yield* RendererServiceTag;

        const activeContextName = yield* configService.getActiveContextName;
        const contextName = pickContextName(
          flags.context,
          activeContextName,
          "default"
        );
        if (!contextName) {
          return yield* Effect.fail(
            new Error("failed to resolve target context name")
          );
        }

        const existingContext = yield* configService.getContext(contextName);

        const apiKey = flags.apiKey ?? process.env.STRAIT_API_KEY?.trim();
        if (!apiKey || apiKey.length === 0) {
          return yield* Effect.fail(
            new Error(
              "API key is required. Pass --api-key or set STRAIT_API_KEY environment variable."
            )
          );
        }

        const serverUrl =
          flags.server ??
          existingContext?.serverUrl ??
          process.env.STRAIT_SERVER?.trim();
        if (!serverUrl || serverUrl.length === 0) {
          return yield* Effect.fail(new MissingServerURLError());
        }

        yield* configService.upsertContext(contextName, {
          serverUrl,
          projectId:
            flags.project ??
            existingContext?.projectId ??
            process.env.STRAIT_PROJECT?.trim(),
        });

        const updatedContext = yield* authService.login(contextName, apiKey);

        if (flags.json) {
          yield* renderer.json({
            context: contextName,
            serverUrl: updatedContext.serverUrl,
            projectId: updatedContext.projectId,
            hasApiKey: Boolean(updatedContext.apiKey),
          });
          return;
        }

        yield* renderer.line(`Logged in for context '${contextName}'.`);
      })
    );
  },
  parameters: {
    positional: {
      kind: "tuple",
      parameters: [],
    },
    flags: {
      context: {
        kind: "parsed",
        parse: String,
        brief: "Context name override",
        optional: true,
      },
      apiKey: {
        kind: "parsed",
        parse: String,
        brief: "API key value",
        optional: true,
      },
      server: {
        kind: "parsed",
        parse: String,
        brief: "Server URL used when creating context",
        optional: true,
      },
      project: {
        kind: "parsed",
        parse: String,
        brief: "Default project ID for target context",
        optional: true,
      },
      json: {
        kind: "boolean",
        brief: "Output JSON",
        optional: true,
      },
    },
  },
  docs: {
    brief: "Store API key credentials in a CLI context",
  },
});

/**
 * `strait auth logout` command implementation.
 */
export const authLogoutCommand = buildCommand({
  async func(this: StraitCommandContext, flags: LogoutFlags) {
    await this.runEffect(
      Effect.gen(function* () {
        const configService = yield* ConfigServiceTag;
        const authService = yield* AuthServiceTag;
        const renderer = yield* RendererServiceTag;

        const activeContextName = yield* configService.getActiveContextName;
        const contextName = pickContextName(flags.context, activeContextName);

        if (!contextName) {
          return yield* Effect.fail(
            new Error("No active context found. Specify one with --context.")
          );
        }

        const updatedContext = yield* authService.logout(contextName);

        if (flags.json) {
          yield* renderer.json({
            context: contextName,
            serverUrl: updatedContext.serverUrl,
            projectId: updatedContext.projectId,
            hasApiKey: Boolean(updatedContext.apiKey),
          });
          return;
        }

        yield* renderer.line(`Logged out from context '${contextName}'.`);
      })
    );
  },
  parameters: {
    positional: {
      kind: "tuple",
      parameters: [],
    },
    flags: {
      context: {
        kind: "parsed",
        parse: String,
        brief: "Context name override",
        optional: true,
      },
      json: {
        kind: "boolean",
        brief: "Output JSON",
        optional: true,
      },
    },
  },
  docs: {
    brief: "Remove stored API key from a CLI context",
  },
});

/**
 * `strait auth whoami` command implementation.
 */
export const authWhoamiCommand = buildCommand({
  async func(this: StraitCommandContext, flags: WhoamiFlags) {
    await this.runEffect(
      Effect.gen(function* () {
        const authService = yield* AuthServiceTag;
        const renderer = yield* RendererServiceTag;

        const identity = yield* authService.whoami(flags.context);

        if (flags.json) {
          yield* renderer.json(identity);
          return;
        }

        if (!identity.contextName) {
          yield* renderer.line("No active context configured.");
          return;
        }

        yield* renderer.line(`Context: ${identity.contextName}`);
        yield* renderer.line(`Server: ${identity.serverUrl ?? "<unset>"}`);
        yield* renderer.line(
          `Project: ${identity.projectId ?? "<unset>"} | API key: ${
            identity.hasApiKey ? "configured" : "missing"
          }`
        );
      })
    );
  },
  parameters: {
    positional: {
      kind: "tuple",
      parameters: [],
    },
    flags: {
      context: {
        kind: "parsed",
        parse: String,
        brief: "Context name override",
        optional: true,
      },
      json: {
        kind: "boolean",
        brief: "Output JSON",
        optional: true,
      },
    },
  },
  docs: {
    brief: "Show local authentication identity status",
  },
});

/**
 * `strait auth` command group.
 */
export const authRoutes = buildRouteMap({
  routes: {
    login: authLoginCommand,
    logout: authLogoutCommand,
    whoami: authWhoamiCommand,
  },
  docs: {
    brief: "Authentication commands",
  },
});
