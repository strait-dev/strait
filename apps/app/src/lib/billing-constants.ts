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
  enterprise: {
    runsPerDay: -1,
    computeCreditUsd: -1,
    members: -1,
    priceMonthly: -1,
  },
} as const;

export type PlanTier = keyof typeof PLAN_LIMITS;
