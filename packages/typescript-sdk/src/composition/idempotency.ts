type HeaderCarrier = {
  readonly headers?: Readonly<Record<string, string>>;
};

export type IdempotencyOptions = {
  readonly headerName?: string;
};

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
