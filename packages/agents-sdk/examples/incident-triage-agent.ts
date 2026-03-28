import OpenAI from "openai";
import type { StraitContext } from "../src/index";
import { createOpenAIAdapter, strait } from "../src/index";

type IncidentInput = {
  incidentId: string;
  logs: string[];
  recentChanges: string[];
  severity: "sev1" | "sev2" | "sev3";
};

type IncidentSummary = {
  actions: string[];
  owner: string;
  summary: string;
};

type ExampleOpenAIClient = {
  chat: {
    completions: {
      create: (...args: unknown[]) => Promise<{
        model: string;
        usage?: {
          completion_tokens: number;
          prompt_tokens: number;
          total_tokens: number;
        };
      }>;
      runTools: (...args: unknown[]) => {
        on: (
          event: string,
          listener: (...args: unknown[]) => unknown
        ) => unknown;
      };
      stream: (...args: unknown[]) => {
        on: (
          event: string,
          listener: (...args: unknown[]) => unknown
        ) => unknown;
      };
    };
  };
};

const openaiClient = new OpenAI({
  apiKey: process.env.OPENAI_API_KEY ?? "test-key",
}) as unknown as ExampleOpenAIClient;

export const incidentTriageAgent = strait.agent<IncidentInput, IncidentSummary>(
  {
    name: "Incident Triage",
    slug: "incident-triage",
    description: "Summarizes incident evidence and proposes next actions.",
    model: "gpt-5.4-mini",
    budget: {
      maxCostMicrousd: 3_000_000,
      maxTokens: 12_000,
      maxToolCalls: 4,
    },
    async run(
      ctx: StraitContext,
      input: IncidentInput
    ): Promise<IncidentSummary> {
      const openai = createOpenAIAdapter(openaiClient, ctx, {
        streamId: "assistant",
      });

      await ctx.log({
        level: "info",
        message: `triaging ${input.incidentId}`,
        data: { severity: input.severity },
      });
      await ctx.progress({
        percent: 15,
        message: "building incident prompt",
        step: "triage",
      });

      await openai.chat.completions.create({
        model: "gpt-5.4-mini",
        messages: [
          {
            role: "system",
            content:
              "You are an incident commander. Summarize the issue and propose the next actions.",
          },
          {
            role: "user",
            content: JSON.stringify({
              incidentId: input.incidentId,
              severity: input.severity,
              recentChanges: input.recentChanges,
              logs: input.logs.slice(-20),
            }),
          },
        ],
      });

      await ctx.checkpoint(
        {
          incidentId: input.incidentId,
          phase: "triaged",
          severity: input.severity,
        },
        {
          source: "incident-triage",
        }
      );

      return {
        summary: `Generated an initial triage plan for ${input.incidentId}.`,
        owner: "platform-oncall",
        actions: [
          "Check the most recent deployment for this service.",
          "Review error-rate and latency dashboards for correlated regressions.",
          "Prepare a rollback or mitigation note if impact continues.",
        ],
      };
    },
  }
);
