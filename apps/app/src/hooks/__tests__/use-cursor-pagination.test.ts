import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import {
  type CursorPaginationSearch,
  useCursorPagination,
} from "../use-cursor-pagination";

function makeNavigate() {
  let search: CursorPaginationSearch = {};
  const navigate = vi.fn(
    (updater: {
      search: (prev: CursorPaginationSearch) => CursorPaginationSearch;
    }) => {
      search = updater.search(search);
    }
  );
  return {
    navigate,
    getSearch: () => search,
  };
}

describe("useCursorPagination", () => {
  it("returns defaults when no search is set", () => {
    const { navigate } = makeNavigate();
    const { result } = renderHook(() => useCursorPagination({}, navigate));

    expect(result.current.cursor).toBeUndefined();
    expect(result.current.perPage).toBe(20);
    expect(result.current.canGoBack).toBe(false);
  });

  it("honors a custom default perPage", () => {
    const { navigate } = makeNavigate();
    const { result } = renderHook(() =>
      useCursorPagination({}, navigate, { defaultPerPage: 50 })
    );

    expect(result.current.perPage).toBe(50);
  });

  it("goNext pushes the current cursor onto the back stack", () => {
    const { navigate, getSearch } = makeNavigate();
    const { result, rerender } = renderHook(
      ({ search }: { search: CursorPaginationSearch }) =>
        useCursorPagination(search, navigate),
      { initialProps: { search: {} } }
    );

    act(() => result.current.goNext("cursor-a"));
    expect(getSearch().cursor).toBe("cursor-a");

    rerender({ search: getSearch() });
    expect(result.current.canGoBack).toBe(true);

    act(() => result.current.goNext("cursor-b"));
    rerender({ search: getSearch() });
    expect(getSearch().cursor).toBe("cursor-b");
    expect(result.current.canGoBack).toBe(true);
  });

  it("goPrev pops the back stack and restores the previous cursor", () => {
    const { navigate, getSearch } = makeNavigate();
    const { result, rerender } = renderHook(
      ({ search }: { search: CursorPaginationSearch }) =>
        useCursorPagination(search, navigate),
      { initialProps: { search: {} } }
    );

    act(() => result.current.goNext("cursor-a"));
    rerender({ search: getSearch() });
    act(() => result.current.goNext("cursor-b"));
    rerender({ search: getSearch() });

    act(() => result.current.goPrev());
    rerender({ search: getSearch() });
    expect(getSearch().cursor).toBe("cursor-a");
    expect(result.current.canGoBack).toBe(true);

    act(() => result.current.goPrev());
    rerender({ search: getSearch() });
    expect(getSearch().cursor).toBeUndefined();
    expect(result.current.canGoBack).toBe(false);
  });

  it("setPerPage resets the cursor and clears the back stack", () => {
    const { navigate, getSearch } = makeNavigate();
    const { result, rerender } = renderHook(
      ({ search }: { search: CursorPaginationSearch }) =>
        useCursorPagination(search, navigate),
      { initialProps: { search: {} } }
    );

    act(() => result.current.goNext("cursor-a"));
    rerender({ search: getSearch() });
    act(() => result.current.goNext("cursor-b"));
    rerender({ search: getSearch() });

    act(() => result.current.setPerPage(50));
    rerender({ search: getSearch() });

    expect(getSearch().perPage).toBe(50);
    expect(getSearch().cursor).toBeUndefined();
    expect(result.current.canGoBack).toBe(false);
  });

  it("reset clears the cursor and the back stack", () => {
    const { navigate, getSearch } = makeNavigate();
    const { result, rerender } = renderHook(
      ({ search }: { search: CursorPaginationSearch }) =>
        useCursorPagination(search, navigate),
      { initialProps: { search: {} } }
    );

    act(() => result.current.goNext("cursor-a"));
    rerender({ search: getSearch() });

    act(() => result.current.reset());
    rerender({ search: getSearch() });

    expect(getSearch().cursor).toBeUndefined();
    expect(result.current.canGoBack).toBe(false);
  });
});
