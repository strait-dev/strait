import { Cause, Effect, Exit, Option } from "effect";

export function runPromise<A, E>(
  effect: Effect.Effect<A, E, never>
): Promise<A> {
  return Effect.runPromiseExit(effect).then((exit) => {
    if (Exit.isSuccess(exit)) {
      return exit.value;
    }

    const failure = Cause.failureOption(exit.cause);
    if (Option.isSome(failure)) {
      throw failure.value;
    }

    throw Cause.squash(exit.cause);
  });
}
