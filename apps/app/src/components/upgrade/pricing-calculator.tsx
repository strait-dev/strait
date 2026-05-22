import { Badge } from "@strait/ui/components/badge";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { useState } from "react";
import { PLAN_LIMITS, type PlanTier } from "@/lib/billing-constants";

const PLAN_TIERS: PlanTier[] = ["free", "starter", "pro", "scale", "business"];

const PLAN_LABELS: Record<PlanTier, string> = {
  free: "Free",
  starter: "Starter",
  pro: "Pro",
  scale: "Scale",
  business: "Business",
  enterprise: "Enterprise",
};

const MEMBER_OVERAGE_PER_SEAT: Record<PlanTier, number> = {
  free: 0,
  starter: 5,
  pro: 10,
  scale: 10,
  business: 0,
  enterprise: 0,
};

const calculatePlanCost = (
  tier: PlanTier,
  runsPerMonth: number,
  members: number
): number => {
  const limits = PLAN_LIMITS[tier];

  if (limits.priceMonthly < 0) {
    return -1;
  }

  let cost = limits.priceMonthly;

  if (limits.runsPerMonth > 0 && runsPerMonth > limits.runsPerMonth) {
    const overageRuns = runsPerMonth - limits.runsPerMonth;
    cost += (overageRuns / 1000) * limits.overagePer1K;
  }

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

  let best = validCosts[0];
  for (const c of validCosts) {
    if (c.cost < best.cost) {
      best = c;
    }
  }
  return best.tier;
};

const PricingCalculator = () => {
  const [runsPerMonth, setRunsPerMonth] = useState(50_000);
  const [members, setMembers] = useState(3);

  const planCosts = PLAN_TIERS.map((tier) => ({
    tier,
    cost: calculatePlanCost(tier, runsPerMonth, members),
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
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <label
              className="text-muted-foreground text-sm"
              htmlFor="calc-runs"
            >
              Runs per month
            </label>
            <span className="font-mono text-sm tabular-nums">
              {runsPerMonth.toLocaleString()}
            </span>
          </div>
          <input
            className="w-full accent-foreground"
            id="calc-runs"
            max={30_000_000}
            min={0}
            onChange={(e) => setRunsPerMonth(Number(e.target.value))}
            step={50_000}
            type="range"
            value={runsPerMonth}
          />
          <div className="flex justify-between text-muted-foreground/60 text-xs">
            <span>0</span>
            <span>30M</span>
          </div>
        </div>

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
