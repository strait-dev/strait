import { describe, expect, it } from "vitest";

import { StraitSDKError } from "./errors";
import {
  agentStep,
  agentWorkflow,
  approvalStep,
  createDynamicSteps,
  debatePattern,
  fanOutSteps,
  orchestratorPattern,
  pipelinePattern,
  sleepStep,
  subWorkflowStep,
  waitForEventStep,
} from "./workflow";

const unknownDependencyError = /unknown workflow dependency missing/i;
const dynamicStepLimitError = /cannot contain more than 100 steps/i;

describe("agent workflow helpers", () => {
  it("builds an agent-backed job step with retry and execution metadata", () => {
    expect(
      agentStep("research", "agent-research", {
        concurrencyKey: "incident:123",
        dependsOn: ["plan"],
        payload: { topic: "agents" },
        resourceClass: "medium",
        retry: {
          backoff: "exponential",
          initialDelaySecs: 5,
          maxAttempts: 3,
        },
      })
    ).toEqual({
      agent_id: "agent-research",
      concurrency_key: "incident:123",
      depends_on: ["plan"],
      payload: { topic: "agents" },
      resource_class: "medium",
      retry_backoff: "exponential",
      retry_initial_delay_secs: 5,
      retry_max_attempts: 3,
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

  it("builds wait-for-event, sleep, and sub-workflow steps", () => {
    expect(
      waitForEventStep("await-webhook", {
        dependsOn: ["dispatch"],
        eventKey: "deploy:finished",
        eventTimeoutSecs: 600,
      })
    ).toEqual({
      depends_on: ["dispatch"],
      event_key: "deploy:finished",
      event_timeout_secs: 600,
      step_ref: "await-webhook",
      step_type: "wait_for_event",
    });

    expect(
      sleepStep("cooldown", {
        dependsOn: ["await-webhook"],
        durationSecs: 30,
      })
    ).toEqual({
      depends_on: ["await-webhook"],
      sleep_duration_secs: 30,
      step_ref: "cooldown",
      step_type: "sleep",
    });

    expect(
      subWorkflowStep("follow-up", {
        dependsOn: ["cooldown"],
        maxNestingDepth: 2,
        subWorkflowId: "workflow-postmortem",
      })
    ).toEqual({
      depends_on: ["cooldown"],
      max_nesting_depth: 2,
      step_ref: "follow-up",
      step_type: "sub_workflow",
      sub_workflow_id: "workflow-postmortem",
    });
  });

  it("rejects duplicate workflow step refs", () => {
    expect(() =>
      agentWorkflow({
        name: "Duplicate refs",
        project_id: "proj-1",
        slug: "duplicate-refs",
        steps: [agentStep("same", "agent-a"), agentStep("same", "agent-b")],
      })
    ).toThrowError(StraitSDKError);
  });

  it("rejects workflow dependencies that do not exist", () => {
    expect(() =>
      agentWorkflow({
        name: "Broken refs",
        project_id: "proj-1",
        slug: "broken-refs",
        steps: [
          agentStep("draft", "writer"),
          agentStep("review", "editor", { dependsOn: ["missing"] }),
        ],
      })
    ).toThrowError(unknownDependencyError);
  });

  it("creates validated dynamic step envelopes with external dependencies", () => {
    expect(
      createDynamicSteps(
        [
          agentStep("research-1", "agent-web", {
            dependsOn: ["planner"],
            payload: { topic: "agents", track: "web" },
          }),
          agentStep("research-2", "agent-papers", {
            dependsOn: ["planner"],
            payload: { topic: "agents", track: "papers" },
          }),
          agentStep("synthesis", "agent-synth", {
            dependsOn: ["research-1", "research-2"],
          }),
        ],
        { knownStepRefs: ["planner"] }
      )
    ).toEqual({
      dynamic_steps: [
        {
          agent_id: "agent-web",
          depends_on: ["planner"],
          payload: { topic: "agents", track: "web" },
          step_ref: "research-1",
          step_type: "job",
        },
        {
          agent_id: "agent-papers",
          depends_on: ["planner"],
          payload: { topic: "agents", track: "papers" },
          step_ref: "research-2",
          step_type: "job",
        },
        {
          agent_id: "agent-synth",
          depends_on: ["research-1", "research-2"],
          step_ref: "synthesis",
          step_type: "job",
        },
      ],
    });
  });

  it("rejects oversized dynamic expansions", () => {
    expect(() =>
      createDynamicSteps(
        new Array(101).fill(null).map((_, index) => ({
          agent_id: `agent-${index}`,
          step_ref: `worker-${index}`,
          step_type: "job" as const,
        }))
      )
    ).toThrowError(dynamicStepLimitError);
  });

  it("builds fan-out worker steps with a synthesizer fan-in", () => {
    expect(
      fanOutSteps({
        dependsOn: ["planner"],
        stepRefPrefix: "research",
        synthesizer: {
          agentId: "agent-synth",
          stepRef: "synthesis",
        },
        workers: [
          { agentId: "agent-web", payload: { track: "web" } },
          { agentId: "agent-papers", payload: { track: "papers" } },
        ],
      })
    ).toEqual([
      {
        agent_id: "agent-web",
        depends_on: ["planner"],
        payload: { track: "web" },
        step_ref: "research-1",
        step_type: "job",
      },
      {
        agent_id: "agent-papers",
        depends_on: ["planner"],
        payload: { track: "papers" },
        step_ref: "research-2",
        step_type: "job",
      },
      {
        agent_id: "agent-synth",
        depends_on: ["research-1", "research-2"],
        step_ref: "synthesis",
        step_type: "job",
      },
    ]);
  });

  it("builds a sequential pipeline pattern", () => {
    const workflow = pipelinePattern({
      name: "Content pipeline",
      projectId: "proj-1",
      slug: "content-pipeline",
      steps: [
        { agentId: "writer", stepRef: "draft" },
        {
          approvers: ["editor"],
          stepRef: "review",
          timeoutSecs: 1800,
          type: "approval",
        },
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

  it("builds an orchestrator pattern with planner fan-out and synthesizer fan-in", () => {
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
