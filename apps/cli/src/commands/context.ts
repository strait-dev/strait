import { buildCommand, buildRouteMap } from "@stricli/core";
import { Effect } from "effect";

import type { StraitCommandContext } from "../context";
import { ConfigServiceTag, RendererServiceTag } from "../runtime";

type ContextCreateFlags = {
  readonly server?: string;
  readonly project?: string;
  readonly apiKey?: string;
  readonly use?: boolean;
  readonly json?: boolean;
};

type ContextUseFlags = {
  readonly json?: boolean;
};

type ContextListFlags = {
  readonly json?: boolean;
};

type ContextCurrentFlags = {
  readonly json?: boolean;
};

/**
 * `strait context create` command implementation.
 */
export const contextCreateCommand = buildCommand({
  async func(
    this: StraitCommandContext,
    flags: ContextCreateFlags,
    name: string
  ) {
    await this.runEffect(
      Effect.gen(function* () {
        const configService = yield* ConfigServiceTag;
        const renderer = yield* RendererServiceTag;

        const updated = yield* configService.upsertContext(name, {
          serverUrl: flags.server,
          projectId: flags.project,
          apiKey: flags.apiKey,
        });

        if (flags.use) {
          yield* configService.setActiveContext(name);
        }

        if (flags.json) {
          yield* renderer.json({
            name,
            ...updated,
            active: Boolean(flags.use),
          });
          return;
        }

        yield* renderer.line(`Context '${name}' saved.`);
      })
    );
  },
  parameters: {
    positional: {
      kind: "tuple",
      parameters: [
        {
          brief: "Context name",
          parse: String,
          placeholder: "name",
        },
      ],
    },
    flags: {
      server: {
        kind: "parsed",
        parse: String,
        brief: "Server URL",
        optional: true,
      },
      project: {
        kind: "parsed",
        parse: String,
        brief: "Default project ID",
        optional: true,
      },
      apiKey: {
        kind: "parsed",
        parse: String,
        brief: "API key (stored in profile)",
        optional: true,
      },
      use: {
        kind: "boolean",
        brief: "Mark context as active",
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
    brief: "Create or update a named context",
  },
});

/**
 * `strait context use` command implementation.
 */
export const contextUseCommand = buildCommand({
  async func(this: StraitCommandContext, flags: ContextUseFlags, name: string) {
    await this.runEffect(
      Effect.gen(function* () {
        const configService = yield* ConfigServiceTag;
        const renderer = yield* RendererServiceTag;

        yield* configService.setActiveContext(name);

        if (flags.json) {
          yield* renderer.json({ activeContext: name });
          return;
        }

        yield* renderer.line(`Using context '${name}'.`);
      })
    );
  },
  parameters: {
    positional: {
      kind: "tuple",
      parameters: [
        {
          brief: "Context name",
          parse: String,
          placeholder: "name",
        },
      ],
    },
    flags: {
      json: {
        kind: "boolean",
        brief: "Output JSON",
        optional: true,
      },
    },
  },
  docs: {
    brief: "Set active context",
  },
});

/**
 * `strait context list` command implementation.
 */
export const contextListCommand = buildCommand({
  async func(this: StraitCommandContext, flags: ContextListFlags) {
    await this.runEffect(
      Effect.gen(function* () {
        const configService = yield* ConfigServiceTag;
        const renderer = yield* RendererServiceTag;

        const contexts = yield* configService.listContexts;

        if (flags.json) {
          yield* renderer.json(contexts);
          return;
        }

        if (contexts.length === 0) {
          yield* renderer.line("No contexts configured.");
          return;
        }

        for (const context of contexts) {
          const marker = context.active ? "*" : " ";
          yield* renderer.line(
            `${marker} ${context.name}  ${context.serverUrl}  project=${
              context.projectId ?? "<unset>"
            }  apiKey=${context.hasApiKey ? "yes" : "no"}`
          );
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
    },
  },
  docs: {
    brief: "List available contexts",
  },
});

/**
 * `strait context current` command implementation.
 */
export const contextCurrentCommand = buildCommand({
  async func(this: StraitCommandContext, flags: ContextCurrentFlags) {
    await this.runEffect(
      Effect.gen(function* () {
        const configService = yield* ConfigServiceTag;
        const renderer = yield* RendererServiceTag;

        const activeContextName = yield* configService.getActiveContextName;
        if (!activeContextName) {
          if (flags.json) {
            yield* renderer.json({ activeContext: null });
            return;
          }
          yield* renderer.line("No active context configured.");
          return;
        }

        const activeContext =
          yield* configService.getContext(activeContextName);

        const payload = {
          name: activeContextName,
          serverUrl: activeContext?.serverUrl,
          projectId: activeContext?.projectId,
          hasApiKey: Boolean(activeContext?.apiKey),
        };

        if (flags.json) {
          yield* renderer.json(payload);
          return;
        }

        yield* renderer.line(`Context: ${payload.name}`);
        yield* renderer.line(`Server: ${payload.serverUrl ?? "<unset>"}`);
        yield* renderer.line(
          `Project: ${payload.projectId ?? "<unset>"} | API key: ${
            payload.hasApiKey ? "configured" : "missing"
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
      json: {
        kind: "boolean",
        brief: "Output JSON",
        optional: true,
      },
    },
  },
  docs: {
    brief: "Show active context details",
  },
});

/**
 * `strait context` command group.
 */
export const contextRoutes = buildRouteMap({
  routes: {
    create: contextCreateCommand,
    use: contextUseCommand,
    list: contextListCommand,
    current: contextCurrentCommand,
  },
  docs: {
    brief: "Context management commands",
  },
});
