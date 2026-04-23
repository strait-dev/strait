export type PlanKey = "free" | "starter" | "pro" | "scale" | "enterprise";

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
    runsPerDay: number | null;
    concurrentRuns: number | null;
    retentionDays: number | null;
    regions: string;
    spendingLimits: string;
    overagePerThousandRuns: number | null;
    executionModes: string;
    workflowSteps: number | null;
    approvalGates: boolean;
    subWorkflows: boolean;
    jobChaining: boolean;
    canaryDeployments: boolean;
    scheduledJobs: number | null;
    webhookEndpoints: number | null;
    apiRateLimit: number | null;
  };
  governance: {
    rbac: "none" | "basic" | "full";
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
    computeCredit: "$1/mo",
    features: [
      "1 project",
      "1 member",
      "5 concurrent runs",
      "Unlimited daily runs",
      "1-day retention",
      "Community support",
    ],
    limits: {
      organizations: 1,
      projectsPerOrg: 1,
      membersPerOrg: 1,
      runsPerDay: null,
      concurrentRuns: 5,
      retentionDays: 1,
      regions: "1 (iad)",
      spendingLimits: "Hard cap",
      overagePerThousandRuns: null,
      executionModes: "Managed only",
      workflowSteps: 10,
      approvalGates: false,
      subWorkflows: false,
      jobChaining: false,
      canaryDeployments: false,
      scheduledJobs: 10,
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
    prices: { monthly: 1999, yearly: 19_999 },
    trial: false,
    creditCardRequired: true,
    computeCredit: "$19.99/mo",
    features: [
      "3 projects per org",
      "5 members per org",
      "25 concurrent runs",
      "Unlimited daily runs",
      "7-day retention",
      "6 regions",
      "Email support (72h)",
    ],
    limits: {
      organizations: 2,
      projectsPerOrg: 3,
      membersPerOrg: 5,
      runsPerDay: null,
      concurrentRuns: 25,
      retentionDays: 7,
      regions: "6",
      spendingLimits: "Configurable",
      overagePerThousandRuns: 200,
      executionModes: "Managed only",
      workflowSteps: 50,
      approvalGates: false,
      subWorkflows: false,
      jobChaining: false,
      canaryDeployments: false,
      scheduledJobs: 25,
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
    badge: undefined,
  },
  pro: {
    name: "Pro",
    description:
      "For growing teams and workflows.\nOrchestration and cost controls built in.",
    prices: { monthly: 4999, yearly: 49_999 },
    trial: false,
    creditCardRequired: true,
    computeCredit: "$49.99/mo",
    features: [
      "10 projects per org",
      "10 members per org",
      "100 concurrent runs",
      "Managed + HTTP execution",
      "30-day retention",
      "All regions + multi-region",
      "Approval gates & sub-workflows",
      "Priority support (24h)",
    ],
    limits: {
      organizations: 5,
      projectsPerOrg: 10,
      membersPerOrg: 10,
      runsPerDay: null,
      concurrentRuns: 100,
      retentionDays: 30,
      regions: "All",
      spendingLimits: "Configurable",
      overagePerThousandRuns: 200,
      executionModes: "Managed + HTTP",
      workflowSteps: 250,
      approvalGates: true,
      subWorkflows: true,
      jobChaining: true,
      canaryDeployments: false,
      scheduledJobs: 100,
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
    prices: { monthly: 9900, yearly: 99_000 },
    trial: false,
    creditCardRequired: true,
    computeCredit: "$99/mo",
    features: [
      "50 projects per org",
      "50 members per org",
      "500 concurrent runs",
      "Managed + HTTP execution",
      "60-day retention",
      "Audit logs",
      "Canary deployments",
      "Priority support + Slack (8h)",
    ],
    limits: {
      organizations: 10,
      projectsPerOrg: 50,
      membersPerOrg: 50,
      runsPerDay: null,
      concurrentRuns: 500,
      retentionDays: 60,
      regions: "All",
      spendingLimits: "Configurable",
      overagePerThousandRuns: 200,
      executionModes: "Managed + HTTP",
      workflowSteps: 1000,
      approvalGates: true,
      subWorkflows: true,
      jobChaining: true,
      canaryDeployments: true,
      scheduledJobs: 500,
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
  enterprise: {
    name: "Enterprise",
    description:
      "For mission-critical infrastructure.\nDedicated compute, SLAs, compliance.",
    prices: { monthly: -1, yearly: -1 },
    trial: false,
    creditCardRequired: false,
    computeCredit: "Starting at $1,000/mo",
    features: [
      "Everything in Scale",
      "SSO and SCIM provisioning",
      "Dedicated isolated compute",
      "VPC peering and static IPs",
      "99.9% SLA with credits",
      "90-day data retention",
      "Dedicated support engineer",
      "Custom RBAC and IP allowlisting",
      "SIEM export and data residency",
      "Reserved capacity and priority queue",
    ],
    limits: {
      organizations: null,
      projectsPerOrg: null,
      membersPerOrg: null,
      runsPerDay: null,
      concurrentRuns: null,
      retentionDays: 90,
      regions: "All",
      spendingLimits: "Custom",
      overagePerThousandRuns: null,
      executionModes: "Managed + HTTP",
      workflowSteps: null,
      approvalGates: true,
      subWorkflows: true,
      jobChaining: true,
      canaryDeployments: true,
      scheduledJobs: null,
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
