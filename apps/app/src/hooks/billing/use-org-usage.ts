import { useQuery } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { apiEffect, runWithFallback } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

type UsageDimension = {
  used: number;
  limit: number;
  percent: number;
  display?: string;
};

type UsageAlert = {
  type: string;
  dimension: string;
  threshold: number;
  message: string;
};

export type OrgUsageData = {
  org_id: string;
  plan: string;
  period: {
    start: string;
    end: string;
  };
  usage: {
    runs_today: UsageDimension;
    concurrent_runs: UsageDimension;
    compute_credit: UsageDimension;
    projects: UsageDimension;
    members: UsageDimension;
    ai_assistant_messages_today: UsageDimension;
    retention_days: number;
    regions_available: number;
  };
  alerts: UsageAlert[];
};

const EMPTY_ORG_USAGE: OrgUsageData = {
  org_id: "",
  plan: "free",
  period: { start: "", end: "" },
  usage: {
    runs_today: { used: 0, limit: 5000, percent: 0, display: "0" },
    concurrent_runs: { used: 0, limit: 5, percent: 0, display: "0" },
    compute_credit: {
      used: 0,
      limit: 0,
      percent: 0,
      display: "$0.00 / $0.00",
    },
    projects: { used: 0, limit: 2, percent: 0, display: "0" },
    members: { used: 0, limit: 3, percent: 0, display: "0" },
    ai_assistant_messages_today: {
      used: 0,
      limit: 20,
      percent: 0,
      display: "0",
    },
    retention_days: 1,
    regions_available: 1,
  },
  alerts: [],
};

const getOrgUsageServerFn = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (ctx) => {
    const orgId = (ctx.context.session as Record<string, unknown>)
      .activeOrganizationId;

    if (!orgId || typeof orgId !== "string") {
      return EMPTY_ORG_USAGE;
    }

    return await runWithFallback(
      apiEffect<OrgUsageData>("/v1/usage/current", {
        params: { org_id: orgId },
      }),
      EMPTY_ORG_USAGE
    );
  });

export function useOrgUsage() {
  return useQuery({
    queryKey: ["org-usage"],
    queryFn: () => getOrgUsageServerFn(),
    refetchInterval: 60_000,
  });
}

export function useApproachingLimits() {
  const { data } = useOrgUsage();
  if (!data?.alerts) {
    return [];
  }
  return data.alerts.filter((a) => a.type === "approaching_limit");
}
