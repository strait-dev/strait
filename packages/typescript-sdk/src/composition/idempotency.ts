type HeaderCarrier = {
  readonly headers?: Readonly<Record<string, string>>;
};

/**
 * Configuration for {@link withIdempotency}.
 */
export type IdempotencyOptions = {
  /** Header name used to carry the idempotency key. */
  readonly headerName?: string;
};

/**
 * Returns a shallow-cloned input object with an idempotency header attached.
 */
export const withIdempotency = <TInput extends HeaderCarrier>(
  input: TInput,
  key: string,
  options?: IdempotencyOptions
): TInput & { readonly headers: Readonly<Record<string, string>> } => {
  const headerName = options?.headerName ?? "Idempotency-Key";

  return {
    ...input,
    headers: {
      ...input.headers,
      [headerName]: key,
    },
  };
};
