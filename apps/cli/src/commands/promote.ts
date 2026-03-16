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

type PromoteFlags = {
  readonly config?: string;
  readonly context?: string;
  readonly server?: string;
  readonly env?: string;
  readonly json?: boolean;
};

/**
 * `strait promote` command promotes a finalized deployment version.
 */
export const promoteCommandRoute = buildCommand({
  async func(
    this: StraitCommandContext,
    flags: PromoteFlags,
    deploymentVersionID: string
  ) {
    await this.runEffect(
      Effect.gen(function* () {
        const apiService = yield* ApiServiceTag;

        const loadedConfig = yield* Effect.tryPromise({
          try: () => loadProjectConfig({ configPath: flags.config }),
          catch: (error) =>
            new Error("failed to load project config for promote", {
              cause: error,
            }),
        });

        const environment = resolveDeploymentEnvironment(
          loadedConfig.config,
          flags.env
        );
        const response = yield* apiService.requestJson<unknown>({
          method: "POST",
          path: `/v1/deployments/${encodeURIComponent(deploymentVersionID)}/promote`,
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
      parameters: [
        {
          brief: "Deployment version identifier",
          parse: String,
          placeholder: "deploymentVersionId",
        },
      ],
    },
    flags: {
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
    brief: "Promote a deployment version",
  },
});
