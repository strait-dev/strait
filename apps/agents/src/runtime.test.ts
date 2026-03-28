import { Effect } from "effect";
import { describe, expect, it } from "vitest";

import {
  buildRuntimeOutput,
  parseEnvelope,
  serializeOutputLine,
} from "./runtime";
import type { DispatchEnvelope } from "./types";

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

  it("terminates with a fail event for explicit runtime failures", async () => {
    const outputs = await Effect.runPromise(
      buildRuntimeOutput({
        ...baseEnvelope,
        payload: {
          _scenario: "fail",
          _error: "boom",
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
});
