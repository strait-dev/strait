import type { RunContext } from "../authoring/job";

export type CheckpointResumeOptions<TState extends Record<string, unknown>> = {
  readonly initialState: TState;
  readonly checkpointInterval?: number;
};

export const withCheckpointResume = async <
  TState extends Record<string, unknown>,
  TOutput,
>(
  ctx: RunContext,
  lastCheckpoint: TState | undefined,
  fn: (
    state: TState,
    updateState: (newState: TState) => void
  ) => Promise<TOutput>,
  options: CheckpointResumeOptions<TState>
): Promise<TOutput> => {
  let currentState = lastCheckpoint ?? options.initialState;
  const interval = options.checkpointInterval ?? 1;
  let stepCount = 0;

  const updateState = (newState: TState) => {
    currentState = newState;
    stepCount++;
    if (stepCount % interval === 0) {
      ctx.checkpoint(currentState).catch(() => undefined);
    }
  };

  const result = await fn(currentState, updateState);

  // Final checkpoint
  await ctx.checkpoint(currentState);

  return result;
};
