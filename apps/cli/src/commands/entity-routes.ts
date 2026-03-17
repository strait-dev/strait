import { buildCommand, buildRouteMap } from "@stricli/core";
import { Effect } from "effect";

import type { StraitCommandContext } from "../context";
import { ApiServiceTag } from "../runtime";
import {
  normalizeCollection,
  renderPayload,
  summarizeRecord,
} from "./operational-helpers";

export type EntityCommandFlags = {
  readonly context?: string;
  readonly server?: string;
  readonly project?: string;
  readonly json?: boolean;
};

export type EntityRouteSpec = {
  readonly groupName: string;
  readonly groupBrief: string;
  readonly basePath: string;
  readonly idPlaceholder: string;
  readonly idCandidates: readonly string[];
  readonly listBrief: string;
  readonly getBrief: string;
  readonly supportProjectFilter?: boolean;
};

/**
 * Builds deterministic `list` and `get` routes for API-backed entity groups.
 */
export const buildEntityRoutes = (spec: EntityRouteSpec) => {
  const listCommand = buildCommand({
    async func(this: StraitCommandContext, flags: EntityCommandFlags) {
      await this.runEffect(
        Effect.gen(function* () {
          const apiService = yield* ApiServiceTag;

          const response = yield* apiService.requestJson<unknown>({
            method: "GET",
            path: spec.basePath,
            query: {
              project_id: spec.supportProjectFilter ? flags.project : undefined,
            },
            requireProject: spec.supportProjectFilter === true,
            connection: {
              contextName: flags.context,
              serverUrl: flags.server,
              projectId: flags.project,
            },
          });

          const records = normalizeCollection(response);
          const plainSummary = records.map((record) =>
            summarizeRecord(record, spec.idCandidates)
          );

          yield* renderPayload(flags.json ? response : records, {
            asJson: flags.json,
            plainSummary,
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
          brief: "Project filter",
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
      brief: spec.listBrief,
    },
  });

  const getCommand = buildCommand({
    async func(
      this: StraitCommandContext,
      flags: EntityCommandFlags,
      id: string
    ) {
      await this.runEffect(
        Effect.gen(function* () {
          const apiService = yield* ApiServiceTag;

          const response = yield* apiService.requestJson<unknown>({
            method: "GET",
            path: `${spec.basePath}/${encodeURIComponent(id)}`,
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
            brief: `Identifier for ${spec.groupName} resource`,
            parse: String,
            placeholder: spec.idPlaceholder,
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
      brief: spec.getBrief,
    },
  });

  return buildRouteMap({
    routes: {
      list: listCommand,
      get: getCommand,
    },
    docs: {
      brief: spec.groupBrief,
    },
  });
};
