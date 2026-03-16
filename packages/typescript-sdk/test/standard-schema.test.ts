import { describe, expect, test } from "bun:test";
import type { StandardSchemaV1 } from "@standard-schema/spec";
import { resolveSchema } from "../src/authoring/utils";
import { defineJob, standardSchema } from "../src/index";
import {
  isStandardSchema,
  StandardSchemaValidationError,
} from "../src/schema-adapters/standard";
import { zodSchema } from "../src/schema-adapters/zod";

/** Helper: create a mock Standard Schema v1 string validator. */
const mockStringSchema = (): StandardSchemaV1<string, string> => ({
  "~standard": {
    version: 1,
    vendor: "test-lib",
    validate(value) {
      if (typeof value === "string") {
        return { value };
      }
      return { issues: [{ message: "Expected a string" }] };
    },
  },
});

/** Helper: create a mock Standard Schema v1 object validator. */
const mockObjectSchema = (): StandardSchemaV1<
  { sku: string },
  { sku: string }
> => ({
  "~standard": {
    version: 1,
    vendor: "test-lib",
    validate(value) {
      if (
        typeof value === "object" &&
        value !== null &&
        "sku" in value &&
        typeof (value as Record<string, unknown>).sku === "string"
      ) {
        return { value: value as { sku: string } };
      }
      return {
        issues: [{ message: "Expected object with string sku", path: ["sku"] }],
      };
    },
  },
});

/** Helper: create a mock async Standard Schema. */
const mockAsyncSchema = (): StandardSchemaV1<string, string> => ({
  "~standard": {
    version: 1,
    vendor: "async-lib",
    async validate(value) {
      await Promise.resolve();
      if (typeof value === "string") {
        return { value: value.toUpperCase() };
      }
      return { issues: [{ message: "Expected a string" }] };
    },
  },
});

describe("isStandardSchema", () => {
  test("returns true for valid Standard Schema v1 objects", () => {
    expect(isStandardSchema(mockStringSchema())).toBe(true);
    expect(isStandardSchema(mockObjectSchema())).toBe(true);
    expect(isStandardSchema(mockAsyncSchema())).toBe(true);
  });

  test("returns false for non-standard-schema objects", () => {
    expect(isStandardSchema(null)).toBe(false);
    expect(isStandardSchema(undefined)).toBe(false);
    expect(isStandardSchema(42)).toBe(false);
    expect(isStandardSchema("string")).toBe(false);
    expect(isStandardSchema({})).toBe(false);
    expect(isStandardSchema({ "~standard": null })).toBe(false);
    expect(isStandardSchema({ "~standard": { version: 2 } })).toBe(false);
    expect(
      isStandardSchema({ "~standard": { version: 1, validate: "not-fn" } })
    ).toBe(false);
  });

  test("returns false for SchemaAdapter objects", () => {
    const adapter = zodSchema({
      parse: (input: unknown) => input as string,
    });
    expect(isStandardSchema(adapter)).toBe(false);
  });
});

describe("standardSchema adapter", () => {
  test("wraps a sync Standard Schema and validates successfully", async () => {
    const adapter = standardSchema(mockStringSchema());

    expect(adapter.kind).toBe("standard:test-lib");

    const result = await adapter.parse("hello");
    expect(result).toBe("hello");
  });

  test("wraps an async Standard Schema and validates successfully", async () => {
    const adapter = standardSchema(mockAsyncSchema());

    expect(adapter.kind).toBe("standard:async-lib");

    const result = await adapter.parse("hello");
    expect(result).toBe("HELLO");
  });

  test("throws StandardSchemaValidationError on validation failure", async () => {
    const adapter = standardSchema(mockStringSchema());

    try {
      await adapter.parse(123);
      expect.unreachable("should have thrown");
    } catch (e) {
      expect(e).toBeInstanceOf(StandardSchemaValidationError);
      const err = e as StandardSchemaValidationError;
      expect(err.issues.length).toBe(1);
      expect(err.issues[0]?.message).toBe("Expected a string");
      expect(err.message).toContain("Expected a string");
    }
  });

  test("validation error includes path information", async () => {
    const adapter = standardSchema(mockObjectSchema());

    try {
      await adapter.parse({ sku: 123 });
      expect.unreachable("should have thrown");
    } catch (e) {
      const err = e as StandardSchemaValidationError;
      expect(err.issues[0]?.path).toEqual(["sku"]);
    }
  });

  test("object schema validates correctly", async () => {
    const adapter = standardSchema(mockObjectSchema());
    const result = await adapter.parse({ sku: "ABC-123" });
    expect(result).toEqual({ sku: "ABC-123" });
  });
});

describe("resolveSchema", () => {
  test("passes through SchemaAdapter unchanged", () => {
    const adapter = zodSchema({
      parse: (input: unknown) => input as string,
    });
    const resolved = resolveSchema(adapter);
    expect(resolved).toBe(adapter);
  });

  test("auto-wraps Standard Schema v1 objects", async () => {
    const stdSchema = mockStringSchema();
    const resolved = resolveSchema(stdSchema);

    expect(resolved.kind).toBe("standard:test-lib");
    const result = await resolved.parse("test");
    expect(result).toBe("test");
  });

  test("throws for invalid schema input", () => {
    expect(() => resolveSchema({} as never)).toThrow("Invalid schema");
  });
});

describe("defineJob with Standard Schema directly", () => {
  test("accepts Standard Schema v1 in schema field", async () => {
    const job = defineJob({
      name: "Standard Job",
      slug: "standard-job",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockObjectSchema(),
    });

    expect(job.kind).toBe("job");

    let capturedPayload: unknown;

    await job.register({
      createJob: () => Promise.resolve({ id: "job_1" }),
      triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
    });

    await job.trigger(
      {
        createJob: () => Promise.resolve({ id: "job_1" }),
        triggerJob: (input) => {
          capturedPayload = (input.body as Record<string, unknown>)?.payload;
          return Promise.resolve({ id: "run_1", status: "queued" });
        },
      },
      { payload: { sku: "ABC-123" } }
    );

    expect(capturedPayload).toEqual({ sku: "ABC-123" });
  });

  test("Standard Schema validation errors propagate on trigger", async () => {
    const job = defineJob({
      name: "Strict Job",
      slug: "strict-job",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockStringSchema(),
    });

    await job.register({
      createJob: () => Promise.resolve({ id: "job_1" }),
      triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
    });

    await expect(
      job.trigger(
        {
          createJob: () => Promise.resolve({ id: "job_1" }),
          triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
        },
        { payload: 999 as unknown as string }
      )
    ).rejects.toThrow(StandardSchemaValidationError);
  });

  test("still works with explicit SchemaAdapter", async () => {
    const job = defineJob({
      name: "Adapter Job",
      slug: "adapter-job",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: zodSchema({
        parse: (input: unknown) => input as { id: string },
      }),
    });

    await job.register({
      createJob: () => Promise.resolve({ id: "job_1" }),
      triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
    });

    const run = await job.trigger(
      {
        createJob: () => Promise.resolve({ id: "job_1" }),
        triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
      },
      { payload: { id: "test" } }
    );

    expect(run.id).toBe("run_1");
  });

  test("async Standard Schema works end-to-end", async () => {
    const job = defineJob({
      name: "Async Job",
      slug: "async-job",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockAsyncSchema(),
    });

    let capturedPayload: unknown;

    await job.register({
      createJob: () => Promise.resolve({ id: "job_1" }),
      triggerJob: () => Promise.resolve({ id: "run_1", status: "queued" }),
    });

    await job.trigger(
      {
        createJob: () => Promise.resolve({ id: "job_1" }),
        triggerJob: (input) => {
          capturedPayload = (input.body as Record<string, unknown>)?.payload;
          return Promise.resolve({ id: "run_1", status: "queued" });
        },
      },
      { payload: "hello" }
    );

    // Async schema transforms to uppercase
    expect(capturedPayload).toBe("HELLO");
  });
});
