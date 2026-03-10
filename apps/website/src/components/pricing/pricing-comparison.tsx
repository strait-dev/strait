import { PLANS } from "@strait/billing/products";

import PricingComparisonClient from "./pricing-comparison.client";
import type {
  PlanKey,
  PricingComparisonClientProps,
  PricingComparisonHeader,
  PricingSection,
  PricingSectionRow,
} from "./pricing-comparison.types";

type FeatureRowDefinition =
  | {
      label: string;
      category: string;
      type: "text";
      match: (feature: string) => boolean;
      format?: (value: string) => string;
      hidden?: boolean;
    }
  | {
      label: string;
      category: string;
      type: "boolean";
      match: (feature: string) => boolean;
      hidden?: boolean;
    };

const plansOrder: PlanKey[] = ["personal", "pro"];

const normalizeFeatures = (features: readonly string[]): string[] =>
  features.map((feature) => feature.trim());

const planDefinitions = {
  personal: {
    name: PLANS.personal.name,
    highlight: false,
    monthlyPrice: PLANS.personal.prices.monthly,
    yearlyPrice: PLANS.personal.prices.yearly,
    features: normalizeFeatures(PLANS.personal.features),
  },
  pro: {
    name: PLANS.pro.name,
    highlight: true,
    monthlyPrice: PLANS.pro.prices.monthly,
    yearlyPrice: PLANS.pro.prices.yearly,
    features: normalizeFeatures(PLANS.pro.features),
  },
} satisfies Record<
  PlanKey,
  {
    name: string;
    highlight: boolean;
    monthlyPrice: number;
    yearlyPrice: number;
    features: string[];
  }
>;

const contains = (keyword: string) => (feature: string) =>
  feature.toLowerCase().includes(keyword);

const FEATURE_ROWS: FeatureRowDefinition[] = [
  {
    label: "Sessions per month",
    category: "Writing & AI",
    type: "text",
    match: (feature) => feature.toLowerCase().includes("sessions/month"),
  },
  {
    label: "Messages per session",
    category: "Writing & AI",
    type: "text",
    match: contains("messages per session"),
  },
  {
    label: "Style profiles",
    category: "Writing & AI",
    type: "text",
    match: (feature) => feature.toLowerCase().includes("style profile"),
  },
  {
    label: "Workspaces",
    category: "Organization",
    type: "text",
    match: (feature) => feature.toLowerCase().includes("workspace"),
  },
  {
    label: "Languages",
    category: "Organization",
    type: "text",
    match: (feature) => feature.toLowerCase().includes("language"),
  },
  {
    label: "Support",
    category: "Support",
    type: "text",
    match: contains("support"),
  },
  {
    label: "Early access",
    category: "Support",
    type: "boolean",
    match: contains("early access"),
  },
];

const buildPricingData = (
  header: PricingComparisonHeader
): PricingComparisonClientProps => {
  const plans = plansOrder.map((key) => ({
    key,
    name: planDefinitions[key].name,
    highlight: planDefinitions[key].highlight,
    prices: {
      monthly: planDefinitions[key].monthlyPrice,
      yearly: planDefinitions[key].yearlyPrice,
    },
    features: planDefinitions[key].features,
  }));

  const sectionsMap = new Map<string, PricingSectionRow[]>();

  for (const row of FEATURE_ROWS) {
    const entries = plansOrder.reduce<Record<PlanKey, string | boolean | null>>(
      (acc, planKey) => {
        const featureMatch = planDefinitions[planKey].features.find((feature) =>
          row.match(feature)
        );

        if (row.type === "text") {
          acc[planKey] = (() => {
            if (!featureMatch) {
              return null;
            }
            if (row.format) {
              return row.format(featureMatch);
            }
            return featureMatch;
          })();
        } else {
          acc[planKey] = Boolean(featureMatch);
        }

        return acc;
      },
      {
        personal: row.type === "text" ? null : false,
        pro: row.type === "text" ? null : false,
      }
    );

    if (!sectionsMap.has(row.category)) {
      sectionsMap.set(row.category, []);
    }

    if (row.hidden) {
      continue;
    }

    sectionsMap.get(row.category)?.push({
      label: row.label,
      type: row.type,
      values: entries,
    });
  }

  const sections: PricingSection[] = Array.from(sectionsMap.entries())
    .filter(([, rows]) => rows.length > 0)
    .map(([name, rows]) => ({
      name,
      rows,
    }));

  return {
    header,
    plans,
    sections,
  };
};

const PricingComparison = () => {
  const header: PricingComparisonHeader = {
    badge: "Compare",
    title: "Compare plans in detail",
    description:
      "See exactly what changes between Personal and Pro across writing limits, organization, and support.",
  };

  const pricingData = buildPricingData(header);

  return <PricingComparisonClient {...pricingData} />;
};

export default PricingComparison;
