import { describe, expect, test } from "bun:test";

import { defineEvent } from "../src/authoring/event";
import { zodSchema } from "../src/index";

describe("defineEvent", () => {
  test("creates definition with correct key", () => {
    const schema = zodSchema({
      parse: (input: unknown) => input as { orderId: string },
      toJSON: () => ({ type: "object" }),
    });

    const event = defineEvent("order.completed", schema);

    expect(event.key).toBe("order.completed");
    expect(event.schema).toBeDefined();
    expect(event.parse).toBeInstanceOf(Function);
  });

  test("parse validates payloads correctly", async () => {
    const schema = zodSchema({
      parse: (input: unknown) => {
        const obj = input as { amount: number };
        if (typeof obj.amount !== "number") {
          throw new Error("Invalid");
        }
        return obj;
      },
      toJSON: () => ({ type: "object" }),
    });

    const event = defineEvent("payment.received", schema);

    const result = await event.parse({ amount: 100 });
    expect(result).toEqual({ amount: 100 });
  });

  test("parse rejects invalid payloads", async () => {
    const schema = zodSchema({
      parse: (input: unknown) => {
        const obj = input as { amount: number };
        if (typeof obj.amount !== "number") {
          throw new Error("amount must be number");
        }
        return obj;
      },
      toJSON: () => ({ type: "object" }),
    });

    const event = defineEvent("payment.received", schema);

    await expect(event.parse({ amount: "not-a-number" })).rejects.toThrow(
      "amount must be number"
    );
  });
});
