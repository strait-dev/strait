import { Button } from "@strait/ui/components/button";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useCallback, useState } from "react";
import {
  orgUsageQueryOptions,
  useApproachingLimits,
} from "@/hooks/billing/use-org-usage";

const UpgradeNudgeBanner = () => {
  const approaching = useApproachingLimits();
  const { data: usageData } = useQuery(orgUsageQueryOptions());
  const navigate = useNavigate();

  const orgId = usageData?.org_id ?? "";
  const periodStart = usageData?.period?.start ?? "";
  const storageKey = `strait:banner_dismissed:${orgId}:${periodStart}`;

  const [dismissed, setDismissed] = useState(() => {
    if (typeof window === "undefined") {
      return false;
    }
    return localStorage.getItem(storageKey) === "true";
  });

  const handleUpgrade = useCallback(() => {
    navigate({ to: "/app/upgrade" });
  }, [navigate]);

  const handleDismiss = () => {
    setDismissed(true);
    localStorage.setItem(storageKey, "true");
  };

  if (!approaching.length || dismissed) {
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
