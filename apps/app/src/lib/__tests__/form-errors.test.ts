import { describe, expect, it } from "vitest";
import { formatFieldErrors } from "@/lib/form-errors";

describe("formatFieldErrors", () => {
  it("joins string array with comma separator", () => {
    expect(formatFieldErrors(["too short", "invalid"])).toBe(
      "too short, invalid"
    );
  });

  it("extracts message from objects with a message property", () => {
    expect(
      formatFieldErrors([{ message: "required" }, { message: "too long" }])
    ).toBe("required, too long");
  });

  it("handles mixed array of strings, message objects, and unknown types", () => {
    expect(formatFieldErrors(["bad input", { message: "required" }, 42])).toBe(
      "bad input, required, 42"
    );
  });

  it("returns empty string for empty array", () => {
    expect(formatFieldErrors([])).toBe("");
  });

  it("uses String() for objects without message property", () => {
    expect(formatFieldErrors([{ code: "ERR" }])).toBe("[object Object]");
  });

  it("handles single-element array without trailing comma", () => {
    expect(formatFieldErrors(["only one"])).toBe("only one");
  });
});
