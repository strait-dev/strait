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
    getter: (key: PlanKey) => string | boolean | null,
    tooltip?: string
  ): PricingSectionRow => ({
    label,
    type,
    values: Object.fromEntries(keys.map((k) => [k, getter(k)])) as Record<
      PlanKey,
      string | boolean | null
    >,
    ...(tooltip ? { tooltip } : {}),
  });

  return [
    {
      name: "Scale",
      rows: [
        row(
          "Organizations",
          "text",
          (k) => formatLimit(PLANS[k].limits.organizations),
          "Separate billing entities. Each org has its own subscription, members, and projects."
        ),
        row(
          "Projects per org",
          "text",
          (k) => formatLimit(PLANS[k].limits.projectsPerOrg),
          "Logical groupings for your jobs and workflows within an organization."
        ),
        row(
          "Members per org",
          "text",
          (k) => formatLimit(PLANS[k].limits.membersPerOrg),
          "Team members who can access the organization's projects and resources."
        ),
        row(
          "Runs per day",
          "text",
          (k) => formatLimit(PLANS[k].limits.runsPerDay),
          "Maximum number of job executions across all projects in a 24-hour window."
        ),
        row(
          "Concurrent runs",
          "text",
          (k) => formatLimit(PLANS[k].limits.concurrentRuns),
          "How many jobs can execute simultaneously across the organization."
        ),
        row(
          "Retention",
          "text",
          (k) => {
            const days = PLANS[k].limits.retentionDays;
            if (days === null) {
              return "Custom";
            }
            return `${days} days`;
          },
          "How long job logs, run history, and execution data are stored."
        ),
        row(
          "Regions",
          "text",
          (k) => PLANS[k].limits.regions,
          "Geographic locations where your jobs can execute. More regions reduce latency for global teams."
        ),
      ],
    },
    {
      name: "AI & Compute",
      rows: [
        row(
          "Compute credit",
          "text",
          (k) => PLANS[k].computeCredit,
          "Monthly infrastructure budget for managed execution. Credit equals your subscription price on paid plans."
        ),
        row(
          "Spending limits",
          "text",
          (k) => PLANS[k].limits.spendingLimits,
          "Controls on compute spending. Hard cap prevents overage; configurable lets you set your own threshold."
        ),
        row(
          "Overage per 1,000 runs",
          "text",
          (k) => {
            const cents = PLANS[k].limits.overagePerThousandRuns;
            if (cents === null) {
              return null;
            }
            return `$${(cents / 100).toFixed(2)}`;
          },
          "Cost per additional 1,000 runs once your included compute credit is exhausted."
        ),
      ],
    },
    {
      name: "Governance",
      rows: [
        row(
          "RBAC",
          "text",
          (k) => formatRbac(PLANS[k].governance.rbac),
          "Role-Based Access Control. Basic provides predefined roles; Full allows custom roles and granular permissions."
        ),
        row(
          "Audit logs",
          "boolean",
          (k) => PLANS[k].governance.auditLogs,
          "Immutable log of all actions taken within the organization for compliance and debugging."
        ),
        row(
          "SSO/SAML",
          "boolean",
          (k) => PLANS[k].governance.ssoSaml,
          "Single Sign-On via SAML 2.0 for centralized authentication through your identity provider."
        ),
      ],
    },
    {
      name: "Support",
      rows: [
        row(
          "Support level",
          "text",
          (k) => supportLevel(k),
          "How you get help. Community is forum-based. Email is 1-business-day SLA. Priority is 4-hour SLA. Dedicated is a named contact."
        ),
      ],
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
