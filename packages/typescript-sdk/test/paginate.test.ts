import { describe, expect, test } from "bun:test";

import { collectAll, paginate } from "../src/composition/paginate";

describe("paginate", () => {
  test("yields all items from a single page", async () => {
    const items = await collectAll(
      paginate(() =>
        Promise.resolve({
          data: [{ id: "1" }, { id: "2" }, { id: "3" }],
        })
      )
    );

    expect(items).toEqual([{ id: "1" }, { id: "2" }, { id: "3" }]);
  });

  test("paginates through multiple pages using next_cursor", async () => {
    let callCount = 0;

    const items = await collectAll(
      paginate((query) => {
        callCount += 1;
        if (!query.cursor) {
          return Promise.resolve({
            data: [{ id: "1" }, { id: "2" }],
            next_cursor: "cursor_2",
            has_more: true,
          });
        }
        if (query.cursor === "cursor_2") {
          return Promise.resolve({
            data: [{ id: "3" }, { id: "4" }],
            next_cursor: "cursor_3",
            has_more: true,
          });
        }
        return Promise.resolve({
          data: [{ id: "5" }],
          has_more: false,
        });
      })
    );

    expect(items).toEqual([
      { id: "1" },
      { id: "2" },
      { id: "3" },
      { id: "4" },
      { id: "5" },
    ]);
    expect(callCount).toBe(3);
  });

  test("handles empty response", async () => {
    const items = await collectAll(
      paginate(() => Promise.resolve({ data: [] }))
    );

    expect(items).toEqual([]);
  });

  test("supports items field instead of data", async () => {
    const items = await collectAll(
      paginate(() =>
        Promise.resolve({
          items: [{ id: "a" }, { id: "b" }],
        })
      )
    );

    expect(items).toEqual([{ id: "a" }, { id: "b" }]);
  });

  test("supports camelCase nextCursor and hasMore", async () => {
    let callCount = 0;

    const items = await collectAll(
      paginate(() => {
        callCount += 1;
        if (callCount === 1) {
          return Promise.resolve({
            data: [{ id: "1" }],
            nextCursor: "c2",
            hasMore: true,
          });
        }
        return Promise.resolve({
          data: [{ id: "2" }],
          hasMore: false,
        });
      })
    );

    expect(items).toEqual([{ id: "1" }, { id: "2" }]);
  });

  test("stops when no next_cursor is returned", async () => {
    let callCount = 0;

    const items = await collectAll(
      paginate(() => {
        callCount += 1;
        return Promise.resolve({
          data: [{ id: String(callCount) }],
        });
      })
    );

    expect(items).toEqual([{ id: "1" }]);
    expect(callCount).toBe(1);
  });

  test("collectAll works with paginate generator", async () => {
    const result = await collectAll(
      paginate(() =>
        Promise.resolve({
          data: [1, 2, 3],
        })
      )
    );

    expect(result).toEqual([1, 2, 3]);
  });

  test("passes limit option to list function", async () => {
    let capturedQuery: Record<string, unknown> = {};

    await collectAll(
      paginate(
        (query) => {
          capturedQuery = query;
          return Promise.resolve({ data: [] });
        },
        { limit: 50 }
      )
    );

    expect(capturedQuery.limit).toBe(50);
  });
});
