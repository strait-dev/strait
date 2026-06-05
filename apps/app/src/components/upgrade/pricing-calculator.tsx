import { PLANS, type PlanKey } from "@strait/billing";
import { Badge } from "@strait/ui/components/badge";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Field, FieldLabel } from "@strait/ui/components/field";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemGroup,
  ItemTitle,
} from "@strait/ui/components/item";
import { Separator } from "@strait/ui/components/separator";
import { Slider } from "@strait/ui/components/slider";
import { useState } from "react";

type SelfServePlanTier = Exclude<PlanKey, "enterprise">;

const PLAN_TIERS = [
  "free",
  "starter",
  "pro",
  "scale",
  "business",
] as const satisfies readonly SelfServePlanTier[];

const calculatePlanCost = (
  tier: SelfServePlanTier,
  runsPerMonth: number,
  members: number
): number | null => {
  const plan = PLANS[tier];
  const monthlyPrice = plan.prices.monthly / 100;
  const includedRuns = plan.limits.runsPerMonth;
  const memberLimit = plan.limits.membersPerOrg;
  const overagePerThousandRuns = plan.limits.overagePerThousandRuns ?? 0;

  if (memberLimit !== null && members > memberLimit) {
    return null;
  }

  let cost = monthlyPrice;

  if (includedRuns !== null && runsPerMonth > includedRuns) {
    const overageRuns = runsPerMonth - includedRuns;
    cost += (overageRuns / 1000) * overagePerThousandRuns;
  }

  return cost;
};

const findRecommendedPlan = (
  costs: { tier: SelfServePlanTier; cost: number | null }[]
) => {
  const validCosts = costs.filter(
    (c): c is { tier: SelfServePlanTier; cost: number } => c.cost !== null
  );
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

const getSliderValue = (value: number | readonly number[], fallback: number) =>
  Array.isArray(value) ? (value[0] ?? fallback) : value;

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
        <Field>
          <div className="flex items-center justify-between">
            <FieldLabel>Runs per month</FieldLabel>
            <span className="font-mono text-sm tabular-nums">
              {runsPerMonth.toLocaleString()}
            </span>
          </div>
          <Slider
            aria-label="Runs per month"
            max={30_000_000}
            min={0}
            onValueChange={(value) => setRunsPerMonth(getSliderValue(value, 0))}
            step={50_000}
            value={[runsPerMonth]}
          />
          <div className="flex justify-between text-muted-foreground text-xs">
            <span>0</span>
            <span>30M</span>
          </div>
        </Field>

        <Field>
          <div className="flex items-center justify-between">
            <FieldLabel>Team members</FieldLabel>
            <span className="font-mono text-sm tabular-nums">{members}</span>
          </div>
          <Slider
            aria-label="Team members"
            max={50}
            min={1}
            onValueChange={(value) => setMembers(getSliderValue(value, 1))}
            step={1}
            value={[members]}
          />
          <div className="flex justify-between text-muted-foreground text-xs">
            <span>1</span>
            <span>50</span>
          </div>
        </Field>

        <div className="space-y-3 pt-1">
          <Separator />
          <ItemGroup className="gap-2">
            {planCosts.map(({ tier, cost }) => (
              <Item key={tier} size="xs" variant="ghost">
                <ItemContent>
                  <ItemTitle>
                    {PLANS[tier].name}
                    {recommended === tier ? (
                      <Badge variant="success-light">Best value</Badge>
                    ) : null}
                  </ItemTitle>
                </ItemContent>
                <ItemActions>
                  <span className="font-mono text-sm tabular-nums">
                    {cost === null
                      ? "Limit exceeded"
                      : `$${cost.toFixed(2)}/mo`}
                  </span>
                </ItemActions>
              </Item>
            ))}
          </ItemGroup>
        </div>
      </CardContent>
    </Card>
  );
};

export default PricingCalculator;
