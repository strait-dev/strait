import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Input } from "@strait/ui/components/input";
import { toast } from "@strait/ui/components/toast/index";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import {
  AGENT_SPENDING_PRESETS,
  formatMicroUsd,
  usdToMicrousd,
} from "@/hooks/api/agent-billing-utils";
import {
  agentSpendingLimitQueryOptions,
  useUpdateAgentSpendingLimit,
} from "@/hooks/api/use-agent-billing";

type AgentSpendingLimitCardProps = {
  orgId: string;
};

const INITIAL_CUSTOM_AMOUNT = "";

/**
 * Card for viewing and setting the optional agent spending limit.
 * Follows the same pattern as the Jobs spending-limits-tab.
 */
function AgentSpendingLimitCard({ orgId }: AgentSpendingLimitCardProps) {
  const { data } = useQuery(agentSpendingLimitQueryOptions(orgId));
  const mutation = useUpdateAgentSpendingLimit(orgId);
  const [customAmount, setCustomAmount] = useState(INITIAL_CUSTOM_AMOUNT);

  const currentLimitUsd = data?.limit_usd ?? -1;
  const isEnabled = data?.enabled ?? false;

  function handlePreset(usd: number) {
    mutation.mutate(usdToMicrousd(usd), {
      onSuccess: () => {
        toast.success(`Agent spending limit set to $${usd}`);
      },
      onError: () => {
        toast.error("Failed to update spending limit");
      },
    });
  }

  function handleCustom() {
    const usd = Number.parseFloat(customAmount);
    if (Number.isNaN(usd) || usd <= 0) {
      toast.error("Enter a valid positive amount");
      return;
    }
    mutation.mutate(usdToMicrousd(usd), {
      onSuccess: () => {
        toast.success(`Agent spending limit set to $${usd}`);
        setCustomAmount(INITIAL_CUSTOM_AMOUNT);
      },
      onError: () => {
        toast.error("Failed to update spending limit");
      },
    });
  }

  function handleDisable() {
    mutation.mutate(-1, {
      onSuccess: () => {
        toast.success("Agent spending limit disabled");
      },
      onError: () => {
        toast.error("Failed to disable spending limit");
      },
    });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Agent Spending Limit</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-muted-foreground text-sm">
          {isEnabled
            ? `Current limit: ${formatMicroUsd(data?.limit_microusd ?? -1)}/month`
            : "No spending limit set. Agent overage is charged at period end."}
        </p>

        <div className="flex flex-wrap gap-2">
          {AGENT_SPENDING_PRESETS.map((usd) => (
            <Button
              disabled={mutation.isPending}
              key={usd}
              onClick={() => handlePreset(usd)}
              size="sm"
              variant={currentLimitUsd === usd ? "default" : "outline"}
            >
              ${usd}
            </Button>
          ))}
        </div>

        <div className="flex items-center gap-2">
          <div className="relative max-w-[160px]">
            <span className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground text-sm">
              $
            </span>
            <Input
              className="pl-6"
              min={1}
              onChange={(e) => setCustomAmount(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && handleCustom()}
              placeholder="Custom"
              type="number"
              value={customAmount}
            />
          </div>
          <Button
            disabled={mutation.isPending || !customAmount}
            onClick={handleCustom}
            size="sm"
          >
            Set
          </Button>
          {isEnabled && (
            <Button
              disabled={mutation.isPending}
              onClick={handleDisable}
              size="sm"
              variant="ghost"
            >
              Remove limit
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

export default AgentSpendingLimitCard;
