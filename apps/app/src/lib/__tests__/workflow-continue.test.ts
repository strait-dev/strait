import { describe, expect, it } from "vitest";
import type { WorkflowRun } from "@/hooks/api/types";
import {
  CONTINUE_VERSION_STRATEGIES,
  canContinueWorkflowRun,
  DEFAULT_CONTINUE_VERSION_STRATEGY,
  isPartOfChain,
  parseContinueInput,
} from "@/lib/workflow-continue";

type ChainFields = Pick<
  WorkflowRun,
  | "continued_from_workflow_run_id"
  | "continued_to_workflow_run_id"
  | "lineage_depth"
>;

describe("continue version strategies", () => {
  it("exposes repin and latest", () => {
    expect(CONTINUE_VERSION_STRATEGIES).toEqual(["repin", "latest"]);
  });

  it("defaults to repin to match the server", () => {
    expect(DEFAULT_CONTINUE_VERSION_STRATEGY).toBe("repin");
  });
});

describe("canContinueWorkflowRun", () => {
  it("allows running and paused", () => {
    expect(canContinueWorkflowRun("running")).toBe(true);
    expect(canContinueWorkflowRun("paused")).toBe(true);
  });

  it("rejects terminal and pending states", () => {
    for (const status of [
      "pending",
      "completed",
      "failed",
      "timed_out",
      "canceled",
      "continued",
    ] as const) {
      expect(canContinueWorkflowRun(status)).toBe(false);
    }
  });

  it("rejects unknown statuses", () => {
    expect(canContinueWorkflowRun("not_a_status")).toBe(false);
  });
});

describe("isPartOfChain", () => {
  const base: ChainFields = { lineage_depth: 0 };

  it("is false for a standalone root run", () => {
    expect(isPartOfChain(base)).toBe(false);
  });

  it("is true for a successor run", () => {
    expect(
      isPartOfChain({ ...base, continued_from_workflow_run_id: "wfr_prev" })
    ).toBe(true);
  });

  it("is true for a predecessor run", () => {
    expect(
      isPartOfChain({ ...base, continued_to_workflow_run_id: "wfr_next" })
    ).toBe(true);
  });

  it("is true when lineage depth is above the root", () => {
    expect(isPartOfChain({ ...base, lineage_depth: 3 })).toBe(true);
  });

  it("tolerates a missing lineage depth", () => {
    expect(isPartOfChain({} as ChainFields)).toBe(false);
  });
});

describe("parseContinueInput", () => {
  it("treats empty and whitespace as no input", () => {
    expect(parseContinueInput("")).toEqual({ ok: true, value: undefined });
    expect(parseContinueInput("   \n ")).toEqual({
      ok: true,
      value: undefined,
    });
  });

  it("parses valid JSON objects, arrays, and scalars", () => {
    expect(parseContinueInput('{"cursor": 1}')).toEqual({
      ok: true,
      value: { cursor: 1 },
    });
    expect(parseContinueInput("[1, 2, 3]")).toEqual({
      ok: true,
      value: [1, 2, 3],
    });
    expect(parseContinueInput("42")).toEqual({ ok: true, value: 42 });
  });

  it("rejects malformed JSON with a friendly message", () => {
    const result = parseContinueInput("{cursor: 1}");
    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.error).toBe("Input must be valid JSON.");
    }
  });
});
