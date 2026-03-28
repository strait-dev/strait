import { describe, expect, it } from "vitest";

import { StraitSDKError } from "./errors";
import {
  agentStep,
  agentWorkflow,
  approvalStep,
  debatePattern,
  orchestratorPattern,
  pipelinePattern,
} from "./workflow";

describe("agent workflow helpers", () => {
  it("builds an agent-backed job step", () => {
    expect(
      agentStep("research", "agent-research", {
        dependsOn: ["plan"],
        payload: { topic: "agents" },
      })
    ).toEqual({
      agent_id: "agent-research",
      depends_on: ["plan"],
      payload: { topic: "agents" },
      step_ref: "research",
      step_type: "job",
    });
  });

  it("builds an approval gate step", () => {
    expect(
      approvalStep("approve", {
        approvers: ["alice"],
        dependsOn: ["draft"],
        timeoutSecs: 3600,
      })
    ).toEqual({
      approval_approvers: ["alice"],
      approval_timeout_secs: 3600,
      depends_on: ["draft"],
      step_ref: "approve",
      step_type: "approval",
    });
  });

  it("rejects duplicate workflow step refs", () => {
    expect(() =>
      agentWorkflow({
        name: "Duplicate refs",
        project_id: "proj-1",
        slug: "duplicate-refs",
        steps: [
          agentStep("same", "agent-a"),
          agentStep("same", "agent-b"),
        ],
      })
    ).toThrowError(StraitSDKError);
  });

  it("builds a sequential pipeline pattern", () => {
    const workflow = pipelinePattern({
      name: "Content pipeline",
      projectId: "proj-1",
      slug: "content-pipeline",
      steps: [
        { agentId: "writer", stepRef: "draft" },
        { approvers: ["editor"], stepRef: "review", timeoutSecs: 1800, type: "approval" },
        { agentId: "publisher", stepRef: "publish" },
      ],
    });

    expect(workflow.steps).toEqual([
      {
        agent_id: "writer",
        step_ref: "draft",
        step_type: "job",
      },
      {
        approval_approvers: ["editor"],
        approval_timeout_secs: 1800,
        depends_on: ["draft"],
        step_ref: "review",
        step_type: "approval",
      },
      {
        agent_id: "publisher",
        depends_on: ["review"],
        step_ref: "publish",
        step_type: "job",
      },
    ]);
  });

  it("builds a debate pattern with a judge fan-in", () => {
    const workflow = debatePattern({
      analysts: [
        { agentId: "agent-a", stepRef: "analyst-a" },
        { agentId: "agent-b", stepRef: "analyst-b" },
      ],
      judge: { agentId: "judge", stepRef: "judge" },
      name: "Debate",
      projectId: "proj-1",
      slug: "debate",
    });

    expect(workflow.max_parallel_steps).toBe(2);
    expect(workflow.steps.at(-1)).toEqual({
      agent_id: "judge",
      depends_on: ["analyst-a", "analyst-b"],
      step_ref: "judge",
      step_type: "job",
    });
  });

  it("builds an orchestrator pattern with planner, workers, and synthesizer", () => {
    const workflow = orchestratorPattern({
      name: "Research",
      planner: { agentId: "planner", stepRef: "plan" },
      projectId: "proj-1",
      slug: "research",
      synthesizer: { agentId: "synth", stepRef: "synthesize" },
      workers: [
        { agentId: "worker-1", stepRef: "search-web" },
        { agentId: "worker-2", stepRef: "search-papers" },
      ],
    });

    expect(workflow.steps).toEqual([
      {
        agent_id: "planner",
        step_ref: "plan",
        step_type: "job",
      },
      {
        agent_id: "worker-1",
        depends_on: ["plan"],
        step_ref: "search-web",
        step_type: "job",
      },
      {
        agent_id: "worker-2",
        depends_on: ["plan"],
        step_ref: "search-papers",
        step_type: "job",
      },
      {
        agent_id: "synth",
        depends_on: ["search-web", "search-papers"],
        step_ref: "synthesize",
        step_type: "job",
      },
    ]);
  });
});
