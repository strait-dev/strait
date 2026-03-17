import { buildCommand, buildRouteMap } from "@stricli/core";
import { Effect } from "effect";

import type { StraitCommandContext } from "../context";
import { ApiServiceTag, ConfigServiceTag } from "../runtime";
import { renderPayload } from "./operational-helpers";

type SecretsFlags = {
  readonly context?: string;
  readonly server?: string;
  readonly project?: string;
  readonly json?: boolean;
};

type ListSecretsFlags = SecretsFlags & {
  readonly jobId?: string;
  readonly environment?: string;
};

type CreateSecretFlags = SecretsFlags & {
  readonly key: string;
  readonly value: string;
  readonly environment: string;
  readonly jobId?: string;
};

/**
 * `strait secrets` command group for project/job secret lifecycle.
 */
export const secretsRoutes = buildRouteMap({
  routes: {
    list: buildCommand({
      async func(this: StraitCommandContext, flags: ListSecretsFlags) {
        await this.runEffect(
          Effect.gen(function* () {
            const apiService = yield* ApiServiceTag;

            const response = yield* apiService.requestJson<unknown>({
              method: "GET",
              path: "/v1/secrets",
              query: {
                project_id: flags.project,
                job_id: flags.jobId,
                environment: flags.environment,
              },
              requireProject: true,
              connection: {
                contextName: flags.context,
                serverUrl: flags.server,
                projectId: flags.project,
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
          project: {
            kind: "parsed",
            parse: String,
            brief: "Project override",
            optional: true,
          },
          jobId: {
            kind: "parsed",
            parse: String,
            brief: "Filter by job identifier",
            optional: true,
          },
          environment: {
            kind: "parsed",
            parse: String,
            brief: "Filter by environment",
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
        brief: "List secrets",
      },
    }),
    create: buildCommand({
      async func(this: StraitCommandContext, flags: CreateSecretFlags) {
        await this.runEffect(
          Effect.gen(function* () {
            const apiService = yield* ApiServiceTag;
            const configService = yield* ConfigServiceTag;

            const connection = yield* configService.resolveConnection({
              contextName: flags.context,
              serverUrl: flags.server,
              projectId: flags.project,
              requireProject: true,
            });

            const response = yield* apiService.requestJson<unknown>({
              method: "POST",
              path: "/v1/secrets",
              body: {
                project_id: connection.projectId,
                job_id: flags.jobId,
                environment: flags.environment,
                secret_key: flags.key,
                secret_value: flags.value,
              },
              connection: {
                contextName: connection.contextName,
                serverUrl: connection.serverUrl,
                projectId: connection.projectId,
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
          project: {
            kind: "parsed",
            parse: String,
            brief: "Project override",
            optional: true,
          },
          key: {
            kind: "parsed",
            parse: String,
            brief: "Secret key name",
          },
          value: {
            kind: "parsed",
            parse: String,
            brief: "Secret value",
          },
          environment: {
            kind: "parsed",
            parse: String,
            brief: "Target environment",
          },
          jobId: {
            kind: "parsed",
            parse: String,
            brief: "Optional job scope",
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
        brief: "Create secret",
      },
    }),
    delete: buildCommand({
      async func(
        this: StraitCommandContext,
        flags: SecretsFlags,
        secretId: string
      ) {
        await this.runEffect(
          Effect.gen(function* () {
            const apiService = yield* ApiServiceTag;

            yield* apiService.requestJson<void>({
              method: "DELETE",
              path: `/v1/secrets/${encodeURIComponent(secretId)}`,
              connection: {
                contextName: flags.context,
                serverUrl: flags.server,
                projectId: flags.project,
              },
            });

            yield* renderPayload(
              {
                id: secretId,
                deleted: true,
              },
              {
                asJson: Boolean(flags.json),
              }
            );
          })
        );
      },
      parameters: {
        positional: {
          kind: "tuple",
          parameters: [
            {
              brief: "Secret identifier",
              parse: String,
              placeholder: "secretId",
            },
          ],
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
          project: {
            kind: "parsed",
            parse: String,
            brief: "Project override",
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
        brief: "Delete secret",
      },
    }),
  },
  docs: {
    brief: "Manage project secrets",
  },
});
