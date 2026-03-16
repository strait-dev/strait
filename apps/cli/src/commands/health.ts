import { buildCommand } from "@stricli/core";
import { Effect } from "effect";

import type { StraitCommandContext } from "../context";
import {
  ApiServiceTag,
  ConfigServiceTag,
  RendererServiceTag,
} from "../runtime";

type HealthFlags = {
  readonly context?: string;
  readonly server?: string;
  readonly json?: boolean;
};

/**
 * `strait health` command implementation.
 */
export const healthCommand = buildCommand({
  async func(this: StraitCommandContext, flags: HealthFlags) {
    await this.runEffect(
      Effect.gen(function* () {
        const configService = yield* ConfigServiceTag;
        const apiService = yield* ApiServiceTag;
        const renderer = yield* RendererServiceTag;

        const connection = yield* configService.resolveConnection({
          contextName: flags.context,
          serverUrl: flags.server,
          requireServer: true,
        });

        const health = yield* apiService.health({
          contextName: flags.context,
          serverUrl: flags.server,
        });

        const payload = {
          status: health.status,
          serverUrl: connection.serverUrl,
          context: connection.contextName,
        };

        if (flags.json) {
          yield* renderer.json(payload);
          return;
        }

        yield* renderer.line(
          `Server health: ${payload.status} (${payload.serverUrl})`
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
      server: {
        kind: "parsed",
        parse: String,
        brief: "Server URL override",
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
    brief: "Check API server health endpoint",
  },
});
