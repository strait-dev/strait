import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { useCallback, useState } from "react";
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
  computeHours: number,
  _teamMembers: number
): ComparisonResult {
  const microPerSecond = 0.017;
  const monthlyComputeCost = computeHours * 3600 * microPerSecond;

  let recommendedPlan = "Free";
  let planCost = 0;

  // Plan thresholds based on actual plan limits:
  // Free: 5K runs/day (~150K/mo), 3 members, no compute credit
  // Starter: 25K runs/day (~750K/mo), 10 members, $19.99 compute credit
  // Pro: 100K runs/day (~3M/mo), 25 members, $49.99 compute credit
  if (
    runsPerMonth > 750_000 ||
    monthlyComputeCost > 19.99 ||
    _teamMembers > 10
  ) {
    recommendedPlan = "Pro";
    planCost = 49.99;
  } else if (
    runsPerMonth > 150_000 ||
    monthlyComputeCost > 0 ||
    _teamMembers > 3
  ) {
    recommendedPlan = "Starter";
    planCost = 19.99;
  }

  return {
    currentCost: 0,
    straitCost: planCost,
    savings: 0,
    recommendedPlan,
    advantages: [
      "5 SDKs (TypeScript, Python, Go, Rust, Java)",
      "All features on every plan",
      "MCP server for AI agents",
      "Apache 2.0 license",
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

  const handleCalculate = useCallback(() => {
    const estimation = estimateStraitCost(
      runsPerMonth,
      computeHours,
      teamMembers
    );
    estimation.currentCost = currentCost;
    estimation.savings = currentCost - estimation.straitCost;
    setResult(estimation);
  }, [runsPerMonth, computeHours, teamMembers, currentCost]);

  return (
    <div className="space-y-6">
      <div className="text-center">
        <h2 className="text-balance font-normal text-foreground text-lg tracking-tight">
          Compare & Save
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
          <CardTitle className="text-sm">Your Current Usage</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div>
            <label className="text-muted-foreground text-xs" htmlFor="runs">
              Runs per month
            </label>
            <input
              className="mt-1 w-full rounded-custom border border-border bg-background px-3 py-2 text-sm"
              id="runs"
              onChange={(e) => setRunsPerMonth(Number(e.target.value))}
              type="number"
              value={runsPerMonth}
            />
          </div>
          <div>
            <label className="text-muted-foreground text-xs" htmlFor="compute">
              Compute hours/month
            </label>
            <input
              className="mt-1 w-full rounded-custom border border-border bg-background px-3 py-2 text-sm"
              id="compute"
              onChange={(e) => setComputeHours(Number(e.target.value))}
              type="number"
              value={computeHours}
            />
          </div>
          <div>
            <label className="text-muted-foreground text-xs" htmlFor="members">
              Team members
            </label>
            <input
              className="mt-1 w-full rounded-custom border border-border bg-background px-3 py-2 text-sm"
              id="members"
              onChange={(e) => setTeamMembers(Number(e.target.value))}
              type="number"
              value={teamMembers}
            />
          </div>
          <div>
            <label className="text-muted-foreground text-xs" htmlFor="cost">
              Current monthly cost ($)
            </label>
            <input
              className="mt-1 w-full rounded-custom border border-border bg-background px-3 py-2 text-sm"
              id="cost"
              onChange={(e) => setCurrentCost(Number(e.target.value))}
              type="number"
              value={currentCost}
            />
          </div>

          <Button className="w-full" onClick={handleCalculate}>
            Calculate Savings
          </Button>
        </CardContent>
      </Card>

      {/* Results */}
      {result && (
        <Card className="border-green-200 dark:border-green-800">
          <CardContent className="space-y-4 p-6">
            <div className="text-center">
              {selectedProvider && (
                <p className="text-muted-foreground text-sm">
                  Switching from{" "}
                  {PROVIDERS.find((p) => p.id === selectedProvider)?.name} to
                  Strait
                </p>
              )}
              <div className="mt-2 grid grid-cols-3 gap-4">
                <div>
                  <p className="text-muted-foreground text-xs">Current</p>
                  <p className="font-medium text-foreground text-lg">
                    {formatCurrency(result.currentCost)}
                    <span className="text-muted-foreground text-xs">/mo</span>
                  </p>
                </div>
                <div>
                  <p className="text-muted-foreground text-xs">
                    Strait ({result.recommendedPlan})
                  </p>
                  <p className="font-medium text-foreground text-lg">
                    {formatCurrency(result.straitCost)}
                    <span className="text-muted-foreground text-xs">/mo</span>
                  </p>
                </div>
                <div>
                  <p className="text-muted-foreground text-xs">You save</p>
                  <p className="font-medium text-green-600 text-lg dark:text-green-400">
                    {formatCurrency(Math.max(result.savings, 0))}
                    <span className="text-muted-foreground text-xs">/mo</span>
                  </p>
                </div>
              </div>
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
              Get Started Free
            </Button>
          </CardContent>
        </Card>
      )}
    </div>
  );
};

export default MigrationCalculator;
