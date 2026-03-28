import { describe, expect, it } from "vitest";

import type { Agent, JobRun, RunStatus, RunUsage } from "@/hooks/api/types";
import {
  buildAgentListRows,
  filterAgents,
  sortAgentsByUpdatedAt,
} from "./agent-list-utils";

const baseAgent = {
  config: {},
  created_at: "2026-03-20T09:00:00Z",
  id: "agent-1",
  job_id: "job-1",
  model: "gpt-5.4",
  name: "Support Agent",
  project_id: "proj-1",
  slug: "support-agent",
  updated_at: "2026-03-21T09:00:00Z",
} satisfies Agent;

function makeRun(id: string, status: RunStatus, createdAt: string): JobRun {
  return {
    attempt: 1,
    created_at: createdAt,
    debug_mode: false,
    error: undefined,
    execution_trace: undefined,
    finished_at: createdAt,
    id,
    job_id: "job_123",
    job_version: 1,
    lineage_depth: 0,
    payload: null,
    priority: 0,
    project_id: "project_123",
    result: null,
    started_at: createdAt,
    status,
    triggered_by: "manual",
  };
}

function makeUsage(
  id: string,
  runId: string,
  costMicrousd: number,
  createdAt: string
): RunUsage {
  return {
    completion_tokens: 10,
    cost_microusd: costMicrousd,
    created_at: createdAt,
    id,
    model: "gpt-5.4",
    prompt_tokens: 20,
    provider: "openai",
    run_id: runId,
    total_tokens: 30,
  };
}

describe("agent-list-utils", () => {
  it("filters agents across name, slug, model, and description", () => {
    const agents = [
      {
        ...baseAgent,
        active_runs: 0,
        total_cost_microusd: 0,
        total_runs: 0,
      },
      {
        ...baseAgent,
        id: "agent-2",
        name: "Ops Agent",
        slug: "ops-agent",
        model: "claude-sonnet-4-5",
        description: "Handles incident response",
        updated_at: "2026-03-22T09:00:00Z",
        active_runs: 0,
        total_cost_microusd: 0,
        total_runs: 0,
      },
    ];

    expect(filterAgents(agents, "incident")).toEqual([agents[1]]);
    expect(filterAgents(agents, "support")).toEqual([agents[0]]);
    expect(filterAgents(agents, "claude")).toEqual([agents[1]]);
  });

  it("sorts by most recently updated first", () => {
    const older = {
      ...baseAgent,
      id: "agent-older",
      updated_at: "2026-03-20T09:00:00Z",
    };
    const newer = {
      ...baseAgent,
      id: "agent-newer",
      updated_at: "2026-03-23T09:00:00Z",
    };

    expect(
      sortAgentsByUpdatedAt([older, newer]).map((agent) => agent.id)
    ).toEqual(["agent-newer", "agent-older"]);
  });

  it("builds agent list rows with run counts, active counts, and total cost", () => {
    const rows = buildAgentListRows(
      [
        baseAgent,
        {
          ...baseAgent,
          id: "agent-2",
          name: "Ops Agent",
          slug: "ops-agent",
          updated_at: "2026-03-22T09:00:00Z",
        },
      ],
      {
        "agent-1": {
          runs: [
            makeRun("run-1", "completed", "2026-03-28T09:00:00Z"),
            makeRun("run-2", "executing", "2026-03-28T10:00:00Z"),
          ],
          usage: [
            makeUsage("usage-1", "run-1", 500, "2026-03-28T09:01:00Z"),
            makeUsage("usage-2", "run-2", 250, "2026-03-28T10:01:00Z"),
          ],
        },
        "agent-2": {
          runs: [],
          usage: [],
        },
      }
    );

    expect(rows[0]).toMatchObject({
      id: "agent-2",
      total_cost_microusd: 0,
      total_runs: 0,
    });
    expect(rows[1]).toMatchObject({
      active_runs: 1,
      id: "agent-1",
      last_run_status: "executing",
      total_cost_microusd: 750,
      total_runs: 2,
    });
  });
});
