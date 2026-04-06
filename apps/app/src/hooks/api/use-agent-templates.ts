import { queryOptions } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME } from "@/hooks/utils";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

export type AgentTemplate = {
  category: string;
  config: Record<string, NonNullable<unknown>>;
  description: string;
  model: string;
  name: string;
  slug: string;
};

/**
 * Fetches the built-in agent templates from the API.
 * Returns an empty array on error so the gallery renders gracefully.
 */
export const fetchAgentTemplates = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (): Promise<AgentTemplate[]> => {
    try {
      return await runWithSentryReport(
        apiEffect<AgentTemplate[]>("/v1/agents/templates")
      );
    } catch {
      return [];
    }
  });

export const agentTemplatesQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.agents.templates.queryKey,
    queryFn: () => fetchAgentTemplates(),
    staleTime: Number.POSITIVE_INFINITY,
    gcTime: DEFAULT_GC_TIME,
  });
