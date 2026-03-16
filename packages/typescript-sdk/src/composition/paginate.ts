type PaginatedQuery = {
  readonly cursor?: string;
  readonly limit?: number;
};

type PaginatedResponse<TItem> = {
  readonly data?: readonly TItem[];
  readonly items?: readonly TItem[];
  readonly next_cursor?: string;
  readonly nextCursor?: string;
  readonly has_more?: boolean;
  readonly hasMore?: boolean;
};

/**
 * Creates an async iterator that automatically paginates through list API responses.
 *
 * Supports responses with `data` or `items` arrays, and `next_cursor`/`nextCursor`
 * for cursor-based pagination.
 *
 * @param listFn - Function that accepts a query object and returns a paginated response.
 * @param options - Optional pagination configuration.
 * @returns An async generator yielding individual items across all pages.
 *
 * @example
 * ```ts
 * // Iterate through all runs
 * for await (const run of paginate((q) => client.listRuns({ query: q }))) {
 *   console.log(run.id, run.status);
 * }
 *
 * // Collect all into an array
 * const allJobs = await collectAll(paginate((q) => client.listJobs({ query: q })));
 * ```
 */
export async function* paginate<TItem>(
  listFn: (query: PaginatedQuery) => Promise<PaginatedResponse<TItem>>,
  options?: { readonly limit?: number }
): AsyncGenerator<TItem> {
  let cursor: string | undefined;

  for (;;) {
    const response = await listFn({
      cursor,
      ...(options?.limit === undefined ? {} : { limit: options.limit }),
    });

    const items = response.data ?? response.items ?? [];

    for (const item of items) {
      yield item;
    }

    const nextCursor = response.next_cursor ?? response.nextCursor;
    const hasMore = response.has_more ?? response.hasMore;

    if (!nextCursor || hasMore === false || items.length === 0) {
      break;
    }

    cursor = nextCursor;
  }
}

/**
 * Collects all items from an async generator into an array.
 *
 * @param gen - An async generator (e.g. from {@link paginate}).
 * @returns A promise that resolves to an array of all yielded items.
 *
 * @example
 * ```ts
 * const allRuns = await collectAll(paginate((q) => client.listRuns({ query: q })));
 * ```
 */
export const collectAll = async <T>(gen: AsyncGenerator<T>): Promise<T[]> => {
  const result: T[] = [];
  for await (const item of gen) {
    result.push(item);
  }
  return result;
};
