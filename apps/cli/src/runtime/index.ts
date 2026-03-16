import { Effect, Layer } from "effect";

import { ApiServiceLive, type ApiServiceTag } from "./api-service";
import { AuthServiceLive, type AuthServiceTag } from "./auth-service";
import { ConfigServiceLive, type ConfigServiceTag } from "./config-service";
import {
  FsProcessServiceLive,
  type FsProcessServiceTag,
} from "./fs-process-service";
import { ProfileStoreLive, type ProfileStoreTag } from "./profile-store";
import {
  RendererServiceLive,
  type RendererServiceTag,
} from "./renderer-service";
import {
  TelemetryServiceLive,
  type TelemetryServiceTag,
} from "./telemetry-service";

export * from "./api-service";
export * from "./auth-service";
export * from "./config-service";
export * from "./contracts";
export * from "./errors";
export * from "./fs-process-service";
export * from "./profile-store";
export * from "./renderer-service";
export * from "./telemetry-service";

/**
 * Union of all runtime services required by foundational commands.
 */
export type CliRuntime =
  | ProfileStoreTag
  | ConfigServiceTag
  | AuthServiceTag
  | ApiServiceTag
  | FsProcessServiceTag
  | RendererServiceTag
  | TelemetryServiceTag;

const profileAndConfigLayer =
  Layer.provideMerge(ProfileStoreLive)(ConfigServiceLive);
const profileConfigAndAuthLayer = Layer.provideMerge(profileAndConfigLayer)(
  AuthServiceLive
);
const apiLayer = Layer.provideMerge(profileConfigAndAuthLayer)(ApiServiceLive);

/**
 * Live runtime layer used by CLI command handlers.
 */
export const CliRuntimeLive = Layer.mergeAll(
  apiLayer,
  FsProcessServiceLive,
  RendererServiceLive,
  TelemetryServiceLive
);

/**
 * Executes an Effect program against the default runtime service graph.
 */
export const runWithRuntime = <A, E>(
  effect: Effect.Effect<A, E, CliRuntime>
): Promise<A> => Effect.runPromise(Effect.provide(effect, CliRuntimeLive));
