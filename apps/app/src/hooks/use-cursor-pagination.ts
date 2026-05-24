import { useCallback, useState } from "react";

export type CursorPaginationSearch = {
  cursor?: string;
  perPage?: number;
};

export type CursorPaginationControls = {
  cursor: string | undefined;
  perPage: number;
  canGoBack: boolean;
  goNext: (nextCursor: string) => void;
  goPrev: () => void;
  setPerPage: (n: number) => void;
  reset: () => void;
};

type Navigate = (updater: {
  search: (prev: CursorPaginationSearch) => CursorPaginationSearch;
}) => void;

const DEFAULT_PER_PAGE = 20;

export const useCursorPagination = (
  search: CursorPaginationSearch,
  navigate: Navigate,
  options?: { defaultPerPage?: number }
): CursorPaginationControls => {
  const defaultPerPage = options?.defaultPerPage ?? DEFAULT_PER_PAGE;
  const [stack, setStack] = useState<(string | undefined)[]>([]);

  const goNext = useCallback(
    (nextCursor: string) => {
      setStack((prev) => [...prev, search.cursor]);
      navigate({
        search: (prev) => ({ ...prev, cursor: nextCursor }),
      });
    },
    [navigate, search.cursor]
  );

  const goPrev = useCallback(() => {
    setStack((prev) => prev.slice(0, -1));
    const previous = stack.at(-1);
    navigate({
      search: (prev) => ({ ...prev, cursor: previous }),
    });
  }, [navigate, stack]);

  const setPerPage = useCallback(
    (n: number) => {
      setStack([]);
      navigate({
        search: (prev) => ({ ...prev, perPage: n, cursor: undefined }),
      });
    },
    [navigate]
  );

  const reset = useCallback(() => {
    setStack([]);
    navigate({
      search: (prev) => ({ ...prev, cursor: undefined }),
    });
  }, [navigate]);

  return {
    cursor: search.cursor,
    perPage: search.perPage ?? defaultPerPage,
    canGoBack: stack.length > 0,
    goNext,
    goPrev,
    setPerPage,
    reset,
  };
};
