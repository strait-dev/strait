import type { CommandContext } from "@stricli/core";
import type { Effect } from "effect";

import type { CliRuntime } from "./runtime";
import { runWithRuntime } from "./runtime";

/**
 * Shared command context passed to every unified CLI command.
 */
export interface StraitCommandContext extends CommandContext {
  /** Mirrors CI intent across command handlers. */
  readonly ci: boolean;
  /** Runs an Effect program against the live CLI runtime graph. */
  readonly runEffect: <A, E, R extends CliRuntime>(
    effect: Effect.Effect<A, E, R>
  ) => Promise<A>;
}

/**
 * Creates command context from the host process.
 */
export const buildContext = (
  processObject: NodeJS.Process
): StraitCommandContext => ({
  process: processObject,
  ci: processObject.env.CI === "true" || processObject.env.STRAIT_CI === "true",
  runEffect: runWithRuntime,
});
