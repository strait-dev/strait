import { Badge } from "@strait/ui/components/badge";
import { Card, CardContent } from "@strait/ui/components/card";
import { Progress } from "@strait/ui/components/progress";
import { useQuery } from "@tanstack/react-query";
import {
  computeCreditPercent,
  formatAgentPlanTier,
  formatMicroUsd,
  formatTokenCount,
} from "@/hooks/api/agent-billing-utils";
import { agentUsageQueryOptions } from "@/hooks/api/use-agent-billing";
import AgentSpendingLimitCard from "./agent-spending-limit-card";

type AgentBillingTabProps = {
  orgId: string;
};

/**
 * Renders the Agents billing tab with plan tier, credit usage,
 * stat cards, overage info, and spending limit controls.
 */
function AgentBillingTab({ orgId }: AgentBillingTabProps) {
  const { data } = useQuery(agentUsageQueryOptions(orgId));

  if (!data) {
    return null;
  }

  const creditPercent = computeCreditPercent(
    data.used_credit_usd,
    data.included_credit_usd
  );
  const progressValue = Math.min(creditPercent, 100);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="font-medium text-base">Agent Plan</h3>
          <p className="text-muted-foreground text-sm">
            Agent execution billing for the current period.
          </p>
        </div>
        <Badge className="text-sm" variant="outline">
          {formatAgentPlanTier(data.agent_plan_tier)}
        </Badge>
      </div>

      <div>
        <div className="mb-2 flex items-center justify-between text-sm">
          <span className="text-muted-foreground">Credit Usage</span>
          <span className="font-medium tabular-nums">
            ${data.used_credit_usd.toFixed(2)} / $
            {data.included_credit_usd.toFixed(2)}
          </span>
        </div>
        <Progress className="h-2" value={progressValue} />
        {creditPercent > 100 && (
          <p className="mt-1 text-sm text-yellow-600">
            ${data.overage_usd.toFixed(2)} in overage this period
          </p>
        )}
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          label="Managed Runs"
          value={data.run_count.toLocaleString()}
        />
        <StatCard
          label="Total Cost"
          value={formatMicroUsd(data.total_cost_microusd)}
        />
        <StatCard
          label="Tokens Tracked"
          value={formatTokenCount(data.total_tokens)}
        />
        <StatCard
          label="Tool Calls"
          value={data.total_tool_calls.toLocaleString()}
        />
      </div>

      {data.upgrade_recommended && data.upgrade_reason && (
        <div className="rounded-lg border border-accent/30 bg-accent/5 p-4">
          <p className="text-sm">{data.upgrade_reason}</p>
        </div>
      )}

      <AgentSpendingLimitCard orgId={orgId} />
    </div>
  );
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <Card>
      <CardContent className="p-4">
        <p className="text-muted-foreground text-xs">{label}</p>
        <p className="mt-1 font-medium text-foreground text-lg tabular-nums">
          {value}
        </p>
      </CardContent>
    </Card>
  );
}

export default AgentBillingTab;
