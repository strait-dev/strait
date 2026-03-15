import { waitForRun, type WaitForRunOptions } from "./wait";

type TriggerFn<TInput, TRun extends { readonly id: string }> = (
  input: TInput
) => Promise<TRun>;

type GetRunFn<TRun extends { readonly status?: string }> = (input: {
  readonly pathParams: { readonly runID: string };
}) => Promise<TRun>;

/**
 * Composes a trigger call with `waitForRun` to trigger a run and poll
 * until it reaches a terminal status.
 *
 * This is a standalone composition helper for users not using the authoring DSL.
 *
 * @param triggerFn - Function that triggers the run and returns the run object with `id`.
 * @param getRun - Function that fetches the run status by ID.
 * @param triggerInput - Input to pass to the trigger function.
 * @param waitOptions - Optional polling configuration.
 * @returns The final run state after reaching a terminal status.
 *
 * @example
 * ```ts
 * const result = await triggerAndWait(
 *   (input) => client.triggerJob({ pathParams: { jobID: "job_1" }, body: input }),
 *   (input) => client.getRun(input),
 *   { payload: { sku: "ABC-123" } },
 *   { timeoutMs: 120_000 },
 * );
 * console.log(result.status); // "completed"
 * ```
 */
export const triggerAndWait = async <
  TInput,
  TRun extends { readonly id: string; readonly status?: string },
>(
  triggerFn: TriggerFn<TInput, TRun>,
  getRun: GetRunFn<TRun>,
  triggerInput: TInput,
  waitOptions?: WaitForRunOptions
): Promise<TRun> => {
  const run = await triggerFn(triggerInput);
  return waitForRun(getRun, run.id, waitOptions);
};
