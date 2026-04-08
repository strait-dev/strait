import { describe, expect, it } from "vitest";
import {
  notifyCursorPageLimit,
  resolveNotifyNextCursor,
} from "./notify-cursor";

describe("notify cursor helpers", () => {
  it("returns undefined when page is not full", () => {
    const items = [
      { created_at: "2026-01-01T00:00:00Z" },
      { created_at: "2026-01-02T00:00:00Z" },
    ];

    expect(resolveNotifyNextCursor(items, 10)).toBeUndefined();
  });

  it("returns last created_at when page reaches limit", () => {
    const items = Array.from({ length: notifyCursorPageLimit }, (_, index) => ({
      created_at: `2026-01-01T00:00:${String(index).padStart(2, "0")}Z`,
    }));

    expect(resolveNotifyNextCursor(items)).toBe(items.at(-1)?.created_at);
  });
});
