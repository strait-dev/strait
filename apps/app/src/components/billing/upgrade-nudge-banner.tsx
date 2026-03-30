import { Button } from "@strait/ui/components/button";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useCallback, useState } from "react";
import {
  orgUsageQueryOptions,
  useApproachingLimits,
} from "@/hooks/billing/use-org-usage";
import { usageForecastQueryOptions } from "@/hooks/billing/use-usage-forecast";

const MICRO_USD = 1_000_000;

const UpgradeNudgeBanner = () => {
  const approaching = useApproachingLimits();
  const { data: usageData } = useQuery(orgUsageQueryOptions());
  const { data: forecast } = useQuery(usageForecastQueryOptions());
  const navigate = useNavigate();

  const orgId = usageData?.org_id ?? "";
  const periodStart = usageData?.period?.start ?? "";
  const storageKey =
    orgId && periodStart
      ? `strait:banner_dismissed:${orgId}:${periodStart}`
      : "";

  const [dismissed, setDismissed] = useState(() => {
    if (typeof window === "undefined" || !storageKey) {
      return false;
    }
    return localStorage.getItem(storageKey) === "true";
  });

  const handleUpgrade = useCallback(() => {
    navigate({ to: "/app/upgrade" });
  }, [navigate]);

  const handleDismiss = () => {
    setDismissed(true);
    if (storageKey) {
      localStorage.setItem(storageKey, "true");
    }
  };

  if (dismissed) {
    return null;
  }

  const currentPlan = usageData?.plan;

  // Priority 1: Scale breakeven -- only show to Pro users.
  if (forecast?.scale_breakeven && currentPlan === "pro") {
    return (
      <div className="flex items-center justify-between rounded-custom border border-accent/30 bg-accent/5 px-4 py-2">
        <p className="text-foreground text-sm">
          Your projected spend exceeds $99/mo. Upgrade to{" "}
          <strong>Scale</strong> for the same price and get 5x concurrent runs,
          audit logs, and canary deploys.
        </p>
        <div className="flex items-center gap-2">
          <Button onClick={handleUpgrade} size="sm" variant="default">
            Upgrade to Scale
          </Button>
          <Button onClick={handleDismiss} size="sm" variant="ghost">
            Dismiss
          </Button>
        </div>
      </div>
    );
  }

  // Priority 2: Projected overage warning for Pro users.
  if (
    forecast?.projected_overage_microusd &&
    forecast.projected_overage_microusd > 0 &&
    currentPlan === "pro"
  ) {
    const projectedOverage = (
      forecast.projected_overage_microusd / MICRO_USD
    ).toFixed(2);
    return (
      <div className="flex items-center justify-between rounded-custom border border-yellow-200 bg-yellow-50 px-4 py-2 dark:border-yellow-800 dark:bg-yellow-950">
        <p className="text-sm text-yellow-800 dark:text-yellow-200">
          Your projected overage this month is{" "}
          <strong>${projectedOverage}</strong>. Consider upgrading for more
          included credit.
        </p>
        <div className="flex items-center gap-2">
          <Button onClick={handleUpgrade} size="sm" variant="default">
            Upgrade
          </Button>
          <Button onClick={handleDismiss} size="sm" variant="ghost">
            Dismiss
          </Button>
        </div>
      </div>
    );
  }

  // Priority 3: Existing approaching limit alerts.
  if (!approaching.length) {
    return null;
  }

  const firstAlert = approaching[0];

  return (
    <div className="flex items-center justify-between rounded-custom border border-yellow-200 bg-yellow-50 px-4 py-2 dark:border-yellow-800 dark:bg-yellow-950">
      <p className="text-sm text-yellow-800 dark:text-yellow-200">
        {firstAlert.message}
      </p>
      <div className="flex items-center gap-2">
        <Button onClick={handleUpgrade} size="sm" variant="default">
          Upgrade
        </Button>
        <Button onClick={handleDismiss} size="sm" variant="ghost">
          Dismiss
        </Button>
      </div>
    </div>
  );
};

export default UpgradeNudgeBanner;
