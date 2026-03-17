import { describe, expect, test } from "bun:test";

import { step, stepToApi } from "../src/authoring/steps";

describe("step.ai", () => {
  test("returns JobStep with LLM-tuned defaults", () => {
    const s = step.ai("llm-process", "job_llm");

    expect(s.type).toBe("job");
    expect(s.stepRef).toBe("llm-process");
    expect(s.jobId).toBe("job_llm");
    expect(s.timeoutSecsOverride).toBe(600);
    expect(s.retryMaxAttempts).toBe(5);
    expect(s.retryBackoff).toBe("exponential");
    expect(s.retryInitialDelaySecs).toBe(2);
    expect(s.retryMaxDelaySecs).toBe(120);
    expect(s.resourceClass).toBe("large");
  });

  test("user overrides take precedence", () => {
    const s = step.ai("llm-process", "job_llm", {
      timeoutSecsOverride: 300,
      retryMaxAttempts: 2,
      resourceClass: "small",
    });

    expect(s.timeoutSecsOverride).toBe(300);
    expect(s.retryMaxAttempts).toBe(2);
    expect(s.resourceClass).toBe("small");
  });

  test("converts to API format via stepToApi", () => {
    const s = step.ai("llm-process", "job_llm", {
      dependsOn: ["validate"],
    });

    const api = stepToApi(s);

    expect(api.step_ref).toBe("llm-process");
    expect(api.type).toBe("job");
    expect(api.job_id).toBe("job_llm");
    expect(api.timeout_secs_override).toBe(600);
    expect(api.retry_max_attempts).toBe(5);
    expect(api.retry_backoff).toBe("exponential");
    expect(api.depends_on).toEqual(["validate"]);
    expect(api.resource_class).toBe("large");
  });

  test("preserves step.job behavior for comparison", () => {
    const jobStep = step.job("basic", "job_basic");
    const aiStep = step.ai("ai", "job_ai");

    // Job step has no defaults
    expect(jobStep.timeoutSecsOverride).toBeUndefined();
    expect(jobStep.retryMaxAttempts).toBeUndefined();

    // AI step has defaults
    expect(aiStep.timeoutSecsOverride).toBe(600);
    expect(aiStep.retryMaxAttempts).toBe(5);
  });
});
