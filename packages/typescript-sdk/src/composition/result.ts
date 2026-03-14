/**
 * A discriminated Result type for SDK operations that should not throw.
 *
 * `ok: true` carries `output`, while `ok: false` carries `error`.
 */
export type SdkResult<TOutput, TError = unknown> =
  | SdkSuccess<TOutput, TError>
  | SdkFailure<TOutput, TError>;

type SdkSuccess<TOutput, TError> = {
  readonly ok: true;
  readonly output: TOutput;
  readonly error: undefined;
  unwrap: () => TOutput;
  match: <A>(branches: {
    readonly ok: (value: TOutput) => A;
    readonly error: (error: TError) => A;
  }) => A;
};

type SdkFailure<TOutput, TError> = {
  readonly ok: false;
  readonly output: undefined;
  readonly error: TError;
  unwrap: () => TOutput;
  match: <A>(branches: {
    readonly ok: (value: TOutput) => A;
    readonly error: (error: TError) => A;
  }) => A;
};

/**
 * Creates a successful {@link SdkResult}.
 */
export const ok = <TOutput, TError = never>(
  output: TOutput
): SdkResult<TOutput, TError> => ({
  ok: true,
  output,
  error: undefined,
  unwrap: () => output,
  match: (branches) => branches.ok(output),
});

/**
 * Creates a failed {@link SdkResult}.
 */
export const err = <TOutput = never, TError = unknown>(
  error: TError
): SdkResult<TOutput, TError> => ({
  ok: false,
  output: undefined,
  error,
  unwrap: () => {
    throw error;
  },
  match: (branches) => branches.error(error),
});

/**
 * Type guard for successful {@link SdkResult} values.
 */
export const isOk = <TOutput, TError>(
  value: SdkResult<TOutput, TError>
): value is SdkSuccess<TOutput, TError> => value.ok;

/**
 * Type guard for failed {@link SdkResult} values.
 */
export const isErr = <TOutput, TError>(
  value: SdkResult<TOutput, TError>
): value is SdkFailure<TOutput, TError> => !value.ok;

/**
 * Converts a throwing Promise-returning operation into a {@link SdkResult}.
 */
export const fromPromise = async <TOutput, TError = unknown>(
  operation: () => Promise<TOutput>
): Promise<SdkResult<TOutput, TError>> => {
  try {
    const output = await operation();
    return ok<TOutput, TError>(output);
  } catch (error) {
    return err<TOutput, TError>(error as TError);
  }
};
