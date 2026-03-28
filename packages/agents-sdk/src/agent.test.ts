import { describe, expect, it } from "vitest";

import { agent } from "./agent";

describe("agent", () => {
  it("normalizes budget strings and preserves handler aliases", async () => {
    const definition = agent({
      name: "Research Assistant",
      model: "gpt-4.1",
      budget: "$5.00",
      handler: async () => ({
        ok: true,
      }),
    });

    expect(definition.budget).toEqual({
      maxCostMicrousd: 5_000_000,
    });
    await expect(definition.run?.({} as never, null as never)).resolves.toEqual(
      {
        ok: true,
      }
    );
  });
});
