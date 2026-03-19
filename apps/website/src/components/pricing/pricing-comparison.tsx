import { PLAN_KEYS, PLANS } from "@strait/billing/products";

import { dashboardHref } from "@/lib/urls.ts";

import PricingComparisonClient from "./pricing-comparison.client.tsx";
import type {
  PlanKey,
  PlanSummary,
  PricingComparisonClientProps,
  PricingComparisonHeader,
  PricingSection,
  PricingSectionRow,
} from "./pricing-comparison.types.ts";

function formatLimit(value: number | null): string {
  if (value === null) {
    return "Unlimited";
  }
  return value.toLocaleString("en-US");
}

function formatRbac(value: "none" | "basic" | "full"): string {
  if (value === "none") {
    return "None";
  }
  if (value === "basic") {
    return "Basic";
  }
  return "Full";
}

function supportLevel(key: PlanKey): string {
  switch (key) {
    case "free":
      return "Community";
    case "starter":
      return "Email";
    case "pro":
      return "Priority";
    case "enterprise":
      return "Dedicated";
    default:
      return "Community";
  }
}

function buildSections(): PricingSection[] {
  const keys = PLAN_KEYS as readonly PlanKey[];

  const row = (
    label: string,
    type: "text" | "boolean",
    getter: (key: PlanKey) => string | boolean | null
  ): PricingSectionRow => ({
    label,
    type,
    values: Object.fromEntries(keys.map((k) => [k, getter(k)])) as Record<
      PlanKey,
      string | boolean | null
    >,
  });

  return [
    {
      name: "Scale",
      rows: [
        row("Organizations", "text", (k) =>
          formatLimit(PLANS[k].limits.organizations)
        ),
        row("Projects per org", "text", (k) =>
          formatLimit(PLANS[k].limits.projectsPerOrg)
        ),
        row("Members per org", "text", (k) =>
          formatLimit(PLANS[k].limits.membersPerOrg)
        ),
        row("Runs per day", "text", (k) =>
          formatLimit(PLANS[k].limits.runsPerDay)
        ),
        row("Concurrent runs", "text", (k) =>
          formatLimit(PLANS[k].limits.concurrentRuns)
        ),
        row("Retention", "text", (k) => {
          const days = PLANS[k].limits.retentionDays;
          if (days === null) {
            return "Custom";
          }
          return `${days} days`;
        }),
        row("Regions", "text", (k) => PLANS[k].limits.regions),
      ],
    },
    {
      name: "AI & Compute",
      rows: [
        row("Compute credit", "text", (k) => PLANS[k].computeCredit),
        row("Spending limits", "text", (k) => PLANS[k].limits.spendingLimits),
        row("Overage per 1,000 runs", "text", (k) => {
          const cents = PLANS[k].limits.overagePerThousandRuns;
          if (cents === null) {
            return null;
          }
          return `$${(cents / 100).toFixed(2)}`;
        }),
        row("AI Assistant messages", "text", (k) => {
          const count = PLANS[k].governance.aiMessagesPerDay;
          if (count === null) {
            return "Unlimited";
          }
          return `${count}/day`;
        }),
        row("AI BYOK", "boolean", (k) => PLANS[k].governance.aiByok),
      ],
    },
    {
      name: "Governance",
      rows: [
        row("RBAC", "text", (k) => formatRbac(PLANS[k].governance.rbac)),
        row("Audit logs", "boolean", (k) => PLANS[k].governance.auditLogs),
        row("SSO/SAML", "boolean", (k) => PLANS[k].governance.ssoSaml),
      ],
    },
    {
      name: "Support",
      rows: [row("Support level", "text", (k) => supportLevel(k))],
    },
  ];
}

function buildPlans(): PlanSummary[] {
  return PLAN_KEYS.map((key) => {
    const plan = PLANS[key];
    return {
      key: key as PlanKey,
      name: plan.name,
      highlight: plan.highlighted,
      badge: plan.badge,
      prices: plan.prices,
      cta: {
        label: plan.cta.label,
        href:
          key === "enterprise" ? plan.cta.href : dashboardHref(plan.cta.href),
      },
    };
  });
}

const PricingComparison = () => {
  const header: PricingComparisonHeader = {
    badge: "Compare",
    title: "Compare every feature",
    description:
      "All plans include every core feature. Paid tiers add scale, governance, and priority support.",
  };

  const data: PricingComparisonClientProps = {
    header,
    plans: buildPlans(),
    sections: buildSections(),
  };

  return <PricingComparisonClient {...data} />;
};

export default PricingComparison;
