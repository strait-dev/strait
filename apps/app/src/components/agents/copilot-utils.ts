import type { Agent, JobRun } from "@/hooks/api/types";

export type CopilotAnswer = {
  bullets: string[];
  summary: string;
  title: string;
};

type AgentRunSummary = {
  agent: Agent;
  failedRuns: number;
  latestRun?: JobRun;
  successfulRuns: number;
  totalRuns: number;
};

function buildAgentRunSummaries(
  agents: Agent[],
  runs: JobRun[]
): AgentRunSummary[] {
  return agents.map((agent) => {
    const agentRuns = runs.filter((run) => run.job_id === agent.job_id);
    const sortedRuns = [...agentRuns].sort((left, right) =>
      right.created_at.localeCompare(left.created_at)
    );

    return {
      agent,
      latestRun: sortedRuns[0],
      totalRuns: agentRuns.length,
      failedRuns: agentRuns.filter((run) =>
        ["crashed", "dead_letter", "failed", "system_failed", "timed_out"].includes(
          run.status
        )
      ).length,
      successfulRuns: agentRuns.filter((run) => run.status === "completed").length,
    };
  });
}

function listAgentLabels(summaries: AgentRunSummary[]): string[] {
  return summaries.map(({ agent, latestRun, totalRuns }) => {
    const latestStatus = latestRun?.status ?? "no runs yet";
    return `${agent.name} (${agent.slug}) has ${totalRuns} runs; latest status: ${latestStatus}.`;
  });
}

function buildOverviewAnswer(summaries: AgentRunSummary[]): CopilotAnswer {
  const noRuns = summaries.filter((summary) => summary.totalRuns === 0);
  const failing = summaries.filter((summary) => summary.failedRuns > 0);
  const healthy = summaries.filter(
    (summary) => summary.successfulRuns > 0 && summary.failedRuns === 0
  );

  return {
    title: "Project Copilot Overview",
    summary:
      healthy.length === summaries.length
        ? "Your current agent fleet looks healthy based on the loaded runs."
        : "Here is the fastest triage view for the currently loaded agent and run data.",
    bullets: [
      `${summaries.length} agents are currently registered in this project.`,
      `${healthy.length} agents have only healthy recent signals in the loaded run window.`,
      `${failing.length} agents show at least one failed, timed out, crashed, or dead-letter run.`,
      `${noRuns.length} agents have never been exercised in the loaded data and should get a smoke run plus an eval suite.`,
    ],
  };
}

function buildFailureAnswer(summaries: AgentRunSummary[]): CopilotAnswer {
  const failing = summaries.filter((summary) => summary.failedRuns > 0);
  const bullets =
    failing.length > 0
      ? listAgentLabels(failing)
      : ["No failing agent runs were found in the loaded run window."];

  return {
    title: "Recent Failures",
    summary:
      failing.length > 0
        ? "These agents should get immediate attention."
        : "No failing agent activity was detected in the recent run sample.",
    bullets,
  };
}

function buildCoverageAnswer(summaries: AgentRunSummary[]): CopilotAnswer {
  const unexercised = summaries.filter((summary) => summary.totalRuns === 0);
  const lowCoverage = summaries.filter(
    (summary) => summary.totalRuns > 0 && summary.successfulRuns === 0
  );

  return {
    title: "Coverage Gaps",
    summary:
      "Use this list to decide which agents need a smoke run, eval suite, or deployment sanity check next.",
    bullets: [
      ...listAgentLabels(unexercised),
      ...listAgentLabels(lowCoverage),
      ...(unexercised.length === 0 && lowCoverage.length === 0
        ? ["All loaded agents have at least one successful run."]
        : []),
    ],
  };
}

function buildEvaluationAnswer(summaries: AgentRunSummary[]): CopilotAnswer {
  const target =
    summaries.find((summary) => summary.failedRuns > 0) ??
    summaries.find((summary) => summary.totalRuns === 0) ??
    summaries[0];

  return {
    title: "Local Evaluation Plan",
    summary:
      target == null
        ? "No agents are loaded yet. Create an agent first, then add an eval suite in packages/agents-sdk."
        : `Start with ${target.agent.name} and add a local eval suite before expanding the prompt surface.`,
    bullets: [
      "Create a local eval suite with defineEvalSuite() and runEvalSuite() in @strait/agents-sdk.",
      "Add one happy-path case, one adversarial case, and one shape assertion for the returned payload.",
      "Exercise the agent locally through apps/agents and compare the eval result with recent run telemetry.",
    ],
  };
}

export function answerCopilotPrompt(
  prompt: string,
  agents: Agent[],
  runs: JobRun[]
): CopilotAnswer {
  const summaries = buildAgentRunSummaries(agents, runs);
  const normalizedPrompt = prompt.trim().toLowerCase();

  if (
    normalizedPrompt.includes("fail") ||
    normalizedPrompt.includes("error") ||
    normalizedPrompt.includes("broken")
  ) {
    return buildFailureAnswer(summaries);
  }

  if (
    normalizedPrompt.includes("coverage") ||
    normalizedPrompt.includes("untested") ||
    normalizedPrompt.includes("smoke") ||
    normalizedPrompt.includes("deploy")
  ) {
    return buildCoverageAnswer(summaries);
  }

  if (
    normalizedPrompt.includes("eval") ||
    normalizedPrompt.includes("quality") ||
    normalizedPrompt.includes("regression")
  ) {
    return buildEvaluationAnswer(summaries);
  }

  return buildOverviewAnswer(summaries);
}

export function buildSuggestedPrompts(): string[] {
  return [
    "Which agents need attention right now?",
    "Show me agents with weak coverage or no smoke runs.",
    "How should I evaluate a new agent locally?",
  ];
}
