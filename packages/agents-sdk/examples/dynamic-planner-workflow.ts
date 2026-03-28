import {
  agentStep,
  agentWorkflow,
  strait,
} from "../src/index";
import type { StraitContext } from "../src/index";

type PlannerInput = {
  topic: string;
  workerAgentIds: string[];
};

type ResearchInput = {
  lens: string;
  topic: string;
};

type DynamicStepPayload = {
  agent_id: string;
  depends_on: string[];
  payload?: Record<string, string>;
  step_ref: string;
};

const SYNTHESIZER_AGENT_ID = "agent_synthesizer";

export const plannerAgent = strait.agent<
  PlannerInput,
  { dynamic_steps: DynamicStepPayload[] }
>({
  name: "Dynamic Research Planner",
  slug: "dynamic-research-planner",
  description: "Plans follow-up research steps and expands the workflow DAG at runtime.",
  model: "gpt-5.4-mini",
  async run(
    ctx: StraitContext,
    input: PlannerInput
  ): Promise<{ dynamic_steps: DynamicStepPayload[] }> {
    await ctx.workflow.state.set("topic", input.topic);
    await ctx.workflow.state.set("worker_count", input.workerAgentIds.length);

    const workerSteps = input.workerAgentIds.map((agentId, index) => ({
      step_ref: `research-${index + 1}`,
      agent_id: agentId,
      depends_on: ["planner"],
      payload: {
        topic: input.topic,
        lens: ["logs", "metrics", "deployments"][index] ?? `track-${index + 1}`,
      },
    }));

    return {
      dynamic_steps: [
        ...workerSteps,
        {
          step_ref: "synthesis",
          agent_id: SYNTHESIZER_AGENT_ID,
          depends_on: workerSteps.map((step) => step.step_ref),
          payload: {
            topic: input.topic,
            summary_style: "incident-brief",
          },
        },
      ],
    };
  },
});

export const researchWorkerAgent = strait.agent<ResearchInput, { finding: string }>({
  name: "Research Worker",
  slug: "research-worker",
  description: "Investigates one lens of a topic and returns a focused finding.",
  model: "gpt-5.4-mini",
  async run(ctx: StraitContext, input: ResearchInput): Promise<{ finding: string }> {
    await ctx.log({
      level: "info",
      message: `researching ${input.topic}`,
      data: { lens: input.lens },
    });

    return {
      finding: `Investigated ${input.topic} through the ${input.lens} lens.`,
    };
  },
});

export const synthesizerAgent = strait.agent<
  { summary_style: string; topic: string },
  { summary: string }
>({
  name: "Research Synthesizer",
  slug: "research-synthesizer",
  description: "Reads shared workflow state and synthesizes worker output.",
  model: "gpt-5.4-mini",
  async run(
    ctx: StraitContext,
    input: { summary_style: string; topic: string }
  ): Promise<{ summary: string }> {
    const topicState = await ctx.workflow.state.get("topic");

    return {
      summary: `Prepared a ${input.summary_style} summary for ${String(topicState.value ?? input.topic)}.`,
    };
  },
});

export const dynamicResearchWorkflow = agentWorkflow({
  name: "Dynamic Research Workflow",
  slug: "dynamic-research-workflow",
  project_id: "proj_agents",
  steps: [
    agentStep("planner", "agent_planner", {
      payload: {
        topic: "billing reliability regression",
        workerAgentIds: ["agent_logs", "agent_metrics", "agent_deploys"],
      },
    }),
  ],
});
