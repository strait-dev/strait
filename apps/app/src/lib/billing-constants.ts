export const PLAN_LIMITS = {
  free: {
    runsPerDay: 5000,
    computeCreditUsd: 5,
    members: 3,
    priceMonthly: 0,
  },
  starter: {
    runsPerDay: 25_000,
    computeCreditUsd: 19.99,
    members: 10,
    priceMonthly: 19.99,
  },
  pro: {
    runsPerDay: 100_000,
    computeCreditUsd: 49.99,
    members: 25,
    priceMonthly: 49.99,
  },
  scale: {
    runsPerDay: -1,
    computeCreditUsd: 99,
    members: 50,
    priceMonthly: 99,
  },
  enterprise: {
    runsPerDay: -1,
    computeCreditUsd: -1,
    members: -1,
    priceMonthly: -1,
  },
} as const;

export type PlanTier = keyof typeof PLAN_LIMITS;

/** Enterprise sub-tier identifiers (commercial terms, not features). */
export type EnterpriseTier =
  | "enterprise_starter"
  | "enterprise_growth"
  | "enterprise_large";

/** Enterprise sub-tier display info. */
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
