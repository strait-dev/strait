import { describe, expect, it } from "vitest";

import type { Agent, JobRun } from "@/hooks/api/types";

import { answerCopilotPrompt, buildSuggestedPrompts } from "./copilot-utils";

const agents: Agent[] = [
  {
    id: "agent-1",
    job_id: "job-1",
    model: "gpt-5.4-mini",
    name: "Planner",
    project_id: "proj-1",
    slug: "planner",
    created_at: "2026-03-28T12:00:00Z",
    updated_at: "2026-03-28T12:00:00Z",
  },
  {
    id: "agent-2",
    job_id: "job-2",
    model: "gpt-5.4-mini",
    name: "Synthesizer",
    project_id: "proj-1",
    slug: "synthesizer",
    created_at: "2026-03-28T12:00:00Z",
    updated_at: "2026-03-28T12:00:00Z",
  },
];

function makeRun(overrides: Partial<JobRun>): JobRun {
  return {
    attempt: 1,
    created_at: "2026-03-28T13:00:00Z",
    debug_mode: false,
    id: "run-default",
    job_id: "job-default",
    job_version: 1,
    lineage_depth: 0,
    priority: 0,
    project_id: "proj-1",
    status: "queued",
    triggered_by: "manual",
    ...overrides,
  };
}

const runs: JobRun[] = [
  makeRun({
    id: "run-1",
    job_id: "job-1",
    project_id: "proj-1",
    created_at: "2026-03-28T13:00:00Z",
    status: "failed",
  }),
  makeRun({
    id: "run-2",
    job_id: "job-1",
    project_id: "proj-1",
    created_at: "2026-03-28T11:00:00Z",
    status: "completed",
  }),
];

describe("answerCopilotPrompt", () => {
  it("returns failure-focused guidance when the prompt asks for failures", () => {
    const answer = answerCopilotPrompt(
      "Which agents are failing?",
      [...agents],
      [...runs]
    );

    expect(answer.title).toBe("Recent Failures");
    expect(answer.bullets[0]).toContain("Planner");
  });

  it("returns evaluation guidance when the prompt mentions evals", () => {
    const answer = answerCopilotPrompt(
      "How should I add evals for a new agent?",
      [...agents],
      [...runs]
    );

    expect(answer.title).toBe("Local Evaluation Plan");
    expect(answer.bullets[0]).toContain("defineEvalSuite");
  });

  it("falls back to an overview answer", () => {
    const answer = answerCopilotPrompt(
      "Give me the current state",
      [...agents],
      [...runs]
    );

    expect(answer.title).toBe("Project Copilot Overview");
    expect(answer.bullets[0]).toContain("2 agents");
  });
});

describe("buildSuggestedPrompts", () => {
  it("returns stable starter prompts", () => {
    expect(buildSuggestedPrompts()).toEqual([
      "Which agents need attention right now?",
      "Show me agents with weak coverage or no smoke runs.",
      "How should I evaluate a new agent locally?",
    ]);
  });
});
