const terminalRunStatuses = new Set([
  "completed",
  "failed",
  "timed_out",
  "crashed",
  "system_failed",
  "canceled",
  "expired",
  "dead_letter",
]);

const sleep = (ms: number): Promise<void> =>
  new Promise((resolve) => {
    setTimeout(resolve, ms);
  });

/**
 * Polling policy for {@link waitForRun}.
 */
export type WaitForRunOptions = {
  /** Maximum wait time before timing out. */
  readonly timeoutMs?: number;
  /** Initial polling delay in milliseconds. */
  readonly initialDelayMs?: number;
  /** Maximum polling delay in milliseconds. */
  readonly maxDelayMs?: number;
  /** Exponential backoff multiplier for polling delay. */
  readonly factor?: number;
  /** Optional custom terminal-status predicate. */
  readonly isTerminal?: (status: string | undefined) => boolean;
};

type RunStatusResponse = {
  readonly status?: string;
};

type RunFetcher<TRun extends RunStatusResponse> = (input: {
  readonly pathParams: { readonly runID: string };
}) => Promise<TRun>;

/**
 * Polls `getRun` until the run reaches a terminal status or times out.
 */
export const waitForRun = async <TRun extends RunStatusResponse>(
  getRun: RunFetcher<TRun>,
  runID: string,
  options?: WaitForRunOptions
): Promise<TRun> => {
  const timeoutMs = options?.timeoutMs ?? 60_000;
  const factor = options?.factor ?? 1.5;
  const maxDelayMs = options?.maxDelayMs ?? 10_000;

  let delayMs = options?.initialDelayMs ?? 500;
  const startedAt = Date.now();

  for (;;) {
    const run = await getRun({ pathParams: { runID } });
    const status = run.status;

    const isTerminal =
      options?.isTerminal?.(status) ??
      (status !== undefined && terminalRunStatuses.has(status));

    if (isTerminal) {
      return run;
    }

    if (Date.now() - startedAt > timeoutMs) {
      throw new Error(
        `waitForRun timed out after ${timeoutMs}ms for run ${runID}`
      );
    }

    await sleep(delayMs);
    delayMs = Math.min(maxDelayMs, Math.max(1, Math.round(delayMs * factor)));
  }
};
