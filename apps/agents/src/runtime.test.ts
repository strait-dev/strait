import { Effect } from "effect";
import fc from "fast-check";
import { describe, expect, it } from "vitest";

import {
  buildRuntimeOutput,
  parseEnvelope,
  serializeOutputLine,
} from "./runtime";
import type { DynamicWorkerLoader } from "./sandbox";
import type { DispatchEnvelope, RuntimeOutputLine } from "./types";

const baseEnvelope: DispatchEnvelope = {
  version: "v1",
  run: {
    id: "run-1",
    project_id: "proj-1",
    attempt: 1,
    timeout_secs: 30,
  },
  agent: {
    id: "agent-1",
    slug: "support-agent",
    model: "gpt-5.4",
  },
  deployment: {
    id: "deployment-1",
    version: 1,
    provider: "local_stub",
  },
  callback: {
    base_url: "http://localhost:8080",
    run_id: "run-1",
    run_token: "token-1",
  },
};

const invalidPlannerPayloadError = /workerAgentIds must be an array/i;
const nonEmptyStringArbitrary = fc
  .string({ minLength: 1 })
  .filter((value) => value.trim().length > 0);

function getCompletionResult(
  outputs: readonly RuntimeOutputLine[]
): Record<string, unknown> {
  const completion = outputs.findLast(
    (output) => output.kind === "event" && output.event.type === "complete"
  );
  if (
    completion?.kind !== "event" ||
    completion.event.type !== "complete" ||
    completion.event.result == null ||
    typeof completion.event.result !== "object" ||
    Array.isArray(completion.event.result)
  ) {
    throw new Error("missing completion result");
  }
  return completion.event.result as Record<string, unknown>;
}

describe("agents runtime", () => {
  it("parses and validates a dispatch envelope", async () => {
    const envelope = await Effect.runPromise(
      parseEnvelope(JSON.stringify(baseEnvelope))
    );

    expect(envelope.agent.slug).toBe("support-agent");
  });

  it("builds the success scenario event stream", async () => {
    const outputs = await Effect.runPromise(buildRuntimeOutput(baseEnvelope));

    expect(outputs.map(serializeOutputLine)).toHaveLength(6);
    expect(outputs.at(-1)).toMatchObject({
      kind: "event",
      event: {
        type: "complete",
      },
    });
  });

  it("emits a raw invalid json line for the invalid_json scenario", async () => {
    const outputs = await Effect.runPromise(
      buildRuntimeOutput({
        ...baseEnvelope,
        payload: {
          _scenario: "invalid_json",
        },
      })
    );

    expect(outputs).toEqual([
      {
        kind: "raw",
        line: "{not-json}",
      },
    ]);
  });

  it("emits duplicate checkpoints when requested", async () => {
    const outputs = await Effect.runPromise(
      buildRuntimeOutput({
        ...baseEnvelope,
        payload: {
          _scenario: "duplicate_checkpoint",
        },
      })
    );

    expect(
      outputs.filter(
        (output) =>
          output.kind === "event" && output.event.type === "checkpoint"
      )
    ).toHaveLength(2);
  });

  it("captures blocked dynamic worker requests as sandbox tool calls", async () => {
    const loader: DynamicWorkerLoader = {
      get: () => ({
        fetch: async () =>
          new Response(
            JSON.stringify({
              body_preview: '{"error":"sandbox_request_blocked"}',
              outbound_reason: "host_not_allowlisted",
              policy_tag: "llm-egress",
              status_code: 403,
              url: "https://blocked.example.com",
            }),
            {
              headers: {
                "content-type": "application/json; charset=utf-8",
              },
              status: 403,
            }
          ),
      }),
    };

    const outputs = await Effect.runPromise(
      buildRuntimeOutput(
        {
          ...baseEnvelope,
          deployment: {
            ...baseEnvelope.deployment,
            sandbox_policy: {
              allow_hosts: ["api.openai.com"],
              default_action: "deny",
              mode: "dynamic_worker",
              network_class: "sandbox",
              policy_tag: "llm-egress",
            },
          },
          payload: {
            _network_url: "https://blocked.example.com",
          },
        },
        {
          dynamicWorkerLoader: loader,
          fetch: async () => new Response("unexpected", { status: 500 }),
        }
      )
    );

    expect(outputs).toContainEqual({
      kind: "event",
      event: {
        duration_ms: 5,
        input: {
          network_class: "sandbox",
          policy_tag: "llm-egress",
          sandbox_mode: "dynamic_worker",
          url: "https://blocked.example.com",
        },
        output: {
          body_preview: '{"error":"sandbox_request_blocked"}',
          outbound_reason: "host_not_allowlisted",
          sandbox_executor: "dynamic_worker",
          status_code: 403,
          url: "https://blocked.example.com",
        },
        status: "blocked",
        tool_name: "sandbox.fetch",
        type: "tool_call",
      },
    });
  });

  it("keeps outbound worker mode as a compatibility path", async () => {
    const outputs = await Effect.runPromise(
      buildRuntimeOutput(
        {
          ...baseEnvelope,
          deployment: {
            ...baseEnvelope.deployment,
            sandbox_policy: {
              allow_hosts: ["api.openai.com"],
              default_action: "deny",
              mode: "outbound_worker",
              network_class: "restricted",
              policy_tag: "llm-egress",
            },
          },
          payload: {
            _network_url: "https://blocked.example.com",
          },
        },
        {
          fetch: async () =>
            new Response('{"error":"blocked"}', {
              headers: {
                "x-strait-outbound-reason": "host_not_allowlisted",
                "x-strait-outbound-status": "blocked",
              },
              status: 403,
            }),
        }
      )
    );

    expect(outputs).toContainEqual({
      kind: "event",
      event: {
        duration_ms: 5,
        input: {
          network_class: "restricted",
          policy_tag: "llm-egress",
          sandbox_mode: "outbound_worker",
          url: "https://blocked.example.com",
        },
        output: {
          body_preview: '{"error":"blocked"}',
          outbound_reason: "host_not_allowlisted",
          sandbox_executor: "outbound_worker",
          status_code: 403,
          url: "https://blocked.example.com",
        },
        status: "blocked",
        tool_name: "sandbox.fetch",
        type: "tool_call",
      },
    });
  });

  it("terminates with a fail event for explicit runtime failures", async () => {
    const outputs = await Effect.runPromise(
      buildRuntimeOutput({
        ...baseEnvelope,
        payload: {
          _error: "boom",
          _scenario: "fail",
        },
      })
    );

    expect(outputs.at(-1)).toMatchObject({
      kind: "event",
      event: {
        type: "fail",
        error: "boom",
      },
    });
  });

  it("builds a dynamic planner result for workflow fan-out", async () => {
    const outputs = await Effect.runPromise(
      buildRuntimeOutput({
        ...baseEnvelope,
        agent: {
          ...baseEnvelope.agent,
          slug: "planner",
        },
        payload: {
          _mode: "dynamic_planner",
          summaryStyle: "incident-brief",
          topic: "billing outage",
          workerAgentIds: ["agent-logs", "agent-metrics", "agent-deployments"],
        },
      })
    );

    const result = getCompletionResult(outputs);
    expect(result.plan_summary).toBe("planned 3 dynamic worker steps");
    expect(result.dynamic_steps).toEqual([
      {
        agent_id: "agent-logs",
        depends_on: ["planner"],
        payload: {
          lens: "track-1",
          topic: "billing outage",
        },
        step_ref: "worker-1",
      },
      {
        agent_id: "agent-metrics",
        depends_on: ["planner"],
        payload: {
          lens: "track-2",
          topic: "billing outage",
        },
        step_ref: "worker-2",
      },
      {
        agent_id: "agent-deployments",
        depends_on: ["planner"],
        payload: {
          lens: "track-3",
          topic: "billing outage",
        },
        step_ref: "worker-3",
      },
      {
        agent_id: "agent-synthesizer",
        depends_on: ["worker-1", "worker-2", "worker-3"],
        payload: {
          summary_style: "incident-brief",
          topic: "billing outage",
        },
        step_ref: "synthesis",
      },
    ]);
  });

  it("builds worker and synthesizer local runtime outputs", async () => {
    const workerOutputs = await Effect.runPromise(
      buildRuntimeOutput({
        ...baseEnvelope,
        payload: {
          _mode: "worker",
          lens: "logs",
          topic: "runtime failures",
        },
      })
    );
    const synthOutputs = await Effect.runPromise(
      buildRuntimeOutput({
        ...baseEnvelope,
        payload: {
          _mode: "synthesizer",
          summaryStyle: "brief",
          topic: "runtime failures",
        },
      })
    );

    expect(getCompletionResult(workerOutputs)).toMatchObject({
      finding: "Investigated runtime failures via logs.",
    });
    expect(getCompletionResult(synthOutputs)).toMatchObject({
      summary: "Prepared a brief summary for runtime failures.",
    });
  });

  it("rejects invalid planner payloads", async () => {
    await expect(
      Effect.runPromise(
        buildRuntimeOutput({
          ...baseEnvelope,
          payload: {
            _mode: "dynamic_planner",
            workerAgentIds: "not-an-array",
          },
        })
      )
    ).rejects.toThrow(invalidPlannerPayloadError);
  });

  it("produces stable worker fan-out shapes for arbitrary worker lists", () => {
    fc.assert(
      fc.property(
        fc.uniqueArray(nonEmptyStringArbitrary, {
          minLength: 1,
          maxLength: 8,
        }),
        (workerAgentIds) => {
          const outputs = Effect.runSync(
            buildRuntimeOutput({
              ...baseEnvelope,
              agent: {
                ...baseEnvelope.agent,
                slug: "planner",
              },
              payload: {
                _mode: "dynamic_planner",
                topic: "runtime failures",
                workerAgentIds,
              },
            })
          );

          const result = getCompletionResult(outputs);
          const dynamicSteps = result.dynamic_steps as Record<
            string,
            unknown
          >[];
          expect(dynamicSteps).toHaveLength(workerAgentIds.length + 1);
          expect(
            new Set(
              dynamicSteps
                .slice(0, Math.max(dynamicSteps.length - 1, 0))
                .map((step) => String(step.step_ref))
            ).size
          ).toBe(workerAgentIds.length);
        }
      ),
      { numRuns: 50 }
    );
  });
});
