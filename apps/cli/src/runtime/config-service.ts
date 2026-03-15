import { Context, Effect, Layer } from "effect";

import type {
  ResolvedConnection,
  StraitCliContext,
  StraitCliProfile,
} from "./contracts";
import {
  ContextNotFoundError,
  MissingProjectIDError,
  MissingServerURLError,
} from "./errors";
import { ProfileStoreTag } from "./profile-store";

/**
 * Input used to upsert a CLI context.
 */
export type UpsertContextInput = {
  readonly serverUrl?: string;
  readonly projectId?: string;
  readonly apiKey?: string | null;
};

/**
 * Input used when resolving runtime connection details.
 */
export type ResolveConnectionInput = {
  readonly contextName?: string;
  readonly serverUrl?: string;
  readonly projectId?: string;
  readonly apiKey?: string;
  readonly requireServer?: boolean;
  readonly requireProject?: boolean;
};
/**
 * Lightweight context summary for list views.
 */
export type ContextSummary = {
  readonly name: string;
  readonly serverUrl: string;
  readonly projectId?: string;
  readonly hasApiKey: boolean;
  readonly active: boolean;
};

type ConfigService = {
  /** Returns profile-level active context name if configured. */
  readonly getActiveContextName: Effect.Effect<string | undefined, Error>;
  /** Lists available CLI contexts sorted by name. */
  readonly listContexts: Effect.Effect<readonly ContextSummary[], Error>;
  /** Fetches one named context. */
  readonly getContext: (
    contextName: string
  ) => Effect.Effect<StraitCliContext | undefined, Error>;
  /** Creates or updates a named context. */
  readonly upsertContext: (
    contextName: string,
    input: UpsertContextInput
  ) => Effect.Effect<StraitCliContext, Error>;
  /** Sets the active context. */
  readonly setActiveContext: (
    contextName: string
  ) => Effect.Effect<void, Error>;
  /** Resolves connection settings from flags, env vars, and profile context. */
  readonly resolveConnection: (
    input?: ResolveConnectionInput
  ) => Effect.Effect<ResolvedConnection, Error>;
};

/**
 * Runtime service for profile + context resolution.
 */
export class ConfigServiceTag extends Context.Tag("ConfigService")<
  ConfigServiceTag,
  ConfigService
>() {}

const getContextName = (
  profile: StraitCliProfile,
  explicitName?: string
): string | undefined => explicitName ?? profile.activeContext;

const makeConfigService = Effect.gen(function* () {
  const profileStore = yield* ProfileStoreTag;

  const loadProfile = profileStore.load;

  const persistProfile = (profile: StraitCliProfile) =>
    profileStore.save(profile);

  const resolveContext = (
    profile: StraitCliProfile,
    contextName?: string
  ): StraitCliContext | undefined => {
    const selectedContextName = getContextName(profile, contextName);
    if (!selectedContextName) {
      return undefined;
    }
    return profile.contexts[selectedContextName];
  };

  const service: ConfigService = {
    getActiveContextName: loadProfile.pipe(
      Effect.map((profile) => profile.activeContext)
    ),
    listContexts: loadProfile.pipe(
      Effect.map((profile) => {
        const activeContext = profile.activeContext;
        return Object.entries(profile.contexts)
          .map(([name, context]) => ({
            name,
            serverUrl: context.serverUrl,
            projectId: context.projectId,
            hasApiKey: Boolean(context.apiKey),
            active: name === activeContext,
          }))
          .sort((left, right) => left.name.localeCompare(right.name));
      })
    ),
    getContext: (contextName) =>
      loadProfile.pipe(Effect.map((profile) => profile.contexts[contextName])),
    upsertContext: (contextName, input) =>
      Effect.gen(function* () {
        const profile = yield* loadProfile;
        const existing = profile.contexts[contextName];

        const serverUrl = input.serverUrl ?? existing?.serverUrl;
        if (!serverUrl || serverUrl.trim().length === 0) {
          return yield* Effect.fail(new MissingServerURLError());
        }

        const apiKey =
          input.apiKey === undefined
            ? existing?.apiKey
            : (input.apiKey ?? undefined);

        const updatedContext: StraitCliContext = {
          serverUrl,
          projectId: input.projectId ?? existing?.projectId,
          apiKey,
        };

        const updatedProfile: StraitCliProfile = {
          activeContext: profile.activeContext ?? contextName,
          contexts: {
            ...profile.contexts,
            [contextName]: updatedContext,
          },
        };

        yield* persistProfile(updatedProfile);
        return updatedContext;
      }),
    setActiveContext: (contextName) =>
      Effect.gen(function* () {
        const profile = yield* loadProfile;

        if (!profile.contexts[contextName]) {
          return yield* Effect.fail(new ContextNotFoundError(contextName));
        }

        yield* persistProfile({
          ...profile,
          activeContext: contextName,
        });
      }),
    resolveConnection: (input) =>
      Effect.gen(function* () {
        const profile = yield* loadProfile;

        const contextName = getContextName(profile, input?.contextName);
        const context = resolveContext(profile, contextName);
        if (input?.contextName && !context) {
          return yield* Effect.fail(
            new ContextNotFoundError(input.contextName)
          );
        }

        const serverUrl =
          input?.serverUrl ??
          process.env.STRAIT_SERVER?.trim() ??
          context?.serverUrl;

        if (
          input?.requireServer !== false &&
          (!serverUrl || serverUrl.length === 0)
        ) {
          return yield* Effect.fail(new MissingServerURLError());
        }

        const projectId =
          input?.projectId ??
          process.env.STRAIT_PROJECT?.trim() ??
          context?.projectId;

        if (
          input?.requireProject === true &&
          (!projectId || projectId.length === 0)
        ) {
          return yield* Effect.fail(new MissingProjectIDError());
        }

        const apiKey =
          input?.apiKey ??
          process.env.STRAIT_API_KEY?.trim() ??
          context?.apiKey;

        return {
          serverUrl: serverUrl ?? "",
          projectId,
          apiKey,
          contextName,
        } satisfies ResolvedConnection;
      }),
  };

  return service;
});

/**
 * Live config service layer.
 */
export const ConfigServiceLive = Layer.effect(
  ConfigServiceTag,
  makeConfigService
);
