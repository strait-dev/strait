import { describe, expect, it } from "vitest";

import { createSandboxTool } from "./sandbox";

describe("createSandboxTool", () => {
  it("builds a local dynamic-worker tool definition", async () => {
    const tool = createSandboxTool({
      name: "web-search",
      description: "Query a search index inside the sandbox",
      image: "ghcr.io/strait/tools/search:latest",
      timeoutMs: 30_000,
      execute: async (input: { query: string }) => ({
        ok: true,
        query: input.query,
      }),
    });

    expect(tool.name).toBe("web-search");
    expect(tool.sandbox).toEqual({
      mode: "dynamic-worker",
      image: "ghcr.io/strait/tools/search:latest",
      timeoutMs: 30_000,
    });
    await expect(tool.execute({ query: "durable execution" })).resolves.toEqual(
      {
        ok: true,
        query: "durable execution",
      }
    );
  });
});
