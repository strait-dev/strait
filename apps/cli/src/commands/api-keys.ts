import { buildCommand, buildRouteMap } from "@stricli/core";
import { Effect } from "effect";

import type { StraitCommandContext } from "../context";
import { ApiServiceTag, ConfigServiceTag } from "../runtime";
import { renderPayload } from "./operational-helpers";

type APIKeysFlags = {
  readonly context?: string;
  readonly server?: string;
  readonly project?: string;
  readonly json?: boolean;
};

type CreateAPIKeyFlags = APIKeysFlags & {
  readonly name: string;
  readonly scopes?: string;
};

type RotateAPIKeyFlags = APIKeysFlags & {
  readonly gracePeriodMinutes?: number;
};

const splitScopes = (raw?: string): readonly string[] | undefined => {
  if (!raw || raw.trim().length === 0) {
    return undefined;
  }

  const scopes = raw
    .split(",")
    .map((scope) => scope.trim())
    .filter((scope) => scope.length > 0);

  return scopes.length > 0 ? scopes : undefined;
};

/**
 * `strait api-keys` command group for key lifecycle operations.
 */
export const apiKeysRoutes = buildRouteMap({
  routes: {
    list: buildCommand({
      async func(this: StraitCommandContext, flags: APIKeysFlags) {
        await this.runEffect(
          Effect.gen(function* () {
            const apiService = yield* ApiServiceTag;

            const response = yield* apiService.requestJson<unknown>({
              method: "GET",
              path: "/v1/api-keys",
              query: {
                project_id: flags.project,
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
          json: {
            kind: "boolean",
            brief: "Output JSON",
            optional: true,
          },
        },
      },
      docs: {
        brief: "List API keys",
      },
    }),
    create: buildCommand({
      async func(this: StraitCommandContext, flags: CreateAPIKeyFlags) {
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
              path: "/v1/api-keys",
              body: {
                project_id: connection.projectId,
                name: flags.name,
                scopes: splitScopes(flags.scopes),
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
          name: {
            kind: "parsed",
            parse: String,
            brief: "Human-readable key name",
          },
          scopes: {
            kind: "parsed",
            parse: String,
            brief: "Comma-separated scopes",
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
        brief: "Create API key",
      },
    }),
    revoke: buildCommand({
      async func(
        this: StraitCommandContext,
        flags: APIKeysFlags,
        keyId: string
      ) {
        await this.runEffect(
          Effect.gen(function* () {
            const apiService = yield* ApiServiceTag;

            yield* apiService.requestJson<void>({
              method: "DELETE",
              path: `/v1/api-keys/${encodeURIComponent(keyId)}`,
              connection: {
                contextName: flags.context,
                serverUrl: flags.server,
                projectId: flags.project,
              },
            });

            yield* renderPayload(
              {
                id: keyId,
                revoked: true,
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
              brief: "API key identifier",
              parse: String,
              placeholder: "keyId",
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
        brief: "Revoke API key",
      },
    }),
    rotate: buildCommand({
      async func(
        this: StraitCommandContext,
        flags: RotateAPIKeyFlags,
        keyId: string
      ) {
        await this.runEffect(
          Effect.gen(function* () {
            const apiService = yield* ApiServiceTag;

            const body =
              flags.gracePeriodMinutes === undefined
                ? {}
                : {
                    grace_period_minutes: flags.gracePeriodMinutes,
                  };

            const response = yield* apiService.requestJson<unknown>({
              method: "POST",
              path: `/v1/api-keys/${encodeURIComponent(keyId)}/rotate`,
              body,
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
          parameters: [
            {
              brief: "API key identifier",
              parse: String,
              placeholder: "keyId",
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
          gracePeriodMinutes: {
            kind: "parsed",
            parse: Number,
            brief: "Old-key grace period in minutes",
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
        brief: "Rotate API key",
      },
    }),
  },
  docs: {
    brief: "Manage API keys",
  },
});
