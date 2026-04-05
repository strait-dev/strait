import { describe, expect, it } from "vitest";

import { createSandboxTool, validateJsonSchema } from "./sandbox";

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
      executionMode: "sandboxed",
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

  it("supports outbound-worker metadata for network-constrained tools", () => {
    const tool = createSandboxTool({
      name: "external-llm",
      mode: "outbound-worker",
      networkClass: "restricted",
      outboundPolicyTag: "llm-egress",
      runtime: "cloudflare-workers",
      execute: () => ({ ok: true }),
    });

    expect(tool.sandbox).toEqual({
      executionMode: "sandboxed",
      mode: "outbound-worker",
      image: undefined,
      networkClass: "restricted",
      outboundPolicyTag: "llm-egress",
      runtime: "cloudflare-workers",
      timeoutMs: undefined,
    });
  });

  it("validates output against schema when provided", async () => {
    const tool = createSandboxTool({
      name: "typed-tool",
      outputSchema: {
        type: "object",
        required: ["status"],
        properties: {
          status: { type: "string" },
          count: { type: "number" },
        },
      },
      execute: () => ({ status: "ok", count: 42 }),
    });

    await expect(tool.execute(null)).resolves.toEqual({
      status: "ok",
      count: 42,
    });
  });

  it("throws on output schema validation failure", async () => {
    const tool = createSandboxTool({
      name: "bad-output-tool",
      outputSchema: {
        type: "object",
        required: ["name"],
        properties: {
          name: { type: "string" },
        },
      },
      execute: () => ({ value: 123 }), // missing "name"
    });

    await expect(tool.execute(null)).rejects.toThrow(
      "output validation failed"
    );
  });

  it("skips validation when no outputSchema", async () => {
    const tool = createSandboxTool({
      name: "no-schema",
      execute: () => "anything goes",
    });

    await expect(tool.execute(null)).resolves.toBe("anything goes");
  });
});

describe("validateJsonSchema", () => {
  it("validates type: string", () => {
    expect(validateJsonSchema("hello", { type: "string" })).toBeNull();
    expect(validateJsonSchema(42, { type: "string" })).toBe(
      "expected string, got number"
    );
  });

  it("validates type: object with required", () => {
    expect(
      validateJsonSchema({ a: 1 }, { type: "object", required: ["a"] })
    ).toBeNull();
    expect(
      validateJsonSchema({ b: 1 }, { type: "object", required: ["a"] })
    ).toBe("missing required property: a");
  });

  it("validates type: array with items", () => {
    expect(
      validateJsonSchema([1, 2], { type: "array", items: { type: "number" } })
    ).toBeNull();
    expect(
      validateJsonSchema([1, "x"], { type: "array", items: { type: "number" } })
    ).toBe("item[1]: expected number, got string");
  });

  it("validates enum", () => {
    expect(validateJsonSchema("a", { enum: ["a", "b"] })).toBeNull();
    expect(validateJsonSchema("c", { enum: ["a", "b"] })).toContain(
      "not in enum"
    );
  });
});
