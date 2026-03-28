import { describe, expect, it } from "vitest";

import {
  defineEvalSuite,
  expectArrayMinLength,
  expectPathEquals,
  expectTextContains,
  runEvalSuite,
} from "./evals";
import { StraitSDKError } from "./errors";

describe("defineEvalSuite", () => {
  it("normalizes tags and rejects duplicate case names", () => {
    expect(() =>
      defineEvalSuite({
        name: "Agent quality",
        cases: [
          {
            name: "happy-path",
            input: {},
          },
          {
            name: "happy-path",
            input: {},
          },
        ],
      })
    ).toThrowError(new StraitSDKError("duplicate eval case name: happy-path"));
  });

  it("requires at least one case", () => {
    expect(() =>
      defineEvalSuite({
        name: "Empty",
        cases: [],
      })
    ).toThrowError(new StraitSDKError("eval suite requires at least one case"));
  });
});

describe("runEvalSuite", () => {
  it("collects assertion and judge results", async () => {
    const result = await runEvalSuite<
      { topic: string },
      { actions: string[]; summary: string }
    >(
      {
        name: "Incident triage",
        cases: [
          {
            name: "returns actionable output",
            input: {
              topic: "billing regression",
            },
            assertions: [
              expectTextContains("summary mentions billing", (output) => output.summary, [
                "billing",
              ]),
              expectArrayMinLength("actions included", (output) => output.actions, 2),
            ],
            judge: {
              name: "manual-judge",
              judge: async (output) => ({
                passed: output.actions.every((action) => action.length > 8),
                score: 0.91,
              }),
            },
          },
        ],
      },
      async (input) => ({
        summary: `Investigated ${input.topic} and proposed mitigations.`,
        actions: ["Roll back the latest deployment", "Validate queue latency"],
      })
    );

    expect(result.passed).toBe(1);
    expect(result.failed).toBe(0);
    expect(result.cases[0]?.judge).toEqual({
      name: "manual-judge",
      passed: true,
      score: 0.91,
    });
    expect(result.cases[0]?.assertions).toEqual([
      {
        name: "summary mentions billing",
        message: "expected selected text to contain billing",
        passed: true,
      },
      {
        name: "actions included",
        message: "expected selected array length to be at least 2",
        passed: true,
      },
    ]);
  });

  it("surfaces executor failures without aborting the suite", async () => {
    const result = await runEvalSuite<{ id: string }, { status: string }>(
      {
        name: "Failure handling",
        cases: [
          {
            name: "executor fails",
            input: { id: "broken" },
          },
          {
            name: "second case still runs",
            input: { id: "ok" },
            assertions: [expectPathEquals("status equals ok", "status", "ok")],
          },
        ],
      },
      (input) => {
        if (input.id === "broken") {
          throw new Error("upstream model timeout");
        }
        return Promise.resolve({ status: "ok" });
      }
    );

    expect(result.failed).toBe(1);
    expect(result.passed).toBe(1);
    expect(result.cases[0]?.error).toBe("upstream model timeout");
    expect(result.cases[1]?.passed).toBe(true);
  });
});

describe("expectation helpers", () => {
  it("checks JSON paths with stable serialization", () => {
    const assertion = expectPathEquals<{ nested: { value: string } }>(
      "nested value matches",
      "nested.value",
      "ok"
    );

    expect(assertion.assert({ nested: { value: "ok" } })).toBe(true);
    expect(assertion.assert({ nested: { value: "bad" } })).toBe(false);
  });

  it("rejects negative array minimum lengths", () => {
    expect(() =>
      expectArrayMinLength("invalid", (value: string[]) => value, -1)
    ).toThrowError(new StraitSDKError("minLength must be a non-negative integer"));
  });
});
