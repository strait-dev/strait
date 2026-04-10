import { describe, expect, it } from "vitest";
import { isNotifyScopedKey, parseNotifyJSONRecord } from "./notify-form";

describe("notify form helpers", () => {
  it("validates scoped keys", () => {
    expect(isNotifyScopedKey("workflow.approvals")).toBe(true);
    expect(isNotifyScopedKey("ops-alerts_critical")).toBe(true);
    expect(isNotifyScopedKey("bad key")).toBe(false);
  });

  it("rejects keys that start or end with separators", () => {
    expect(isNotifyScopedKey(".key")).toBe(false);
    expect(isNotifyScopedKey("key.")).toBe(false);
    expect(isNotifyScopedKey("-key")).toBe(false);
    expect(isNotifyScopedKey("key-")).toBe(false);
    expect(isNotifyScopedKey("_key")).toBe(false);
    expect(isNotifyScopedKey("key_")).toBe(false);
    expect(isNotifyScopedKey("..double")).toBe(false);
  });

  it("accepts single alphanumeric character keys", () => {
    expect(isNotifyScopedKey("a")).toBe(true);
    expect(isNotifyScopedKey("Z")).toBe(true);
    expect(isNotifyScopedKey("9")).toBe(true);
  });

  it("parses valid JSON object", () => {
    const result = parseNotifyJSONRecord('{"subject":{"value":"hello"}}');

    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.data).toEqual({ subject: { value: "hello" } });
    }
  });

  it("rejects invalid json and invalid shapes", () => {
    expect(parseNotifyJSONRecord("not-json")).toEqual({
      ok: false,
      reason: "invalid_json",
    });
    expect(parseNotifyJSONRecord('["array"]')).toEqual({
      ok: false,
      reason: "invalid_shape",
    });
  });
});
