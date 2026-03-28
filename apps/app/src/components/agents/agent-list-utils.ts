import type { Agent } from "@/hooks/api/types";

export function filterAgents(agents: Agent[], query?: string): Agent[] {
  const normalizedQuery = query?.trim().toLowerCase();
  if (!normalizedQuery) {
    return sortAgentsByUpdatedAt(agents);
  }

  return sortAgentsByUpdatedAt(
    agents.filter((agent) =>
      [agent.name, agent.slug, agent.model, agent.description ?? ""]
        .join(" ")
        .toLowerCase()
        .includes(normalizedQuery)
    )
  );
}

export function sortAgentsByUpdatedAt(agents: Agent[]): Agent[] {
  return [...agents].sort(
    (left, right) =>
      new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime()
  );
}
