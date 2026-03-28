import {
  agentStep,
  agentWorkflow,
  approvalStep,
  type DynamicWorkflowStepEnvelope,
  fanOutSteps,
  type StraitContext,
  strait,
} from "../src/index";

type PlannerInput = {
  topic: string;
  workerAgentIds: string[];
};

const plannerAgent = strait.agent<PlannerInput, DynamicWorkflowStepEnvelope>({
  description: "Plans a research fan-out and stores shared workflow state.",
  model: "gpt-5.4-mini",
  name: "Research Planner",
  slug: "research-planner",
  async run(ctx: StraitContext, input: PlannerInput) {
    await ctx.workflow.state.set("topic", input.topic);

    return ctx.createDynamicSteps(
      fanOutSteps({
        dependsOn: ["planner"],
        stepRefPrefix: "research",
        synthesizer: {
          agentId: "agent-synthesizer",
          payload: {
            summaryStyle: "executive-brief",
            topic: input.topic,
          },
          stepRef: "synthesis",
        },
        workers: input.workerAgentIds.map((agentId, index) => ({
          agentId,
          payload: {
            lens:
              ["logs", "metrics", "deployments"][index] ?? `track-${index + 1}`,
            topic: input.topic,
          },
        })),
      }),
      { knownStepRefs: ["planner"] }
    );
  },
});

const workerAgent = strait.agent({
  description: "Executes one research branch with durable AI step tracking.",
  model: "gpt-5.4-mini",
  name: "Research Worker",
  slug: "research-worker",
  async run(ctx: StraitContext, input: { lens: string; topic: string }) {
    await ctx.checkpoint({
      lens: input.lens,
      phase: "researching",
      topic: input.topic,
    });
    await ctx.log({
      level: "info",
      message: `Researching ${input.topic} via ${input.lens}`,
    });

    return {
      finding: `Investigated ${input.topic} via ${input.lens}.`,
      lens: input.lens,
    };
  },
});

const editorAgent = strait.agent({
  description: "Synthesizes all worker findings after approval.",
  model: "gpt-5.4",
  name: "Research Editor",
  slug: "research-editor",
  async run(
    ctx: StraitContext,
    input: { summaryStyle: string; topic: string }
  ) {
    const topic = await ctx.workflow.state.get("topic");

    return {
      style: input.summaryStyle,
      summary: `Prepared a ${input.summaryStyle} for ${String(topic.value ?? input.topic)}.`,
    };
  },
});

export const multiAgentResearchWorkflow = agentWorkflow({
  description:
    "Planner fan-out, parallel workers, approval gate, and synthesis.",
  max_parallel_steps: 4,
  name: "Multi-Agent Research Pipeline",
  project_id: "proj_agents",
  slug: "multi-agent-research-pipeline",
  steps: [
    agentStep("planner", "agent-planner", {
      payload: {
        topic: "runtime orchestration reliability",
        workerAgentIds: ["agent-logs", "agent-metrics", "agent-deployments"],
      },
    }),
    ...fanOutSteps({
      dependsOn: ["planner"],
      stepRefPrefix: "research",
      synthesizer: {
        agentId: "agent-editor",
        payload: {
          summaryStyle: "incident-brief",
          topic: "runtime orchestration reliability",
        },
        stepRef: "synthesis",
      },
      workers: [
        {
          agentId: "agent-logs",
          payload: {
            lens: "logs",
            topic: "runtime orchestration reliability",
          },
        },
        {
          agentId: "agent-metrics",
          payload: {
            lens: "metrics",
            topic: "runtime orchestration reliability",
          },
        },
        {
          agentId: "agent-deployments",
          payload: {
            lens: "deployments",
            topic: "runtime orchestration reliability",
          },
        },
      ],
    }),
    approvalStep("approve-summary", {
      approvers: ["ops@strait.dev"],
      dependsOn: ["synthesis"],
      timeoutSecs: 3600,
    }),
    agentStep("publish-summary", "agent-editor", {
      dependsOn: ["approve-summary"],
      payload: {
        summaryStyle: "incident-brief",
      },
    }),
  ],
});

export { editorAgent, plannerAgent, workerAgent };
