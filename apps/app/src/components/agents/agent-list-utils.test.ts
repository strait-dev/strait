import { describe, expect, it } from "vitest";

import type { Agent } from "@/hooks/api/types";
import { filterAgents, sortAgentsByUpdatedAt } from "./agent-list-utils";

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

describe("agent-list-utils", () => {
  it("filters agents across name, slug, model, and description", () => {
    const agents: Agent[] = [
      baseAgent,
      {
        ...baseAgent,
        id: "agent-2",
        name: "Ops Agent",
        slug: "ops-agent",
        model: "claude-sonnet-4-5",
        description: "Handles incident response",
        updated_at: "2026-03-22T09:00:00Z",
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
});
