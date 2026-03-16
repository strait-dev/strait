import { buildCommand } from "@stricli/core";
import { Effect } from "effect";

import { loadProjectConfig } from "../compiler";
import type { StraitCommandContext } from "../context";
import { ApiServiceTag } from "../runtime";
import {
  createDeploymentMutationBody,
  resolveDeploymentEnvironment,
} from "./deployment-helpers";
import { renderPayload } from "./operational-helpers";

type RollbackFlags = {
  readonly to: string;
  readonly config?: string;
  readonly context?: string;
  readonly server?: string;
  readonly env?: string;
  readonly json?: boolean;
};

/**
 * `strait rollback` command promotes a previous deployment version as rollback target.
 */
export const rollbackCommandRoute = buildCommand({
  async func(this: StraitCommandContext, flags: RollbackFlags) {
    await this.runEffect(
      Effect.gen(function* () {
        const apiService = yield* ApiServiceTag;

        const loadedConfig = yield* Effect.tryPromise({
          try: () => loadProjectConfig({ configPath: flags.config }),
          catch: (error) =>
            new Error("failed to load project config for rollback", {
              cause: error,
            }),
        });

        const environment = resolveDeploymentEnvironment(
          loadedConfig.config,
          flags.env
        );
        const response = yield* apiService.requestJson<unknown>({
          method: "POST",
          path: `/v1/deployments/${encodeURIComponent(flags.to)}/rollback`,
          body: createDeploymentMutationBody(
            loadedConfig.config.project.id,
            environment
          ),
          connection: {
            contextName: flags.context,
            serverUrl: flags.server,
            projectId: loadedConfig.config.project.id,
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
      to: {
        kind: "parsed",
        parse: String,
        brief: "Deployment version identifier to roll back to",
      },
      config: {
        kind: "parsed",
        parse: String,
        brief: "Path to strait.config file",
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
      env: {
        kind: "parsed",
        parse: String,
        brief: "Target environment",
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
    brief: "Rollback to a deployment version",
  },
});
