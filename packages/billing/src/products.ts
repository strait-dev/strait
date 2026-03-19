export type PlanKey = "free" | "starter" | "pro" | "enterprise";

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

export const PLAN_KEYS: PlanKey[] = ["free", "starter", "pro", "enterprise"];

export const PLANS: Record<PlanKey, Plan> = {
  free: {
    name: "Free",
    description: "For side projects and experimentation.",
    prices: { monthly: 0, yearly: 0 },
    trial: false,
    creditCardRequired: false,
    computeCredit: "$0 included",
    features: [
      "1 organization",
      "3 projects",
      "1 member",
      "100 runs/day",
      "1 concurrent run",
      "7-day retention",
      "Community support",
    ],
    limits: {
      organizations: 1,
      projectsPerOrg: 3,
      membersPerOrg: 1,
      runsPerDay: 100,
      concurrentRuns: 1,
      retentionDays: 7,
      regions: "1 (iad)",
      spendingLimits: "Hard cap",
      overagePerThousandRuns: null,
    },
    governance: {
      rbac: "none",
      auditLogs: false,
      ssoSaml: false,
      aiMessagesPerDay: 10,
      aiByok: false,
    },
    cta: { label: "Get started free", href: "/get-started?plan=free" },
    highlighted: false,
  },
  starter: {
    name: "Starter",
    description: "For small teams shipping to production.",
    prices: { monthly: 1999, yearly: 19999 },
    trial: true,
    creditCardRequired: true,
    computeCredit: "$5 included",
    features: [
      "3 organizations",
      "10 projects per org",
      "5 members per org",
      "1,000 runs/day",
      "5 concurrent runs",
      "30-day retention",
      "Email support",
    ],
    limits: {
      organizations: 3,
      projectsPerOrg: 10,
      membersPerOrg: 5,
      runsPerDay: 1000,
      concurrentRuns: 5,
      retentionDays: 30,
      regions: "3",
      spendingLimits: "Hard cap",
      overagePerThousandRuns: 200,
    },
    governance: {
      rbac: "basic",
      auditLogs: false,
      ssoSaml: false,
      aiMessagesPerDay: 50,
      aiByok: false,
    },
    cta: { label: "Start free trial", href: "/get-started?plan=starter" },
    highlighted: false,
    badge: "14-day trial",
  },
  pro: {
    name: "Pro",
    description: "For growing teams that need scale and governance.",
    prices: { monthly: 4999, yearly: 49999 },
    trial: true,
    creditCardRequired: true,
    computeCredit: "$20 included",
    features: [
      "Unlimited organizations",
      "Unlimited projects",
      "25 members per org",
      "10,000 runs/day",
      "25 concurrent runs",
      "90-day retention",
      "Priority support",
    ],
    limits: {
      organizations: null,
      projectsPerOrg: null,
      membersPerOrg: 25,
      runsPerDay: 10000,
      concurrentRuns: 25,
      retentionDays: 90,
      regions: "6",
      spendingLimits: "Configurable",
      overagePerThousandRuns: 150,
    },
    governance: {
      rbac: "full",
      auditLogs: true,
      ssoSaml: false,
      aiMessagesPerDay: null,
      aiByok: true,
    },
    cta: { label: "Start free trial", href: "/get-started?plan=pro" },
    highlighted: true,
    badge: "Most popular",
  },
  enterprise: {
    name: "Enterprise",
    description: "For organizations with custom security and compliance needs.",
    prices: { monthly: -1, yearly: -1 },
    trial: false,
    creditCardRequired: false,
    computeCredit: "Custom",
    features: [
      "Unlimited everything",
      "Unlimited members",
      "Unlimited runs",
      "Unlimited concurrent runs",
      "Custom retention",
      "Dedicated support",
      "SSO/SAML",
    ],
    limits: {
      organizations: null,
      projectsPerOrg: null,
      membersPerOrg: null,
      runsPerDay: null,
      concurrentRuns: null,
      retentionDays: null,
      regions: "All",
      spendingLimits: "Custom",
      overagePerThousandRuns: null,
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
  if (price === 0) return "$0";
  if (price < 0) return "Custom";
  if (interval === "yearly") return formatPriceWithCents(Math.round(price / 12));
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
