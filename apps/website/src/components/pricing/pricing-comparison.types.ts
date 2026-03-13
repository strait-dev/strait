export type PlanKey = "personal" | "pro";

export type PlanSummary = {
  key: PlanKey;
  name: string;
  highlight: boolean;
  prices: {
    monthly: number;
    yearly: number;
  };
  features: string[];
};

export type PricingSectionRow = {
  label: string;
  type: "text" | "boolean";
  values: Record<PlanKey, string | boolean | null>;
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
