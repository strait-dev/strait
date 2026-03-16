import { mkdir, readFile, writeFile } from "node:fs/promises";
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import { Context, Effect, Layer } from "effect";

import {
  EMPTY_PROFILE,
  type StraitCliContext,
  type StraitCliProfile,
} from "./contracts";

const DEFAULT_CONFIG_DIRECTORY = join(homedir(), ".config", "strait");
const DEFAULT_PROFILE_FILENAME = "cli.json";

type ProfileStore = {
  /** Returns the absolute profile path used for persistence. */
  readonly profilePath: Effect.Effect<string>;
  /** Loads and validates CLI profile data from disk. */
  readonly load: Effect.Effect<StraitCliProfile, Error>;
  /** Persists CLI profile data to disk. */
  readonly save: (profile: StraitCliProfile) => Effect.Effect<void, Error>;
};

/**
 * Runtime service responsible for reading/writing persisted CLI profile state.
 */
export class ProfileStoreTag extends Context.Tag("ProfileStore")<
  ProfileStoreTag,
  ProfileStore
>() {}

const coerceContext = (value: unknown): StraitCliContext | undefined => {
  if (typeof value !== "object" || value === null) {
    return undefined;
  }

  const record = value as Record<string, unknown>;
  if (
    typeof record.serverUrl !== "string" ||
    record.serverUrl.trim().length === 0
  ) {
    return undefined;
  }

  const projectId =
    typeof record.projectId === "string" && record.projectId.trim().length > 0
      ? record.projectId
      : undefined;
  const apiKey =
    typeof record.apiKey === "string" && record.apiKey.trim().length > 0
      ? record.apiKey
      : undefined;

  return {
    serverUrl: record.serverUrl,
    projectId,
    apiKey,
  };
};

const coerceProfile = (value: unknown): StraitCliProfile => {
  if (typeof value !== "object" || value === null) {
    return EMPTY_PROFILE;
  }

  const record = value as Record<string, unknown>;
  const activeContext =
    typeof record.activeContext === "string" &&
    record.activeContext.trim().length > 0
      ? record.activeContext
      : undefined;

  const contextsRecord =
    typeof record.contexts === "object" && record.contexts !== null
      ? (record.contexts as Record<string, unknown>)
      : {};

  const contexts = Object.fromEntries(
    Object.entries(contextsRecord)
      .map(
        ([name, contextValue]) => [name, coerceContext(contextValue)] as const
      )
      .filter((entry): entry is readonly [string, StraitCliContext] =>
        Boolean(entry[1])
      )
  );

  return {
    activeContext,
    contexts,
  };
};

const resolveProfilePath = (): string => {
  const configuredDirectory =
    process.env.STRAIT_CONFIG_DIR?.trim() || DEFAULT_CONFIG_DIRECTORY;
  const configuredPath = process.env.STRAIT_PROFILE_PATH?.trim();

  if (configuredPath && configuredPath.length > 0) {
    return configuredPath;
  }

  return join(configuredDirectory, DEFAULT_PROFILE_FILENAME);
};

const liveProfileStore: ProfileStore = {
  profilePath: Effect.sync(resolveProfilePath),
  load: Effect.gen(function* () {
    const profilePath = yield* Effect.sync(resolveProfilePath);

    const readResult = yield* Effect.tryPromise({
      try: () => readFile(profilePath, "utf8"),
      catch: (error) => error,
    }).pipe(
      Effect.catchAll((error) => {
        const nodeError = error as NodeJS.ErrnoException;
        if (nodeError.code === "ENOENT") {
          return Effect.succeed<string>("");
        }
        return Effect.fail(
          new Error(`failed to read profile '${profilePath}'`, { cause: error })
        );
      })
    );

    if (readResult.trim().length === 0) {
      return EMPTY_PROFILE;
    }

    return yield* Effect.try({
      try: () => coerceProfile(JSON.parse(readResult) as unknown),
      catch: (error) =>
        new Error(`failed to parse profile '${profilePath}' as JSON`, {
          cause: error,
        }),
    });
  }),
  save: (profile) =>
    Effect.gen(function* () {
      const profilePath = yield* Effect.sync(resolveProfilePath);
      const profileDirectory = dirname(profilePath);

      yield* Effect.tryPromise({
        try: () => mkdir(profileDirectory, { recursive: true }),
        catch: (error) =>
          new Error(
            `failed to create profile directory '${profileDirectory}'`,
            {
              cause: error,
            }
          ),
      });

      yield* Effect.tryPromise({
        try: () =>
          writeFile(
            profilePath,
            `${JSON.stringify(profile, null, 2)}\n`,
            "utf8"
          ),
        catch: (error) =>
          new Error(`failed to write profile '${profilePath}'`, {
            cause: error,
          }),
      });
    }),
};

/**
 * Live profile-store layer backed by local JSON file persistence.
 */
export const ProfileStoreLive = Layer.succeed(
  ProfileStoreTag,
  liveProfileStore
);
