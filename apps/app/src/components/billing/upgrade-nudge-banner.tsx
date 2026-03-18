import { Button } from "@strait/ui/components/button";
import { useNavigate } from "@tanstack/react-router";
import { useCallback, useState } from "react";
import { useApproachingLimits } from "@/hooks/billing/use-org-usage";

export function UpgradeNudgeBanner() {
  const approaching = useApproachingLimits();
  const navigate = useNavigate();
  const [dismissed, setDismissed] = useState(false);

  const handleUpgrade = useCallback(() => {
    navigate({ to: "/app/upgrade" });
  }, [navigate]);

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
        <Button
          onClick={() => setDismissed(true)}
          size="sm"
          variant="ghost"
        >
          Dismiss
        </Button>
      </div>
    </div>
  );
}
