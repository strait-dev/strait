import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import {
  DescriptionDetails,
  DescriptionList,
  DescriptionTerm,
} from "@strait/ui/components/description-list";
import { Field, FieldLabel } from "@strait/ui/components/field";
import { NumberInputWithChevrons } from "@strait/ui/components/number-input-with-chevrons";
import { useState } from "react";
import { formatCurrency } from "@/lib/format";

type Provider = "trigger.dev" | "inngest" | "hatchet" | "temporal";

const PROVIDERS: { id: Provider; name: string }[] = [
  { id: "trigger.dev", name: "Trigger.dev" },
  { id: "inngest", name: "Inngest" },
  { id: "hatchet", name: "Hatchet" },
  { id: "temporal", name: "Temporal" },
];

type ComparisonResult = {
  currentCost: number;
  straitCost: number;
  savings: number;
  recommendedPlan: string;
  advantages: string[];
};

function estimateStraitCost(
  runsPerMonth: number,
  _computeHours: number,
  _teamMembers: number
): ComparisonResult {
  let recommendedPlan = "Free";
  let planCost = 0;

  if (runsPerMonth > 25_000_000) {
    recommendedPlan = "Enterprise";
    planCost = 1500;
  } else if (runsPerMonth > 5_000_000) {
    recommendedPlan = "Business";
    planCost = 499;
  } else if (runsPerMonth > 1_000_000 || _teamMembers > 10) {
    recommendedPlan = "Scale";
    planCost = 299;
  } else if (runsPerMonth > 50_000 || _teamMembers > 3) {
    recommendedPlan = "Pro";
    planCost = 99;
  } else if (runsPerMonth > 5000 || _teamMembers > 1) {
    recommendedPlan = "Starter";
    planCost = 19;
  }

  return {
    currentCost: 0,
    straitCost: planCost,
    savings: 0,
    recommendedPlan,
    advantages: [
      "Orchestration-run billing with no compute-time metering",
      "HTTP and worker execution modes",
      "Workflow retries, scheduling, and observability",
      "Open API and SDK access",
      "No vendor lock-in",
    ],
  };
}

const MigrationCalculator = () => {
  const [selectedProvider, setSelectedProvider] = useState<Provider | null>(
    null
  );
  const [runsPerMonth, setRunsPerMonth] = useState(50_000);
  const [computeHours, setComputeHours] = useState(20);
  const [teamMembers, setTeamMembers] = useState(8);
  const [currentCost, setCurrentCost] = useState(75);
  const [result, setResult] = useState<ComparisonResult | null>(null);

  const handleCalculate = () => {
    const estimation = estimateStraitCost(
      runsPerMonth,
      computeHours,
      teamMembers
    );
    estimation.currentCost = currentCost;
    estimation.savings = currentCost - estimation.straitCost;
    setResult(estimation);
  };

  return (
    <div className="space-y-6">
      <div className="text-center">
        <h2 className="text-balance font-normal text-foreground text-lg tracking-tight">
          Compare & save
        </h2>
        <p className="text-muted-foreground text-sm">
          See how much you could save by switching to Strait
        </p>
      </div>

      {/* Provider Selection */}
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        {PROVIDERS.map((provider) => (
          <Button
            key={provider.id}
            onClick={() => setSelectedProvider(provider.id)}
            variant={selectedProvider === provider.id ? "default" : "outline"}
          >
            {provider.name}
          </Button>
        ))}
      </div>

      {/* Usage Inputs */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Your current usage</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <Field>
            <FieldLabel htmlFor="runs">Runs per month</FieldLabel>
            <NumberInputWithChevrons
              id="runs"
              min={0}
              name="runs"
              onChange={setRunsPerMonth}
              value={runsPerMonth}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="compute">
              Current provider compute hours/month
            </FieldLabel>
            <NumberInputWithChevrons
              id="compute"
              min={0}
              name="compute"
              onChange={setComputeHours}
              value={computeHours}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="members">Team members</FieldLabel>
            <NumberInputWithChevrons
              id="members"
              min={0}
              name="members"
              onChange={setTeamMembers}
              value={teamMembers}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="cost">Current monthly cost ($)</FieldLabel>
            <NumberInputWithChevrons
              formatOptions={{ currency: "USD", style: "currency" }}
              id="cost"
              min={0}
              name="cost"
              onChange={setCurrentCost}
              value={currentCost}
            />
          </Field>

          <Button className="w-full" onClick={handleCalculate}>
            Calculate savings
          </Button>
        </CardContent>
      </Card>

      {/* Results */}
      {result && (
        <Card variant="outline">
          <CardContent className="space-y-4 p-4">
            <div className="text-center">
              {selectedProvider && (
                <p className="text-muted-foreground text-sm">
                  Switching from{" "}
                  {PROVIDERS.find((p) => p.id === selectedProvider)?.name} to
                  Strait
                </p>
              )}
              <DescriptionList
                className="mt-3"
                divided
                orientation="horizontal"
                size="sm"
              >
                <DescriptionTerm>Current</DescriptionTerm>
                <DescriptionDetails className="text-right tabular-nums">
                  {formatCurrency(result.currentCost)}/mo
                </DescriptionDetails>
                <DescriptionTerm>
                  Strait ({result.recommendedPlan})
                </DescriptionTerm>
                <DescriptionDetails className="text-right tabular-nums">
                  {formatCurrency(result.straitCost)}/mo
                </DescriptionDetails>
                <DescriptionTerm>You save</DescriptionTerm>
                <DescriptionDetails className="text-right">
                  <Badge size="sm" variant="success-light">
                    {formatCurrency(Math.max(result.savings, 0))}/mo
                  </Badge>
                </DescriptionDetails>
              </DescriptionList>
            </div>

            <div className="space-y-1">
              <p className="font-medium text-foreground text-xs">
                Plus you get:
              </p>
              {result.advantages.map((adv) => (
                <p className="text-muted-foreground text-xs" key={adv}>
                  - {adv}
                </p>
              ))}
            </div>

            <Button className="w-full" variant="default">
              Get started free
            </Button>
          </CardContent>
        </Card>
      )}
    </div>
  );
};

export default MigrationCalculator;
