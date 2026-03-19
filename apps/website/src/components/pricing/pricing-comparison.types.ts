export type PlanKey = "free" | "starter" | "pro" | "enterprise";

export type PlanSummary = {
  key: PlanKey;
  name: string;
  highlight: boolean;
  badge?: string;
  prices: { monthly: number; yearly: number };
  cta: { label: string; href: string };
};

export type PricingSectionRow = {
  label: string;
  type: "text" | "boolean";
  values: Record<PlanKey, string | boolean | null>;
  tooltip?: string;
  hidden?: boolean;
};

export type PricingSection = {
  name: string;
  rows: PricingSectionRow[];
};

export type PricingComparisonHeader = {
  badge: string;
  title: string;
  description: string;
};

export type PricingComparisonClientProps = {
  header: PricingComparisonHeader;
  plans: PlanSummary[];
  sections: PricingSection[];
};
