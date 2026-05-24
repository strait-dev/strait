export const PLAN_LIMITS = {
  free: {
    runsPerMonth: 5000,
    members: 1,
    priceMonthly: 0,
    overagePer1K: 0.5,
  },
  starter: {
    runsPerMonth: 50_000,
    members: 3,
    priceMonthly: 19,
    overagePer1K: 0.4,
  },
  pro: {
    runsPerMonth: 1_000_000,
    members: 10,
    priceMonthly: 99,
    overagePer1K: 0.2,
  },
  scale: {
    runsPerMonth: 5_000_000,
    members: 50,
    priceMonthly: 299,
    overagePer1K: 0.06,
  },
  business: {
    runsPerMonth: 25_000_000,
    members: -1,
    priceMonthly: 499,
    overagePer1K: 0.03,
  },
  enterprise: {
    runsPerMonth: -1,
    members: -1,
    priceMonthly: -1,
    overagePer1K: 0.03,
  },
} as const;

export type PlanTier = keyof typeof PLAN_LIMITS;

export type EnterpriseTier =
  | "enterprise_starter"
  | "enterprise_growth"
  | "enterprise_large";

export const ENTERPRISE_TIERS: Record<
  EnterpriseTier,
  {
    name: string;
    annualCommitment: number;
    monthlyEquivalent: number;
    includedCreditMonthly: number;
    computeDiscountPct: number;
    slaPct: number;
  }
> = {
  enterprise_starter: {
    name: "Starter Enterprise",
    annualCommitment: 18_000,
    monthlyEquivalent: 1500,
    includedCreditMonthly: 1000,
    computeDiscountPct: 10,
    slaPct: 99.9,
  },
  enterprise_growth: {
    name: "Growth Enterprise",
    annualCommitment: 48_000,
    monthlyEquivalent: 4000,
    includedCreditMonthly: 2500,
    computeDiscountPct: 15,
    slaPct: 99.95,
  },
  enterprise_large: {
    name: "Large Enterprise",
    annualCommitment: 96_000,
    monthlyEquivalent: 8000,
    includedCreditMonthly: 0,
    computeDiscountPct: 20,
    slaPct: 99.95,
  },
};
