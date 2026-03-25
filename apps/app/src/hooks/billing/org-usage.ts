export type UsageDimension = {
  used: number;
  limit: number;
  percent: number;
  display?: string;
};

export type UsageAlert = {
  type: string;
  dimension: string;
  threshold: number;
  message: string;
};

type BaseUsageDimensions = {
  runs_today: UsageDimension;
  concurrent_runs: UsageDimension;
  compute_credit: UsageDimension;
  projects: UsageDimension;
  members: UsageDimension;
  retention_days: number;
  regions_available: number;
};

export type RawOrgUsageDimensions = BaseUsageDimensions & {
  ai_model_calls_today?: UsageDimension;
  ai_assistant_messages_today?: UsageDimension;
};

export type OrgUsageDimensions = BaseUsageDimensions & {
  ai_model_calls_today: UsageDimension;
  ai_assistant_messages_today: UsageDimension;
};

export type RawOrgUsageData = {
  org_id: string;
  plan: string;
  period: {
    start: string;
    end: string;
  };
  usage: RawOrgUsageDimensions;
  included_credit_microusd: number;
  period_spend_microusd: number;
  overage_microusd: number;
  alerts: UsageAlert[];
  payment_status?: string;
  grace_period_end?: string;
};

export type OrgUsageData = Omit<RawOrgUsageData, "usage"> & {
  usage: OrgUsageDimensions;
};

const EMPTY_AI_MODEL_CALLS: UsageDimension = {
  used: 0,
  limit: 20,
  percent: 0,
  display: "0",
};

/** Default empty usage data returned when no organization is active. */
export const EMPTY_ORG_USAGE: OrgUsageData = {
  org_id: "",
  plan: "free",
  period: { start: "", end: "" },
  included_credit_microusd: 0,
  period_spend_microusd: 0,
  overage_microusd: 0,
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
    ai_model_calls_today: EMPTY_AI_MODEL_CALLS,
    ai_assistant_messages_today: EMPTY_AI_MODEL_CALLS,
    retention_days: 1,
    regions_available: 1,
  },
  alerts: [],
};

export function normalizeOrgUsageData(raw: RawOrgUsageData): OrgUsageData {
  const aiModelCalls =
    raw.usage.ai_model_calls_today ??
    raw.usage.ai_assistant_messages_today ??
    EMPTY_ORG_USAGE.usage.ai_model_calls_today;

  return {
    ...raw,
    usage: {
      ...raw.usage,
      ai_model_calls_today: aiModelCalls,
      ai_assistant_messages_today:
        raw.usage.ai_assistant_messages_today ?? aiModelCalls,
    },
  };
}
