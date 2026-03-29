import type { StraitContext } from "../src/index";
import { createSandboxTool, strait } from "../src/index";

type ResearchRequest = {
  query: string;
  scope: "lightweight" | "heavy";
};

type WorkflowTriggerResult = {
  accepted: boolean;
  workflowRunId: string;
};

const triggerWorkflow = createSandboxTool<
  { payload: Record<string, unknown>; workflowSlug: string },
  WorkflowTriggerResult
>({
  description:
    "Triggers a durable Strait workflow when the request leaves the low-latency agent path.",
  mode: "dynamic-worker",
  name: "trigger_workflow",
  networkClass: "restricted",
  outboundPolicyTag: "workflow-handoff",
  runtime: "javascript",
  timeoutMs: 15_000,
  execute(input) {
    return {
      accepted: true,
      workflowRunId: `wfr_${input.workflowSlug}`,
    };
  },
});

export const researchRouterAgent = strait.agent<
  ResearchRequest,
  { mode: "agent" | "workflow"; summary: string; workflowRunId?: string }
>({
  description:
    "Keeps lightweight research in the agent loop and escalates heavy work to a durable workflow.",
  model: "gpt-5.4-mini",
  name: "Research Router",
  slug: "research-router",
  async run(
    ctx: StraitContext,
    input: ResearchRequest
  ): Promise<{
    mode: "agent" | "workflow";
    summary: string;
    workflowRunId?: string;
  }> {
    await ctx.log({
      level: "info",
      message: `routing ${input.query}`,
      data: { scope: input.scope },
    });

    if (input.scope === "lightweight") {
      return {
        mode: "agent",
        summary: `Handled ${input.query} entirely inside the agent runtime.`,
      };
    }

    const workflow = await triggerWorkflow.execute({
      payload: {
        query: input.query,
        requestedBy: "research-router",
      },
      workflowSlug: "deep-research-pipeline",
    });

    await ctx.checkpoint({
      phase: "workflow_handoff",
      workflow_run_id: workflow.workflowRunId,
    });

    return {
      mode: "workflow",
      summary:
        "Escalated the request to a Fly-backed workflow for heavier processing.",
      workflowRunId: workflow.workflowRunId,
    };
  },
});
