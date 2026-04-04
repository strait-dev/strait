import { Badge } from "@strait/ui/components/badge";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { useState } from "react";
import { PLAN_LIMITS, type PlanTier } from "@/lib/billing-constants";

const PLAN_TIERS: PlanTier[] = ["free", "starter", "pro"];

const PLAN_LABELS: Record<PlanTier, string> = {
  free: "Free",
  starter: "Starter",
  pro: "Pro",
  scale: "Scale",
  enterprise: "Enterprise",
};

const OVERAGE_RATE_PER_1K_RUNS: Record<PlanTier, number> = {
  free: 0,
  starter: 1.0,
  pro: 0.75,
  scale: 0.5,
  enterprise: 0,
};

const COMPUTE_RATE_PER_HOUR: Record<PlanTier, number> = {
  free: 0,
  starter: 0.1,
  pro: 0.08,
  scale: 0.06,
  enterprise: 0,
};

const MEMBER_OVERAGE_PER_SEAT: Record<PlanTier, number> = {
  free: 0,
  starter: 5,
  pro: 10,
  scale: 10,
  enterprise: 0,
};

const calculatePlanCost = (
  tier: PlanTier,
  runsPerDay: number,
  computeHours: number,
  members: number
): number => {
  const limits = PLAN_LIMITS[tier];

  if (limits.priceMonthly < 0) {
    return -1;
  }

  let cost = limits.priceMonthly;

  // Overage runs cost
  if (limits.runsPerDay > 0 && runsPerDay > limits.runsPerDay) {
    const dailyOverage = runsPerDay - limits.runsPerDay;
    const monthlyOverageRuns = dailyOverage * 30;
    cost += (monthlyOverageRuns / 1000) * OVERAGE_RATE_PER_1K_RUNS[tier];
  }

  // Compute overage
  const includedHours =
    limits.computeCreditUsd > 0
      ? limits.computeCreditUsd / (COMPUTE_RATE_PER_HOUR[tier] || 1)
      : 0;
  if (computeHours > includedHours && COMPUTE_RATE_PER_HOUR[tier] > 0) {
    cost += (computeHours - includedHours) * COMPUTE_RATE_PER_HOUR[tier];
  }

  // Member overage
  if (limits.members > 0 && members > limits.members) {
    cost += (members - limits.members) * MEMBER_OVERAGE_PER_SEAT[tier];
  }

  return cost;
};

const findRecommendedPlan = (costs: { tier: PlanTier; cost: number }[]) => {
  const validCosts = costs.filter((c) => c.cost >= 0);
  if (validCosts.length === 0) {
    return null;
  }

  // Find cheapest non-free plan that covers the usage, or free if it works
  let best = validCosts[0];
  for (const c of validCosts) {
    if (c.cost < best.cost) {
      best = c;
    }
  }
  return best.tier;
};

const PricingCalculator = () => {
  const [runsPerDay, setRunsPerDay] = useState(5000);
  const [computeHours, setComputeHours] = useState(50);
  const [members, setMembers] = useState(3);

  const planCosts = PLAN_TIERS.map((tier) => ({
    tier,
    cost: calculatePlanCost(tier, runsPerDay, computeHours, members),
  }));

  const recommended = findRecommendedPlan(planCosts);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="font-medium text-sm">
          Estimate Your Monthly Cost
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* Runs per day */}
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <label
              className="text-muted-foreground text-sm"
              htmlFor="calc-runs"
            >
              Runs per day
            </label>
            <span className="font-mono text-sm tabular-nums">
              {runsPerDay.toLocaleString()}
            </span>
          </div>
          <input
            className="w-full accent-foreground"
            id="calc-runs"
            max={200_000}
            min={0}
            onChange={(e) => setRunsPerDay(Number(e.target.value))}
            step={1000}
            type="range"
            value={runsPerDay}
          />
          <div className="flex justify-between text-muted-foreground/60 text-xs">
            <span>0</span>
            <span>200,000</span>
          </div>
        </div>

        {/* Compute hours */}
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <label
              className="text-muted-foreground text-sm"
              htmlFor="calc-compute"
            >
              Compute hours / month
            </label>
            <span className="font-mono text-sm tabular-nums">
              {computeHours}
            </span>
          </div>
          <input
            className="w-full accent-foreground"
            id="calc-compute"
            max={500}
            min={0}
            onChange={(e) => setComputeHours(Number(e.target.value))}
            step={10}
            type="range"
            value={computeHours}
          />
          <div className="flex justify-between text-muted-foreground/60 text-xs">
            <span>0</span>
            <span>500</span>
          </div>
        </div>

        {/* Team members */}
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <label
              className="text-muted-foreground text-sm"
              htmlFor="calc-members"
            >
              Team members
            </label>
            <span className="font-mono text-sm tabular-nums">{members}</span>
          </div>
          <input
            className="w-full accent-foreground"
            id="calc-members"
            max={50}
            min={1}
            onChange={(e) => setMembers(Number(e.target.value))}
            step={1}
            type="range"
            value={members}
          />
          <div className="flex justify-between text-muted-foreground/60 text-xs">
            <span>1</span>
            <span>50</span>
          </div>
        </div>

        {/* Cost breakdown */}
        <div className="space-y-2 border-border border-t pt-4">
          {planCosts.map(({ tier, cost }) => (
            <div
              className="flex items-center justify-between rounded-md px-3 py-2 text-sm transition-colors hover:bg-muted/50"
              key={tier}
            >
              <div className="flex items-center gap-2">
                <span className="font-medium">{PLAN_LABELS[tier]}</span>
                {recommended === tier ? (
                  <Badge variant="success-light">Best value</Badge>
                ) : null}
              </div>
              <span className="font-mono tabular-nums">
                {cost < 0 ? "Custom" : `$${cost.toFixed(2)}/mo`}
              </span>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
};

export default PricingCalculator;
