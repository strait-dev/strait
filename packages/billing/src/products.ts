export type PlanKey =
  | "free"
  | "starter"
  | "pro"
  | "scale"
  | "business"
  | "enterprise";

export type Plan = {
  name: string;
  description: string;
  prices: { monthly: number; yearly: number }; // cents. 0=free, -1=custom
  trial: boolean;
  creditCardRequired: boolean;
  computeCredit: string;
  features: string[];
  limits: {
    organizations: number | null;
    projectsPerOrg: number | null;
    membersPerOrg: number | null;
    runsPerMonth: number | null;
    concurrentRuns: number | null;
    retentionDays: number | null;
    regions: string;
    spendingLimits: string;
    overagePerThousandRuns: number | null; // cents
    executionModes: string;
    workflowSteps: number | null;
    approvalGates: boolean;
    subWorkflows: boolean;
    jobChaining: boolean;
    canaryDeployments: boolean;
    scheduledJobs: number | null;
    cronMinIntervalSeconds: number | null; // 0 = sub-second
    environments: number | null;
    webhookEndpoints: number | null;
    apiRateLimit: number | null;
  };
  governance: {
    rbac: "none" | "basic" | "full" | "advanced";
    auditLogs: boolean;
    ssoSaml: boolean;
    aiMessagesPerDay: number | null;
    aiByok: boolean;
  };
  cta: { label: string; href: string };
  highlighted: boolean;
  badge?: string;
};

export const PLAN_KEYS: PlanKey[] = [
  "free",
  "starter",
  "pro",
  "scale",
  "business",
  "enterprise",
];

export const PLANS: Record<PlanKey, Plan> = {
  free: {
    name: "Free",
    description:
      "For side projects and experimentation.\nNo credit card required.",
    prices: { monthly: 0, yearly: 0 },
    trial: false,
    creditCardRequired: false,
    computeCredit: "5,000 runs/mo",
    features: [
      "1 project",
      "1 member",
      "3 concurrent runs",
      "5,000 runs/mo",
      "7-day retention",
      "Community support",
    ],
    limits: {
      organizations: 1,
      projectsPerOrg: 1,
      membersPerOrg: 1,
      runsPerMonth: 5000,
      concurrentRuns: 3,
      retentionDays: 7,
      regions: "1 (iad)",
      spendingLimits: "$50 hard cap (with card)",
      overagePerThousandRuns: 50,
      executionModes: "HTTP + worker",
      workflowSteps: 10,
      approvalGates: false,
      subWorkflows: false,
      jobChaining: false,
      canaryDeployments: false,
      scheduledJobs: 1,
      cronMinIntervalSeconds: 300,
      environments: 1,
      webhookEndpoints: 0,
      apiRateLimit: 60,
    },
    governance: {
      rbac: "none",
      auditLogs: false,
      ssoSaml: false,
      aiMessagesPerDay: 20,
      aiByok: false,
    },
    cta: { label: "Get started free", href: "/login?redirect=/app/upgrade" },
    highlighted: false,
  },
  starter: {
    name: "Starter",
    description:
      "For small teams shipping to production.\nEverything you need to go live.",
    prices: { monthly: 1900, yearly: 18_000 },
    trial: false,
    creditCardRequired: true,
    computeCredit: "50,000 runs/mo",
    features: [
      "3 projects per org",
      "3 members per org",
      "15 concurrent runs",
      "50,000 runs/mo",
      "14-day retention",
      "All regions",
      "Email support (72h)",
    ],
    limits: {
      organizations: 2,
      projectsPerOrg: 3,
      membersPerOrg: 3,
      runsPerMonth: 50_000,
      concurrentRuns: 15,
      retentionDays: 14,
      regions: "All",
      spendingLimits: "$100 default, configurable",
      overagePerThousandRuns: 40,
      executionModes: "HTTP + worker",
      workflowSteps: 25,
      approvalGates: false,
      subWorkflows: false,
      jobChaining: false,
      canaryDeployments: false,
      scheduledJobs: 5,
      cronMinIntervalSeconds: 60,
      environments: 1,
      webhookEndpoints: 3,
      apiRateLimit: 300,
    },
    governance: {
      rbac: "basic",
      auditLogs: false,
      ssoSaml: false,
      aiMessagesPerDay: 100,
      aiByok: false,
    },
    cta: { label: "Get started", href: "/login?redirect=/app/upgrade" },
    highlighted: false,
  },
  pro: {
    name: "Pro",
    description:
      "For growing teams and workflows.\nOrchestration and cost controls built in.",
    prices: { monthly: 9900, yearly: 94_800 },
    trial: false,
    creditCardRequired: true,
    computeCredit: "1M runs/mo",
    features: [
      "10 projects per org",
      "10 members per org",
      "100 concurrent runs",
      "1M runs/mo",
      "30-day retention",
      "All regions",
      "Approval gates and sub-workflows",
      "Priority support (24h)",
    ],
    limits: {
      organizations: 5,
      projectsPerOrg: 10,
      membersPerOrg: 10,
      runsPerMonth: 1_000_000,
      concurrentRuns: 100,
      retentionDays: 30,
      regions: "All",
      spendingLimits: "$200 default, configurable",
      overagePerThousandRuns: 20,
      executionModes: "HTTP + worker",
      workflowSteps: 100,
      approvalGates: true,
      subWorkflows: true,
      jobChaining: true,
      canaryDeployments: false,
      scheduledJobs: 25,
      cronMinIntervalSeconds: 30,
      environments: 3,
      webhookEndpoints: 10,
      apiRateLimit: 1000,
    },
    governance: {
      rbac: "full",
      auditLogs: false,
      ssoSaml: false,
      aiMessagesPerDay: 500,
      aiByok: true,
    },
    cta: { label: "Get started", href: "/login?redirect=/app/upgrade" },
    highlighted: true,
    badge: "Most popular",
  },
  scale: {
    name: "Scale",
    description:
      "For high-volume production workloads.\nAudit trails, canary deploys, analytics.",
    prices: { monthly: 29_900, yearly: 286_800 },
    trial: false,
    creditCardRequired: true,
    computeCredit: "5M runs/mo",
    features: [
      "50 projects per org",
      "50 members per org",
      "300 concurrent runs",
      "5M runs/mo",
      "60-day retention",
      "Audit logs",
      "Canary deployments",
      "Priority support + Slack (8h)",
    ],
    limits: {
      organizations: 10,
      projectsPerOrg: 50,
      membersPerOrg: 50,
      runsPerMonth: 5_000_000,
      concurrentRuns: 300,
      retentionDays: 60,
      regions: "All",
      spendingLimits: "$500 default, configurable",
      overagePerThousandRuns: 6,
      executionModes: "HTTP + worker",
      workflowSteps: 500,
      approvalGates: true,
      subWorkflows: true,
      jobChaining: true,
      canaryDeployments: true,
      scheduledJobs: 100,
      cronMinIntervalSeconds: 1,
      environments: 10,
      webhookEndpoints: 25,
      apiRateLimit: 3000,
    },
    governance: {
      rbac: "full",
      auditLogs: true,
      ssoSaml: false,
      aiMessagesPerDay: 1000,
      aiByok: true,
    },
    cta: {
      label: "Upgrade to Scale",
      href: "/login?redirect=/app/upgrade",
    },
    highlighted: false,
  },
  business: {
    name: "Business",
    description:
      "For organizations with compliance and scale needs.\nSSO, advanced RBAC, unlimited environments.",
    prices: { monthly: 49_900, yearly: 478_800 },
    trial: false,
    creditCardRequired: true,
    computeCredit: "25M runs/mo",
    features: [
      "Unlimited projects, members, environments",
      "500 concurrent runs",
      "25M runs/mo",
      "90-day retention",
      "SSO/SAML and SCIM",
      "Advanced RBAC and IP allowlisting",
      "SIEM export",
      "Sub-second cron scheduling",
      "Priority support + Slack (8h)",
    ],
    limits: {
      organizations: null,
      projectsPerOrg: null,
      membersPerOrg: null,
      runsPerMonth: 25_000_000,
      concurrentRuns: 500,
      retentionDays: 90,
      regions: "All",
      spendingLimits: "$1,500 default, configurable",
      overagePerThousandRuns: 3,
      executionModes: "HTTP + worker",
      workflowSteps: null,
      approvalGates: true,
      subWorkflows: true,
      jobChaining: true,
      canaryDeployments: true,
      scheduledJobs: null,
      cronMinIntervalSeconds: 0,
      environments: null,
      webhookEndpoints: null,
      apiRateLimit: null,
    },
    governance: {
      rbac: "advanced",
      auditLogs: true,
      ssoSaml: true,
      aiMessagesPerDay: null,
      aiByok: true,
    },
    cta: {
      label: "Upgrade to Business",
      href: "/login?redirect=/app/upgrade",
    },
    highlighted: false,
  },
  enterprise: {
    name: "Enterprise",
    description:
      "For mission-critical infrastructure.\nDedicated compute, SLAs, compliance.",
    prices: { monthly: -1, yearly: -1 },
    trial: false,
    creditCardRequired: false,
    computeCredit: "Unlimited (custom contract)",
    features: [
      "Everything in Business",
      "Dedicated compute (isolated Fly org)",
      "Static IPs and VPC peering",
      "Data residency",
      "Custom RBAC",
      "Reserved capacity and priority queue",
      "99.9% SLA with credits",
      "Dedicated support engineer",
    ],
    limits: {
      organizations: null,
      projectsPerOrg: null,
      membersPerOrg: null,
      runsPerMonth: null,
      concurrentRuns: null,
      retentionDays: null,
      regions: "All",
      spendingLimits: "Custom",
      overagePerThousandRuns: 3,
      executionModes: "HTTP + worker",
      workflowSteps: null,
      approvalGates: true,
      subWorkflows: true,
      jobChaining: true,
      canaryDeployments: true,
      scheduledJobs: null,
      cronMinIntervalSeconds: 0,
      environments: null,
      webhookEndpoints: null,
      apiRateLimit: null,
    },
    governance: {
      rbac: "full",
      auditLogs: true,
      ssoSaml: true,
      aiMessagesPerDay: null,
      aiByok: true,
    },
    cta: { label: "Contact sales", href: "/contact" },
    highlighted: false,
  },
};

export function formatPlanPrice(
  plan: Plan,
  interval: "monthly" | "yearly"
): string {
  const price = plan.prices[interval];
  if (price === 0) {
    return "$0";
  }
  if (price < 0) {
    return "Custom";
  }
  if (interval === "yearly") {
    return formatPriceWithCents(Math.round(price / 12));
  }
  return formatPriceWithCents(price);
}

export function formatPrice(cents: number, currency = "USD"): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency,
    maximumFractionDigits: 0,
  }).format(cents / 100);
}

export function formatPriceWithCents(cents: number, currency = "USD"): string {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency,
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(cents / 100);
}
