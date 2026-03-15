import { describe, expect, test } from "bun:test";

import { defineJob, zodSchema } from "../src/index";

const mockSchema = zodSchema({
  parse: (input: unknown) => input as { sku: string },
  toJSON: () => ({ type: "object" }),
});

const mockClient = {
  createJob: (input: { readonly body: unknown }) =>
    Promise.resolve({ id: "job_1", ...(input.body as object) }),
  triggerJob: (input: { readonly body?: unknown }) =>
    Promise.resolve({ id: "run_1", status: "queued" }),
};

describe("DefineJobOptions → toRegistrationBody mapping", () => {
  test("maps all basic fields to snake_case", () => {
    const job = defineJob({
      name: "Test Job",
      slug: "test-job",
      endpointUrl: "https://example.com/jobs/test",
      projectId: "proj_1",
      schema: mockSchema,
      description: "A test job",
      groupId: "grp_1",
      tags: { team: "backend" },
      environmentId: "env_prod",
    });

    const body = job.toRegistrationBody();
    expect(body.project_id).toBe("proj_1");
    expect(body.name).toBe("Test Job");
    expect(body.slug).toBe("test-job");
    expect(body.endpoint_url).toBe("https://example.com/jobs/test");
    expect(body.description).toBe("A test job");
    expect(body.group_id).toBe("grp_1");
    expect(body.tags).toEqual({ team: "backend" });
    expect(body.environment_id).toBe("env_prod");
  });

  test("maps cron and timezone fields", () => {
    const job = defineJob({
      name: "Cron Job",
      slug: "cron-job",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockSchema,
      cron: "*/5 * * * *",
      timezone: "America/New_York",
      executionWindowCron: "0 9-17 * * 1-5",
    });

    const body = job.toRegistrationBody();
    expect(body.cron).toBe("*/5 * * * *");
    expect(body.timezone).toBe("America/New_York");
    expect(body.execution_window_cron).toBe("0 9-17 * * 1-5");
  });

  test("maps concurrency and rate limiting fields", () => {
    const job = defineJob({
      name: "Rate Limited",
      slug: "rate-limited",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockSchema,
      maxConcurrency: 5,
      rateLimitMax: 100,
      rateLimitWindowSecs: 60,
    });

    const body = job.toRegistrationBody();
    expect(body.max_concurrency).toBe(5);
    expect(body.rate_limit_max).toBe(100);
    expect(body.rate_limit_window_secs).toBe(60);
  });

  test("maps retry fields", () => {
    const job = defineJob({
      name: "Retry Job",
      slug: "retry-job",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockSchema,
      maxAttempts: 5,
      retryStrategy: "exponential",
      retryDelaysSecs: [1, 2, 4, 8],
    });

    const body = job.toRegistrationBody();
    expect(body.max_attempts).toBe(5);
    expect(body.retry_strategy).toBe("exponential");
    expect(body.retry_delays_secs).toEqual([1, 2, 4, 8]);
  });

  test("maps timeout and TTL fields", () => {
    const job = defineJob({
      name: "Timeout Job",
      slug: "timeout-job",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockSchema,
      timeoutSecs: 300,
      runTtlSecs: 86400,
      dedupWindowSecs: 60,
    });

    const body = job.toRegistrationBody();
    expect(body.timeout_secs).toBe(300);
    expect(body.run_ttl_secs).toBe(86400);
    expect(body.dedup_window_secs).toBe(60);
  });

  test("maps webhook fields", () => {
    const job = defineJob({
      name: "Webhook Job",
      slug: "webhook-job",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockSchema,
      webhookUrl: "https://hooks.example.com/notify",
      webhookSecret: "whsec_123",
      fallbackEndpointUrl: "https://fallback.example.com",
    });

    const body = job.toRegistrationBody();
    expect(body.webhook_url).toBe("https://hooks.example.com/notify");
    expect(body.webhook_secret).toBe("whsec_123");
    expect(body.fallback_endpoint_url).toBe("https://fallback.example.com");
  });

  test("includes payload_schema from schema adapter", () => {
    const job = defineJob({
      name: "Schema Job",
      slug: "schema-job",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockSchema,
    });

    const body = job.toRegistrationBody();
    expect(body.payload_schema).toEqual({ type: "object" });
  });

  test("omits undefined optional fields", () => {
    const job = defineJob({
      name: "Minimal",
      slug: "minimal",
      endpointUrl: "https://example.com",
      projectId: "proj_1",
      schema: mockSchema,
    });

    const body = job.toRegistrationBody();
    expect(body.cron).toBeUndefined();
    expect(body.max_concurrency).toBeUndefined();
    expect(body.max_attempts).toBeUndefined();
    expect(body.timeout_secs).toBeUndefined();
    expect(body.webhook_url).toBeUndefined();
    expect(body.tags).toBeUndefined();
  });

  test("register-time projectId overrides definition-time", () => {
    const job = defineJob({
      name: "Override",
      slug: "override",
      endpointUrl: "https://example.com",
      projectId: "proj_default",
      schema: mockSchema,
    });

    const body = job.toRegistrationBody("proj_override");
    expect(body.project_id).toBe("proj_override");
  });

  test("throws when no projectId is provided", () => {
    const job = defineJob({
      name: "No Project",
      slug: "no-project",
      endpointUrl: "https://example.com",
      schema: mockSchema,
    });

    expect(() => job.toRegistrationBody()).toThrow("requires projectId");
  });
});
