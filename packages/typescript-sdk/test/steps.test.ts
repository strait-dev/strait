import { describe, expect, test } from "bun:test";

import { step, stepToApi } from "../src/authoring/steps";

describe("step builder", () => {
  test("step.job creates a job step with correct type and fields", () => {
    const s = step.job("validate", "job_validate");
    expect(s.type).toBe("job");
    expect(s.stepRef).toBe("validate");
    expect(s.jobId).toBe("job_validate");
  });

  test("step.job with all base options", () => {
    const s = step.job("charge", "job_charge", {
      dependsOn: ["validate"],
      onFailure: "fail_workflow",
      retryMaxAttempts: 3,
      retryBackoff: "exponential",
      retryInitialDelaySecs: 1,
      retryMaxDelaySecs: 60,
      timeoutSecsOverride: 120,
      outputTransform: "$.result",
      concurrencyKey: "order-{{ orderId }}",
      resourceClass: "large",
      condition: { status: "approved" },
      payload: { key: "value" },
    });

    expect(s.dependsOn).toEqual(["validate"]);
    expect(s.onFailure).toBe("fail_workflow");
    expect(s.retryMaxAttempts).toBe(3);
    expect(s.retryBackoff).toBe("exponential");
    expect(s.retryInitialDelaySecs).toBe(1);
    expect(s.retryMaxDelaySecs).toBe(60);
    expect(s.timeoutSecsOverride).toBe(120);
    expect(s.outputTransform).toBe("$.result");
    expect(s.concurrencyKey).toBe("order-{{ orderId }}");
    expect(s.resourceClass).toBe("large");
    expect(s.condition).toEqual({ status: "approved" });
    expect(s.payload).toEqual({ key: "value" });
  });

  test("step.approval creates an approval step", () => {
    const s = step.approval("review", {
      dependsOn: ["charge"],
      approvalTimeoutSecs: 3600,
      approvers: ["admin@example.com", "manager@example.com"],
    });

    expect(s.type).toBe("approval");
    expect(s.stepRef).toBe("review");
    expect(s.approvalTimeoutSecs).toBe(3600);
    expect(s.approvers).toEqual(["admin@example.com", "manager@example.com"]);
    expect(s.dependsOn).toEqual(["charge"]);
  });

  test("step.subWorkflow creates a sub-workflow step", () => {
    const s = step.subWorkflow("notify-all", "wf_notifications", {
      dependsOn: ["cooldown"],
      maxNestingDepth: 2,
    });

    expect(s.type).toBe("sub_workflow");
    expect(s.stepRef).toBe("notify-all");
    expect(s.subWorkflowId).toBe("wf_notifications");
    expect(s.maxNestingDepth).toBe(2);
  });

  test("step.waitForEvent creates a wait-for-event step", () => {
    const s = step.waitForEvent("shipping", "shipping.confirmed", {
      dependsOn: ["review"],
      eventTimeoutSecs: 86_400,
      eventNotifyUrl: "https://notify.example.com",
    });

    expect(s.type).toBe("wait_for_event");
    expect(s.stepRef).toBe("shipping");
    expect(s.eventKey).toBe("shipping.confirmed");
    expect(s.eventTimeoutSecs).toBe(86_400);
    expect(s.eventNotifyUrl).toBe("https://notify.example.com");
  });

  test("step.sleep creates a sleep step", () => {
    const s = step.sleep("cooldown", 60, { dependsOn: ["shipping"] });

    expect(s.type).toBe("sleep");
    expect(s.stepRef).toBe("cooldown");
    expect(s.sleepDurationSecs).toBe(60);
    expect(s.dependsOn).toEqual(["shipping"]);
  });
});

describe("stepToApi", () => {
  test("converts job step to snake_case API format", () => {
    const api = stepToApi(
      step.job("charge", "job_charge", {
        dependsOn: ["validate"],
        onFailure: "fail_workflow",
        retryMaxAttempts: 3,
        retryBackoff: "exponential",
      })
    );

    expect(api).toEqual({
      step_ref: "charge",
      type: "job",
      job_id: "job_charge",
      depends_on: ["validate"],
      on_failure: "fail_workflow",
      retry_max_attempts: 3,
      retry_backoff: "exponential",
    });
  });

  test("converts approval step to API format", () => {
    const api = stepToApi(
      step.approval("review", {
        approvalTimeoutSecs: 3600,
        approvers: ["admin@example.com"],
      })
    );

    expect(api).toEqual({
      step_ref: "review",
      type: "approval",
      approval_timeout_secs: 3600,
      approvers: ["admin@example.com"],
    });
  });

  test("converts sub-workflow step to API format", () => {
    const api = stepToApi(
      step.subWorkflow("notify", "wf_notify", { maxNestingDepth: 2 })
    );

    expect(api).toEqual({
      step_ref: "notify",
      type: "sub_workflow",
      sub_workflow_id: "wf_notify",
      max_nesting_depth: 2,
    });
  });

  test("converts wait-for-event step to API format", () => {
    const api = stepToApi(
      step.waitForEvent("wait", "order.shipped", {
        eventTimeoutSecs: 86_400,
        eventNotifyUrl: "https://notify.example.com",
      })
    );

    expect(api).toEqual({
      step_ref: "wait",
      type: "wait_for_event",
      event_key: "order.shipped",
      event_timeout_secs: 86_400,
      event_notify_url: "https://notify.example.com",
    });
  });

  test("converts sleep step to API format", () => {
    const api = stepToApi(step.sleep("pause", 30));

    expect(api).toEqual({
      step_ref: "pause",
      type: "sleep",
      sleep_duration_secs: 30,
    });
  });

  test("includes all base step options in API format", () => {
    const api = stepToApi(
      step.job("full", "job_full", {
        retryInitialDelaySecs: 1,
        retryMaxDelaySecs: 60,
        timeoutSecsOverride: 120,
        outputTransform: "$.data",
        concurrencyKey: "key-1",
        resourceClass: "medium",
      })
    );

    expect(api.retry_initial_delay_secs).toBe(1);
    expect(api.retry_max_delay_secs).toBe(60);
    expect(api.timeout_secs_override).toBe(120);
    expect(api.output_transform).toBe("$.data");
    expect(api.concurrency_key).toBe("key-1");
    expect(api.resource_class).toBe("medium");
  });

  test("minimal step only includes required fields", () => {
    const api = stepToApi(step.job("simple", "job_simple"));

    expect(api).toEqual({
      step_ref: "simple",
      type: "job",
      job_id: "job_simple",
    });
  });
});
