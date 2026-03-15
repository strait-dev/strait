import { Context, Effect, Layer } from "effect";
import { ConfigServiceTag } from "./config-service";
import type { StraitCliContext } from "./contracts";
import { ContextNotFoundError, MissingCredentialError } from "./errors";

/**
 * Identity view returned by `whoami`-style commands.
 */
export type AuthIdentity = {
  readonly contextName?: string;
  readonly serverUrl?: string;
  readonly projectId?: string;
  readonly hasApiKey: boolean;
};

type AuthService = {
  /** Stores an API key in the specified context. */
  readonly login: (
    contextName: string,
    apiKey: string
  ) => Effect.Effect<StraitCliContext, Error>;
  /** Removes API key material from the specified context. */
  readonly logout: (
    contextName: string
  ) => Effect.Effect<StraitCliContext, Error>;
  /** Resolves API key for the selected/default context. */
  readonly getApiKey: (
    contextName?: string
  ) => Effect.Effect<string | undefined, Error>;
  /** Returns local auth identity status for current context. */
  readonly whoami: (contextName?: string) => Effect.Effect<AuthIdentity, Error>;
  /** Resolves required API key or fails with actionable error. */
  readonly requireApiKey: (
    contextName?: string
  ) => Effect.Effect<string, Error>;
};

/**
 * Runtime service for credential lifecycle operations.
 */
export class AuthServiceTag extends Context.Tag("AuthService")<
  AuthServiceTag,
  AuthService
>() {}

const makeAuthService = Effect.gen(function* () {
  const configService = yield* ConfigServiceTag;

  const service: AuthService = {
    login: (contextName, apiKey) =>
      configService.upsertContext(contextName, {
        apiKey,
      }),
    logout: (contextName) =>
      Effect.gen(function* () {
        const existing = yield* configService.getContext(contextName);
        if (!existing) {
          return yield* Effect.fail(new ContextNotFoundError(contextName));
        }

        return yield* configService.upsertContext(contextName, {
          serverUrl: existing.serverUrl,
          projectId: existing.projectId,
          apiKey: null,
        });
      }),
    getApiKey: (contextName) =>
      configService
        .resolveConnection({
          contextName,
          requireServer: false,
        })
        .pipe(Effect.map((connection) => connection.apiKey)),
    whoami: (contextName) =>
      configService
        .resolveConnection({
          contextName,
          requireServer: false,
        })
        .pipe(
          Effect.map((connection) => ({
            contextName: connection.contextName,
            serverUrl: connection.serverUrl || undefined,
            projectId: connection.projectId,
            hasApiKey: Boolean(connection.apiKey),
          }))
        ),
    requireApiKey: (contextName) =>
      service.getApiKey(contextName).pipe(
        Effect.flatMap((apiKey) => {
          if (!apiKey || apiKey.trim().length === 0) {
            return Effect.fail(new MissingCredentialError());
          }
          return Effect.succeed(apiKey);
        })
      ),
  };

  return service;
});

/**
 * Live auth service layer.
 */
export const AuthServiceLive = Layer.effect(AuthServiceTag, makeAuthService);
