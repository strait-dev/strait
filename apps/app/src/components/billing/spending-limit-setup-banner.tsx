import { Button } from "@strait/ui/components/button";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { orgUsageQueryOptions } from "@/hooks/billing/use-org-usage";
import {
  spendingLimitQueryOptions,
  useUpdateSpendingLimit,
} from "@/hooks/billing/use-spending-limit";

const MICRO_USD = 1_000_000;
const PRESET_AMOUNTS = [25, 50, 100, 250];
const PAID_PLANS = new Set(["starter", "pro", "scale"]);

const SpendingLimitSetupBanner = () => {
  const { data: usage } = useQuery(orgUsageQueryOptions());
  const { data: spending } = useQuery(spendingLimitQueryOptions());
  const updateLimit = useUpdateSpendingLimit();

  const orgId = usage?.org_id ?? "";
  const storageKey = `strait:spending_prompt_dismissed:${orgId}`;

  const [dismissed, setDismissed] = useState(() => {
    if (typeof window === "undefined") {
      return false;
    }
    return localStorage.getItem(storageKey) === "true";
  });

  if (dismissed || !usage || !spending) {
    return null;
  }

  const plan = usage.plan;
  const isPaid = PAID_PLANS.has(plan);
  const hasNoLimit = spending.spending_limit_usd === -1;

  if (!(isPaid && hasNoLimit)) {
    return null;
  }

  const handleSetLimit = (dollars: number) => {
    updateLimit.mutate({
      limitMicrousd: dollars * MICRO_USD,
      action: "reject",
    });
    handleDismiss();
  };

  const handleDismiss = () => {
    setDismissed(true);
    localStorage.setItem(storageKey, "true");
  };

  return (
    <div className="rounded border border-border bg-card px-4 py-4">
      <div className="flex flex-col gap-3">
        <div>
          <p className="font-medium text-foreground text-sm">
            Set a spending limit
          </p>
          <p className="text-muted-foreground text-sm">
            Control your monthly costs and avoid unexpected charges. You can
            change this anytime in Spending settings.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {PRESET_AMOUNTS.map((amount) => (
            <Button
              disabled={updateLimit.isPending}
              key={amount}
              onClick={() => handleSetLimit(amount)}
              variant="outline"
            >
              ${amount}
            </Button>
          ))}
          <Button
            disabled={updateLimit.isPending}
            onClick={handleDismiss}
            variant="ghost"
          >
            No limit
          </Button>
        </div>
      </div>
    </div>
  );
};

export default SpendingLimitSetupBanner;
