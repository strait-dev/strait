import { buildCommand } from "@stricli/core";
import { Effect } from "effect";

import type { StraitCommandContext } from "../context";
import { ApiServiceTag } from "../runtime";
import { renderPayload } from "./operational-helpers";

type StatsFlags = {
  readonly context?: string;
  readonly server?: string;
  readonly json?: boolean;
};

/**
 * `strait stats` command for queue and executor health counters.
 */
export const statsCommand = buildCommand({
  async func(this: StraitCommandContext, flags: StatsFlags) {
    await this.runEffect(
      Effect.gen(function* () {
        const apiService = yield* ApiServiceTag;

        const response = yield* apiService.requestJson<unknown>({
          method: "GET",
          path: "/v1/stats",
          connection: {
            contextName: flags.context,
            serverUrl: flags.server,
          },
        });

        yield* renderPayload(response, {
          asJson: Boolean(flags.json),
        });
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
    brief: "Show queue statistics",
  },
});
