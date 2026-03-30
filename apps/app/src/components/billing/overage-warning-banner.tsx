import { Button } from "@strait/ui/components/button";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { orgUsageQueryOptions } from "@/hooks/billing/use-org-usage";

const MICRO_USD = 1_000_000;

const OverageWarningBanner = () => {
  const { data } = useQuery(orgUsageQueryOptions());

  const orgId = data?.org_id ?? "";
  const periodStart = data?.period?.start ?? "";
  const storageKey = `strait:overage_dismissed:${orgId}:${periodStart}`;

  const [dismissed, setDismissed] = useState(() => {
    if (typeof window === "undefined") {
      return false;
    }
    return localStorage.getItem(storageKey) === "true";
  });

  if (!data || dismissed) {
    return null;
  }

  const creditUsedPercent = data.credit_used_percent ?? 0;
  const overageMicro = data.overage_microusd ?? 0;
  const includedCreditMicro = data.included_credit_microusd ?? 0;
  const includedCreditDollars = (includedCreditMicro / MICRO_USD).toFixed(2);
  const overageDollars = (overageMicro / MICRO_USD).toFixed(2);

  const isInOverage = overageMicro > 0;
  const isApproachingLimit = creditUsedPercent >= 80 && !isInOverage;

  if (!isInOverage && !isApproachingLimit) {
    return null;
  }

  const handleDismiss = () => {
    setDismissed(true);
    localStorage.setItem(storageKey, "true");
  };

  if (isInOverage) {
    return (
      <div className="flex items-center justify-between rounded-custom border border-destructive/30 bg-destructive/5 px-4 py-3">
        <p className="text-sm text-destructive">
          You're <strong>${overageDollars}</strong> over your included credit.
          Set a spending limit to control costs.
        </p>
        <div className="flex items-center gap-2">
          <a href="/app/billing?tab=spending">
            <Button size="sm" variant="destructive">
              Set limit
            </Button>
          </a>
          <Button onClick={handleDismiss} size="sm" variant="ghost">
            Dismiss
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="flex items-center justify-between rounded-custom border border-yellow-200 bg-yellow-50 px-4 py-3 dark:border-yellow-800 dark:bg-yellow-950">
      <p className="text-sm text-yellow-800 dark:text-yellow-200">
        You've used <strong>{Math.round(creditUsedPercent)}%</strong> of your $
        {includedCreditDollars} compute credit this period.
      </p>
      <Button onClick={handleDismiss} size="sm" variant="ghost">
        Dismiss
      </Button>
    </div>
  );
};

export default OverageWarningBanner;
